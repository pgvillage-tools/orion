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

// KeeperRoutes returns the routes to be added for the RemoveKeepers code
func (h *Handlers) KeeperRoutes() []Route {
	return []Route{
		{endpoints.KeeperEndPoint.Route(endpoints.MethodDelete), h.DeleteKeeperHandler},
	}
}

// DeleteKeeperHandler endpoint
func (h *Handlers) DeleteKeeperHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(time.Second))
	defer cancelFunc()

	cd, pair, err := h.client.GetClusterData(ctx)
	if err != nil {
		handleError(ctx, err, w, "failed to get cluster data")
		return
	} else if cd == nil {
		handleError(ctx, errors.New("cd is nil"), w, "cluster data is not set")
		return
	} else if cd.Cluster == nil {
		handleError(ctx, errors.New("cd.Cluster is nil"), w, "cluster is not set")
		return
	} else if cd.Cluster.Spec == nil {
		handleError(ctx, errors.New("cluster spec is nil"), w, "cluster spec is not set")
		return
	}
	keeperID := r.PathValue("id")
	newCd := cd.DeepCopy()
	if _, exists := newCd.Keepers[keeperID]; !exists {
		handleError(ctx, errors.New("keeper is not known"), w, "failed to delete the keeper")
		return
	}
	delete(newCd.Keepers, keeperID)
	_, err = h.client.AtomicPutClusterData(ctx, newCd, pair)
	if err != nil {
		handleError(ctx, err, w, "failed to delete the keeper")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
