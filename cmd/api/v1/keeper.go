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
	"fmt"
	"net/http"
	"time"

	"github.com/pgvillage-tools/orion/internal/consensus"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// KeeperRoutes returns the routes to be added for the RemoveKeepers code
func (h *Handlers) KeeperRoutes() []Route {
	return []Route{
		{endpoints.KeeperEndPoint.Route(endpoints.MethodDelete), h.DeleteKeeperHandler},
	}
}

// DeleteKeeperHandler removes a keeper, and its db, from the cluster data. It is meant for a keeper that is gone for
// good: a keeper that still runs registers itself again, and the sentinel adds it back. The keeper of the primary
// can't be removed (409): fail it first, and remove it once another keeper runs the primary.
func (h *Handlers) DeleteKeeperHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(time.Second))
	defer cancelFunc()

	cd, pair, err := h.client.GetClusterData(ctx)
	if err != nil {
		handleError(ctx, err, w, "failed to get cluster data")
		return
	}
	if cd == nil || cd.Cluster == nil || cd.Cluster.Spec == nil {
		endpoints.WriteProblem(w, http.StatusNotFound, "there is no cluster yet")
		return
	}
	keeperID := r.PathValue("id")
	newCd := cd.DeepCopy()
	keeper, exists := newCd.Keepers[keeperID]
	if !exists {
		endpoints.WriteProblem(w, http.StatusNotFound, "keeper "+keeperID+" doesn't exist")
		return
	}
	if db := newCd.FindDB(keeper); db != nil {
		if db.UID == newCd.Cluster.Status.Master {
			endpoints.WriteProblem(w, http.StatusConflict, fmt.Sprintf("keeper %[1]s runs the primary: fail it first "+
				"(POST /v1/cluster/keepers/%[1]s/actions/fail), and remove it once another keeper runs the primary",
				keeperID))
			return
		}
		delete(newCd.DBs, db.UID)
	}
	delete(newCd.Keepers, keeperID)
	if _, err = h.client.AtomicPutClusterData(ctx, newCd, pair); err != nil {
		if errors.Is(err, consensus.ErrKeyModified) {
			endpoints.WriteProblem(w, http.StatusConflict,
				"the cluster data changed while removing the keeper; try again")
			return
		}
		handleError(ctx, err, w, "failed to delete the keeper")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
