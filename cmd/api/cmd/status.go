// Copyright 2026 PgVillage
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	cmdcommon "github.com/pgvillage-tools/orion/cmd"
	"github.com/pgvillage-tools/orion/internal/consensus"
)

func (h *Handlers) StatusRoutes() []Route {
	return []Route{
		{"GET /status", h.StatusHandler},
		{"GET /proxy/status", h.ProxyStatusHandler},
		{"GET /sentinel/status", h.SentinelStatusHandler},
	}
}

// ProxyStatusHandler endpoint
func (h *Handlers) ProxyStatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(time.Second))
	defer cancelFunc()

	if proxiesInfo, err := h.client.GetProxiesInfo(ctx); err != nil {
		handleError(ctx, err, w, "failed to get proxy info")
	} else {
		h.writeJSON(ctx, w, proxiesInfo)
	}
}

// SentinelStatusHandler endpoint
func (h *Handlers) SentinelStatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(time.Second))
	defer cancelFunc()

	if sentinelsInfo, err := h.client.GetSentinelsInfo(ctx); err != nil {
		handleError(ctx, err, w, "failed to get sentinel status")
	} else {
		h.writeJSON(ctx, w, sentinelsInfo)
	}
}

func (h *Handlers) sentinelInfo(ctx context.Context) (is apiv1.InfoSentinels, err error) {
	var ssi apiv1.SentinelsInfo
	election, err := cmdcommon.NewElection(ctx, &cfg.CommonConfig, "")
	if err != nil {
		handleError(ctx, err, nil, "failed to get election info")
		return nil, err
	}
	lsid, err := election.Leader()
	if err != nil && err != consensus.ErrElectionNoLeader {
		handleError(ctx, err, nil, "failed to get leader info")
		return nil, err
	}
	if ssi, err = h.client.GetSentinelsInfo(ctx); err != nil {
		handleError(ctx, err, nil, "failed to get sentinels info")
		return nil, err
	}
	for _, si := range ssi {
		leader := lsid != "" && si.UID == lsid
		is = append(is, apiv1.InfoSentinel{
			UID:    si.UID,
			Leader: leader,
		})
	}
	return is, nil
}

func (h *Handlers) proxyInfo(ctx context.Context) (ip apiv1.InfoProxies, err error) {
	proxiesInfo, err := h.client.GetProxiesInfo(ctx)
	if err != nil {
		handleError(ctx, err, nil, "failed to get proxy info")
		return nil, err
	}
	proxiesInfoSlice := proxiesInfo.ToSlice()

	sort.Sort(proxiesInfoSlice)
	for _, pi := range proxiesInfoSlice {
		ip = append(ip, apiv1.InfoProxy{UID: pi.UID, Generation: pi.Generation})
	}
	return ip, nil
}

func (h *Handlers) keepersInfo(ctx context.Context, cd *apiv1.Data) (ik apiv1.InfoKeepers, err error) {
	kssKeys := cd.Keepers.SortedKeys()
	for _, kuid := range kssKeys {
		k := cd.Keepers[kuid]
		db := cd.FindDB(k)
		dbListenAddress := "(no db assigned)"
		var (
			pgHealthy           bool
			pgCurrentGeneration int64
			pgWantedGeneration  int64
		)
		if db != nil {
			pgHealthy = db.Status.Healthy
			pgCurrentGeneration = db.Status.CurrentGeneration
			pgWantedGeneration = db.Generation

			dbListenAddress = "(unknown)"
			if db.Status.ListenAddress != "" {
				dbListenAddress = fmt.Sprintf("%s:%s", db.Status.ListenAddress, db.Status.Port)
			}
		}
		ik = append(ik, apiv1.InfoKeeper{
			UID:                 kuid,
			ListenAddress:       dbListenAddress,
			Healthy:             k.Status.Healthy,
			PgHealthy:           pgHealthy,
			PgWantedGeneration:  pgWantedGeneration,
			PgCurrentGeneration: pgCurrentGeneration,
		})
	}
	return ik, err
}

func (h *Handlers) clusterInfo(ctx context.Context) (apiv1.InfoCluster, error) {
	var (
		cd  *apiv1.Data
		p   apiv1.InfoProxies
		s   apiv1.InfoSentinels
		k   apiv1.InfoKeepers
		err error
	)
	cd, _, err = h.client.GetClusterData(ctx)
	k, err = h.keepersInfo(ctx, cd)
	if err != nil {
		return apiv1.InfoCluster{}, err
	}
	p, err = h.proxyInfo(ctx)
	if err != nil {
		return apiv1.InfoCluster{}, err
	}
	s, err = h.sentinelInfo(ctx)

	var clusterStatus apiv1.InfoGenericStatus

	if cd.Cluster == nil || cd.DBs == nil {
		clusterStatus = apiv1.InfoGenericStatus{"available": false}
	} else if cd.Cluster.Status.Master == "" {
		clusterStatus = apiv1.InfoGenericStatus{"available": true}
	} else {
		primary := cd.Cluster.Status.Master
		primaryKeeper := cd.DBs[primary].Spec.KeeperUID
		clusterStatus = apiv1.InfoGenericStatus{
			"available":     true,
			"primaryDB":     cd.DBs[primary].UID,
			"primaryKeeper": cd.Keepers[primaryKeeper].UID,
		}
	}

	return apiv1.InfoCluster{
		Keepers:   k,
		Proxies:   p,
		Sentinels: s,
		Status:    clusterStatus,
	}, nil
}

func (h *Handlers) StatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(time.Second))
	defer cancelFunc()

	ic, err := h.clusterInfo(ctx)
	if err != nil {
		handleError(ctx, err, w, "")
		return
	}
	h.writeJSON(ctx, w, ic)
}
