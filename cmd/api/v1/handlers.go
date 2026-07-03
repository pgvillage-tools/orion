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
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"

	"github.com/pgvillage-tools/orion/internal/consensus"
)

// Route defines a combination of a pattern and the func to handle the request
type Route struct {
	Pattern string
	Handler http.HandlerFunc
}

// Handlers is a convenience struct for all required to handle api calls
type Handlers struct {
	client consensus.Store
	Ready  *atomic.Bool
	nextID int
}

// NewHandlers returns a new Handlers object
func NewHandlers(client consensus.Store, ready *atomic.Bool) *Handlers {
	return &Handlers{
		client: client,
		Ready:  ready,
		nextID: 1,
	}
}

// Routes collects and returns all routes for this handler
func (h *Handlers) Routes() []Route {
	var routes []Route
	routes = append(routes, h.ClusterDataRoutes()...)
	routes = append(routes, h.FailKeeperRoutes()...)
	routes = append(routes, h.HealthRoutes()...)
	routes = append(routes, h.InitRoutes()...)
	routes = append(routes, h.StatusRoutes()...)
	routes = append(routes, h.UpdateRoutes()...)
	return routes
}

// Helper method
func (h *Handlers) writeJSON(ctx context.Context, w http.ResponseWriter, data interface{}) {
	var buf bytes.Buffer

	if err := json.NewEncoder(&buf).Encode(data); err != nil {
		handleError(ctx, err, w, "error while encoding json")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(buf.Bytes()); err != nil {
		handleError(ctx, err, w, "error while writing json")
	}
}
