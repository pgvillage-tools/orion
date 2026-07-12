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

	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
)

const ()

// HealthRoutes collect and return all health routes
func (h *Handlers) HealthRoutes() []Route {
	return []Route{
		{endpoints.HealthEndPoint.Route(endpoints.MethodGet), h.HealthzHandler},
		{endpoints.ReadyEndPoint.Route(endpoints.MethodGet), h.ReadyzHandler},
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
