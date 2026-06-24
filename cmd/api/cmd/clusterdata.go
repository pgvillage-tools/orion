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
	"encoding/json"
	"net/http"
	"time"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/logging"
)

const readMaxBytes int64 = 1 << 30

func (h *Handlers) ClusterDataRoutes() []Route {
	return []Route{
		{"GET /clusterdata", h.GetClusterDataHandler},
		{"PUT /clusterdata", h.PutClusterDataHandler},
	}
}

// GetClusterDataHandler endpoint
func (h *Handlers) GetClusterDataHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(time.Second))
	defer cancelFunc()

	if cd, _, err := h.client.GetClusterData(ctx); err != nil {
		handleError(ctx, err, w, "failed to get cluster data")
	} else {
		h.writeJSON(ctx, w, cd)
	}
}

// PutClusterDataHandler endpoint
func (h *Handlers) PutClusterDataHandler(w http.ResponseWriter, r *http.Request) {
	ctx, logger := logging.GetLogComponent(r.Context(), logging.WebApiComponent)
	ctx, cancelFunc := context.WithDeadline(ctx, time.Now().Add(time.Second))
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
