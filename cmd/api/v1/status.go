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

package v1

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/consensus"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// StatusRoutes collects and returns all Status routes
func (h *Handlers) StatusRoutes() []Route {
	return []Route{
		{endpoints.StatusEndPoint.Route(endpoints.MethodGet), h.StatusHandler},
		{endpoints.ProxiesEndPoint.Route(endpoints.MethodGet), h.ProxiesHandler},
		{endpoints.SentinelsEndPoint.Route(endpoints.MethodGet), h.SentinelsHandler},
	}
}

// ProxiesHandler returns the proxies, sorted on uid (the same list as in the cluster status)
func (h *Handlers) ProxiesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(cfg.StoreTimeout))
	defer cancelFunc()

	proxies, err := h.proxyInfo(ctx)
	if err != nil {
		handleError(ctx, err, w, "failed to get the proxies")
		return
	}
	if proxies == nil {
		proxies = apiv1.InfoProxies{}
	}
	h.writeJSON(ctx, w, proxies)
}

// SentinelsHandler returns the sentinels, sorted on uid, with the leader (the same list as in the cluster status)
func (h *Handlers) SentinelsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(cfg.StoreTimeout))
	defer cancelFunc()

	sentinels, err := h.sentinelInfo(ctx)
	if err != nil {
		handleError(ctx, err, w, "failed to get the sentinels")
		return
	}
	if sentinels == nil {
		sentinels = apiv1.InfoSentinels{}
	}
	h.writeJSON(ctx, w, sentinels)
}

func (h *Handlers) sentinelInfo(ctx context.Context) (is apiv1.InfoSentinels, err error) {
	ssi, err := h.client.GetSentinelsInfo(ctx)
	if err != nil {
		handleError(ctx, err, nil, "failed to get sentinels info")
		return nil, err
	}
	lsid, err := h.election.Leader(ctx)
	if err != nil && err != consensus.ErrElectionNoLeader {
		handleError(ctx, err, nil, "failed to get leader info")
		return nil, err
	}
	for _, si := range ssi {
		leader := lsid != "" && si.UID == lsid
		is = append(is, apiv1.InfoSentinel{
			UID:    si.UID,
			Leader: leader,
		})
	}
	sort.Slice(is, func(i, j int) bool { return is[i].UID < is[j].UID })
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

// keepersInfo returns the keepers of cd, sorted on uid. It only reads cd, so it cannot fail.
func keepersInfo(cd *apiv1.Data) (ik apiv1.InfoKeepers) {
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
	return ik
}

func (h *Handlers) clusterInfo(ctx context.Context) (apiv1.InfoCluster, error) {
	var (
		cd  *apiv1.Data
		p   apiv1.InfoProxies
		s   apiv1.InfoSentinels
		err error
	)
	cd, _, err = h.client.GetClusterData(ctx)
	if err != nil {
		return apiv1.InfoCluster{}, err
	}
	if cd == nil {
		// no cluster data yet (no cluster was created): no keepers, and the cluster isn't available
		cd = &apiv1.Data{}
	}
	p, err = h.proxyInfo(ctx)
	if err != nil {
		return apiv1.InfoCluster{}, err
	}
	s, err = h.sentinelInfo(ctx)
	if err != nil {
		return apiv1.InfoCluster{}, err
	}

	return newInfoCluster(keepersInfo(cd), p, s, clusterStatus(cd)), nil
}

// newInfoCluster returns the cluster info, with empty lists instead of nil ones, so they are encoded as [] and not as
// null
func newInfoCluster(k apiv1.InfoKeepers, p apiv1.InfoProxies, s apiv1.InfoSentinels,
	status apiv1.InfoGenericStatus) apiv1.InfoCluster {
	if k == nil {
		k = apiv1.InfoKeepers{}
	}
	if p == nil {
		p = apiv1.InfoProxies{}
	}
	if s == nil {
		s = apiv1.InfoSentinels{}
	}
	return apiv1.InfoCluster{Keepers: k, Proxies: p, Sentinels: s, Status: status}
}

// clusterStatus returns the status of the cluster in cd:
//   - available: true when the cluster has a primary db whose keeper is in the cluster data. In a standby cluster
//     (role standby) that is the db that replicates from the external primary, and that the others replicate from.
//   - primaryDB and primaryKeeper: the uids of that db and its keeper (only when available)
//   - role: the role of the cluster, master or standby (only when the cluster data has a cluster)
func clusterStatus(cd *apiv1.Data) apiv1.InfoGenericStatus {
	status := apiv1.InfoGenericStatus{"available": false}
	if cd == nil || cd.Cluster == nil {
		return status
	}
	if role := cd.Cluster.DefSpec().Role; role != nil {
		status["role"] = string(*role)
	}
	db, ok := cd.DBs[cd.Cluster.Status.Master]
	if !ok || db == nil || db.Spec == nil {
		return status
	}
	keeper, ok := cd.Keepers[db.Spec.KeeperUID]
	if !ok || keeper == nil {
		return status
	}
	status["available"] = true
	status["primaryDB"] = db.UID
	status["primaryKeeper"] = keeper.UID
	return status
}

// StatusHandler handles a status request
func (h *Handlers) StatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(cfg.StoreTimeout))
	defer cancelFunc()

	ic, err := h.clusterInfo(ctx)
	if err != nil {
		handleError(ctx, err, w, "failed to get the cluster status")
		return
	}
	h.writeJSON(ctx, w, ic)
}
