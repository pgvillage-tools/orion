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
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/consensus"
	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/util"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// ClusterRoutes adds all Cluster routes to the list of all routes
func (h *Handlers) ClusterRoutes() []Route {
	return []Route{
		{endpoints.ClusterEndPoint.Route(endpoints.MethodGet), h.GetClusterHandler},
		{endpoints.ClusterEndPoint.Route(endpoints.MethodPut), h.PutClusterHandler},
		{endpoints.ClusterEndPoint.Route(endpoints.MethodPost), h.PostClusterHandler},
	}
}

// GetClusterHandler endpoint
func (h *Handlers) GetClusterHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(cfg.StoreTimeout))
	defer cancelFunc()

	if cd, _, err := h.client.GetClusterData(ctx); err != nil {
		handleError(ctx, err, w, "failed to get cluster data")
	} else {
		if cd == nil {
			endpoints.WriteProblem(w, http.StatusNotFound, "there is no cluster data yet")
		} else {
			h.writeJSON(ctx, w, cd)
		}
	}
}

// PutClusterHandler endpoint
func (h *Handlers) PutClusterHandler(w http.ResponseWriter, r *http.Request) {
	ctx, logger := logging.GetLogComponent(r.Context(), logging.WebApiComponent)
	ctx, cancelFunc := context.WithDeadline(ctx, time.Now().Add(cfg.StoreTimeout))
	defer cancelFunc()

	reader := http.MaxBytesReader(w, r.Body, readMaxBytes)
	defer func() {
		if err := reader.Close(); err != nil {
			logger.Error().AnErr("error", err).Msg("Failed to close Body")
		}
	}()
	var newCD apiv1.Data
	err := json.NewDecoder(reader).Decode(&newCD)
	if err != nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, "failed to parse the cluster data: "+err.Error())
		return
	}
	if newCD.Cluster == nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, "the cluster data has no cluster")
		return
	}
	if newCD.Cluster.Spec == nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, "the cluster has no spec")
		return
	}
	if err = newCD.Cluster.Spec.Validate(); err != nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, "invalid spec: "+err.Error())
		return
	}
	// replace the cluster data, whatever it is now (like stolonctl clusterdata write)
	if err = h.client.PutClusterData(ctx, &newCD); err != nil {
		handleError(ctx, err, w, "failed to write new cluster data definition")
	}
}

// PostClusterHandler creates the cluster, from the spec in the body (an empty body creates a new cluster with the
// default spec), and answers 201 with the cluster data. It refuses (409) when there already is a cluster with a spec.
func (h *Handlers) PostClusterHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(time.Second))
	defer cancelFunc()

	cd, pair, err := h.client.GetClusterData(ctx)
	if err != nil {
		handleError(ctx, err, w, "failed to get cluster data")
		return
	}
	if cd != nil && cd.Cluster != nil && cd.Cluster.Spec != nil {
		endpoints.WriteProblem(w, http.StatusConflict,
			"the cluster already exists; use PUT or PATCH on /v1/cluster/spec to change its spec")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, readMaxBytes))
	if err != nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, "failed to read the body: "+err.Error())
		return
	}
	var cs *apiv1.Spec
	if len(body) == 0 {
		cs = &apiv1.Spec{InitMode: util.ToPtr(apiv1.New)}
	} else if err = json.Unmarshal(body, &cs); err != nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, "failed to parse the spec: "+err.Error())
		return
	}
	if cs == nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, "the spec can't be null")
		return
	}
	if err = cs.Validate(); err != nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, "invalid spec: "+err.Error())
		return
	}

	switch {
	case cd == nil:
		cd = apiv1.NewClusterData(apiv1.NewCluster(common.UID(), cs))
	case cd.Cluster == nil:
		cd.Cluster = apiv1.NewCluster(common.UID(), cs)
	default:
		// a cluster without a spec
		cd.Cluster.Spec = cs
	}
	// only write when the cluster data didn't change since it was read (pair is nil when there was none: then it
	// is only written when there still is none), so two clients creating a cluster at the same time can't both win
	if _, err = h.client.AtomicPutClusterData(ctx, cd, pair); err != nil {
		if errors.Is(err, consensus.ErrKeyModified) {
			endpoints.WriteProblem(w, http.StatusConflict, "the cluster data changed while creating the cluster")
			return
		}
		handleError(ctx, err, w, "cannot create the cluster")
		return
	}
	w.Header().Set("Location", "/"+endpoints.APIVersion+"/"+string(endpoints.ClusterEndPoint))
	h.writeJSONStatus(ctx, w, http.StatusCreated, cd)
}
