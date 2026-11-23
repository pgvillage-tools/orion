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

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/consensus"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// Route defines a combination of a pattern and the func to handle the request
type Route struct {
	Pattern string
	Handler http.HandlerFunc
}

// Handlers is a convenience struct for all required to handle api calls
type Handlers struct {
	client   consensus.Store
	election consensus.Election
	Ready    *atomic.Bool
	nextID   int
	// storeTimeout is the deadline of the store calls of a request
	storeTimeout time.Duration
	// bootstrapping is true while the api port is in bootstrap mode
	bootstrapping atomic.Bool
	// bootstrap holds the bootstrap secret and the sessions in bootstrap mode; nil in normal mode
	bootstrap *bootstrapper
	// bootstrapDone is closed (by finishBootstrap) when the api switches from bootstrap mode to normal mode
	bootstrapDone chan struct{}
	bootstrapOnce sync.Once
}

// NewHandlers returns a new Handlers object. election is only used to read the sentinel leader; the api never runs for
// election itself.
func NewHandlers(client consensus.Store, election consensus.Election, ready *atomic.Bool) *Handlers {
	return &Handlers{
		client:   client,
		election: election,
		Ready:    ready,
		nextID:   1,

		storeTimeout:  cluster.DefaultStoreTimeout,
		bootstrapDone: make(chan struct{}),
	}
}

// Routes collects and returns all routes of the api port (see ProbeRoutes for the probe port)
func (h *Handlers) Routes() []Route {
	var routes []Route
	routes = append(routes, h.ClusterRoutes()...)
	routes = append(routes, h.ClusterSpecRoutes()...)
	routes = append(routes, h.KeeperRoutes()...)
	routes = append(routes, h.FailKeeperRoutes()...)
	routes = append(routes, h.PromoteReplicaSetRoutes()...)
	routes = append(routes, h.StatusRoutes()...)
	routes = append(routes, h.MemberRoutes()...)
	routes = append(routes, h.WhoAmIRoutes()...)
	return routes
}

// Helper method
func (h *Handlers) writeJSON(ctx context.Context, w http.ResponseWriter, data interface{}) {
	h.writeJSONStatus(ctx, w, http.StatusOK, data)
}

// writeJSONStatus responds with status and data as JSON
func (h *Handlers) writeJSONStatus(ctx context.Context, w http.ResponseWriter, status int, data interface{}) {
	var buf bytes.Buffer

	if err := json.NewEncoder(&buf).Encode(data); err != nil {
		handleError(ctx, err, w, "error while encoding json")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(buf.Bytes()); err != nil {
		// the status is sent already, so only log it (the client is gone, most likely)
		handleError(ctx, err, nil, "error while writing json")
	}
}

// writeAccepted answers 202 Accepted for a change the sentinel applies after the request: the Location header points
// to the cluster status, which shows when it is done (e.g. orion-cli --wait polls it)
func writeAccepted(w http.ResponseWriter) {
	w.Header().Set("Location", "/"+endpoints.APIVersion+"/"+string(endpoints.StatusEndPoint))
	w.WriteHeader(http.StatusAccepted)
}
