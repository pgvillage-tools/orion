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
	"errors"
	"net/http"
	"time"

	"github.com/pgvillage-tools/orion/internal/auth"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// ProbeRoutes collect and return all routes of the probe port. metrics serves /metrics.
func (h *Handlers) ProbeRoutes(metrics http.Handler) []Route {
	return []Route{
		{endpoints.HealthEndPoint.Route(endpoints.MethodGet), h.HealthzHandler},
		{endpoints.ReadyEndPoint.Route(endpoints.MethodGet), h.ReadyzHandler},
		{endpoints.ModeEndPoint.Route(endpoints.MethodGet), h.ModeHandler},
		{endpoints.MetricsEndPoint.Route(endpoints.MethodGet), metrics.ServeHTTP},
	}
}

// HealthzHandler handles Health requests (incl consensus roundtrip)
func (h *Handlers) HealthzHandler(w http.ResponseWriter, r *http.Request) {
	var (
		status = http.StatusOK
		msg    = []byte("OK")
	)
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(time.Second))
	defer cancelFunc()

	if err := h.client.Healthy(ctx); err != nil {
		handleError(ctx, err, w, "store is not healthy")
		return
	}
	w.WriteHeader(status)
	if _, err := w.Write(msg); err != nil {
		handleError(ctx, err, w, "unable to return healthz response")
	}
}

// ReadyzHandler handles ready requests (excl consensus roundtrip)
func (h *Handlers) ReadyzHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.Ready.Load() {
		handleError(ctx, errors.New("Service not ready yet"), w, "Not Ready")
		return
	}

	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("Ready"))

	if err != nil {
		handleError(ctx, err, w, "unable to return readyz response")
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
