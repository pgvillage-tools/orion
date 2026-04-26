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

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"

	"github.com/pgvillage-tools/orion/internal/consensus"
	"github.com/pgvillage-tools/orion/internal/logging"
)

type Route struct {
	Pattern string
	Handler http.HandlerFunc
}

type Handlers struct {
	client consensus.Store
	Ready  *atomic.Bool
	nextID int
}

func NewHandlers(client consensus.Store, ready *atomic.Bool) *Handlers {
	return &Handlers{
		client: client,
		Ready:  ready,
		nextID: 1,
	}
}

func (h *Handlers) Routes() []Route {
	var routes []Route
	routes = append(routes, h.HealthRoutes()...)
	routes = append(routes, h.StatusRoutes()...)
	routes = append(routes, h.UpdateRoutes()...)
	routes = append(routes, h.ClusterDataRoutes()...)
	return routes
}

// Helper method
func (h *Handlers) writeJSON(ctx context.Context, w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
		logger.Error().AnErr("err", err).Msg("error while writing json")
	}
}
