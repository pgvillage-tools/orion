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

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/consensus"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// FailKeeperRoutes returns the routes to be added for the FailKeeper code
func (h *Handlers) FailKeeperRoutes() []Route {
	return []Route{
		{endpoints.FailKeeperEndPoint.Route(endpoints.MethodPost), h.FailKeeperHandler},
	}
}

// FailKeeperHandler marks a keeper as failed (POST .../keepers/{id}/actions/fail), so the sentinel fails over when it
// runs the primary
func (h *Handlers) FailKeeperHandler(w http.ResponseWriter, r *http.Request) {
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

	// The sentinel treats a failed keeper as failed for one check only: when no other keeper can take over the primary
	// in that check, the primary stays. So refuse to fail the primary when nothing can take over.
	if !canFailOver(newCd, keeperInfo) {
		endpoints.WriteProblem(w, http.StatusConflict, "keeper "+keeperID+" runs the primary, and no other keeper "+
			"can take over now (it needs a healthy keeper with a healthy, converged database on the same timeline)")
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

// canFailOver returns whether failing keeper can lead to a failover: true when keeper doesn't run the primary, or
// when another keeper could take over (it is healthy, and its database is healthy, converged and on the timeline of
// the primary). It is a pre-check of what the sentinel requires, not all of it (e.g. the lag and can-be-master).
func canFailOver(cd *apiv1.Data, keeper *apiv1.Keeper) bool {
	if cd.Cluster == nil {
		return true
	}
	primary, ok := cd.DBs[cd.Cluster.Status.Master]
	if !ok || primary == nil || primary.Spec == nil || primary.Spec.KeeperUID != keeper.UID {
		// not the primary (or no primary at all): nothing to fail over
		return true
	}
	for _, db := range cd.DBs {
		if db == nil || db.Spec == nil || db.UID == primary.UID {
			continue
		}
		k, ok := cd.Keepers[db.Spec.KeeperUID]
		if !ok || k == nil || !k.Status.Healthy {
			continue
		}
		if db.Status.Healthy && db.Status.CurrentGeneration == db.Generation &&
			db.Status.TimelineID == primary.Status.TimelineID {
			return true
		}
	}
	return false
}
