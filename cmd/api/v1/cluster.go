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
	"net/http"
	"time"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/logging"
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
			w.WriteHeader(http.StatusNotFound)
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
		handleError(ctx, err, w, "failed to parse new cluster data definition")
		return
	}
	_, err = h.client.AtomicPutClusterData(ctx, &newCD, nil)
	if err != nil {
		handleError(ctx, err, w, "failed to write new cluster data definition")
	}
}

// PostClusterHandler endpoint
func (h *Handlers) PostClusterHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(time.Second))
	defer cancelFunc()

	cd, _, err := h.client.GetClusterData(ctx)
	if err != nil {
		handleError(ctx, err, w, "failed to get cluster data")
	} else if cd == nil {
		_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
		logger.Debug().Msg("Cluster is nil")
	} else if cd.Cluster != nil {
		handleError(ctx, err, w, "cluster is already set")
	}

	cs := &apiv1.Spec{}

	newCSReader := http.MaxBytesReader(w, r.Body, readMaxBytes)
	defer func() {
		if err := r.Body.Close(); err != nil {
			handleError(ctx, err, w, "Failed to close Body")
		}
	}()
	newCSBytes := []byte{}
	n, err := newCSReader.Read(newCSBytes)
	if err != nil {
		handleError(ctx, err, w, "Failed to read Body")
	}
	if n == 0 {
		newCluster := apiv1.New
		cs.InitMode = &newCluster
	} else {
		err = json.Unmarshal(newCSBytes, &cs)
		if err != nil {
			handleError(ctx, err, w, "failed to parse spec definition")
			return
		}
	}

	if err := cs.Validate(); err != nil {
		handleError(ctx, err, w, "invalid spec")
		return
	}

	c := apiv1.NewCluster(common.UID(), cs)
	cd = apiv1.NewClusterData(c)

	// We ignore if cd has been modified between reading and writing
	if err := h.client.PutClusterData(ctx, cd); err != nil {
		handleError(ctx, err, w, "cannot update spec")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
