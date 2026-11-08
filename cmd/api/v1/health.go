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
	"net/http"
	"time"

	"github.com/pgvillage-tools/orion/internal/auth"
	"github.com/pgvillage-tools/orion/internal/logging"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// storeCheckTimeout is the time the store gets to answer the readiness check
const storeCheckTimeout = time.Second

// ProbeRoutes collect and return all routes of the probe port. metrics serves /metrics.
func (h *Handlers) ProbeRoutes(metrics http.Handler) []Route {
	return []Route{
		{endpoints.LiveEndPoint.Route(endpoints.MethodGet), h.LivezHandler},
		{endpoints.ReadyEndPoint.Route(endpoints.MethodGet), h.ReadyzHandler},
		{endpoints.ModeEndPoint.Route(endpoints.MethodGet), h.ModeHandler},
		{endpoints.MetricsEndPoint.Route(endpoints.MethodGet), metrics.ServeHTTP},
	}
}

// LivezHandler handles liveness requests: 200 as long as the process serves requests. It doesn't check the store, so
// a store outage doesn't make Kubernetes restart the api (which wouldn't fix the store).
func (h *Handlers) LivezHandler(w http.ResponseWriter, r *http.Request) {
	writeProbe(r.Context(), w, http.StatusOK, "OK")
}

// ReadyzHandler handles readiness requests: 200 when the api serves, isn't shutting down and its store answers within
// storeCheckTimeout; 503 otherwise, so Kubernetes and load balancers stop sending requests to it.
func (h *Handlers) ReadyzHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithTimeout(r.Context(), storeCheckTimeout)
	defer cancelFunc()
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)

	if !h.Ready.Load() {
		writeProbe(ctx, w, http.StatusServiceUnavailable, "Not Ready")
		return
	}
	if err := h.client.Healthy(ctx); err != nil {
		logger.Warn().AnErr("error", err).Msg("not ready: the store doesn't answer")
		writeProbe(ctx, w, http.StatusServiceUnavailable, "Not Ready")
		return
	}
	writeProbe(ctx, w, http.StatusOK, "Ready")
}

// writeProbe writes the response of a probe
func writeProbe(ctx context.Context, w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	if _, err := w.Write([]byte(msg)); err != nil {
		_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
		logger.Error().AnErr("error", err).Msg("unable to write the probe response")
	}
}

// ModeHandler returns the mode of the api
func (h *Handlers) ModeHandler(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(r.Context(), w, endpoints.ModeResponse{Mode: endpoints.ModeNormal})
}

// WhoAmIRoutes collect and return the whoami route
func (h *Handlers) WhoAmIRoutes() []Route {
	return []Route{
		{endpoints.WhoAmIEndPoint.Route(endpoints.MethodGet), h.WhoAmIHandler},
	}
}

// WhoAmIHandler returns the identity of the client
func (h *Handlers) WhoAmIHandler(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	h.writeJSON(r.Context(), w, endpoints.WhoAmIResponse{
		Name:   identity.Name,
		Groups: identity.Groups,
		Method: identity.Method,
	})
}
