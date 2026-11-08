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

	"github.com/pgvillage-tools/orion/internal/consensus"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// FailKeeperRoutes returns the routes to be added for the FailKeeper code
func (h *Handlers) FailKeeperRoutes() []Route {
	return []Route{
		{endpoints.FailKeeperEndPoint.Route(endpoints.MethodPut), h.PutFailKeeperHandler},
	}
}

// PutFailKeeperHandler endpoint
func (h *Handlers) PutFailKeeperHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(time.Second))
	defer cancelFunc()

	keeperID := r.PathValue("id")

	cd, pair, err := h.client.GetClusterData(ctx)
	if err != nil {
		handleError(ctx, err, w, "failed to get cluster data")
		return
	}
	if cd == nil || cd.Cluster == nil || cd.Cluster.Spec == nil {
		endpoints.WriteProblem(w, http.StatusNotFound, "there is no cluster yet")
		return
	}
	newCd := cd.DeepCopy()
	keeperInfo := newCd.Keepers[keeperID]
	if keeperInfo == nil {
		endpoints.WriteProblem(w, http.StatusNotFound, "keeper "+keeperID+" doesn't exist")
		return
	}

	keeperInfo.Status.ForceFail = true

	if _, err = h.client.AtomicPutClusterData(ctx, newCd, pair); err != nil {
		if errors.Is(err, consensus.ErrKeyModified) {
			endpoints.WriteProblem(w, http.StatusConflict,
				"the cluster data changed while failing the keeper; try again")
			return
		}
		handleError(ctx, err, w, "failed to store cluster data")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
