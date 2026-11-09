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
	"github.com/pgvillage-tools/orion/internal/util"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// PromoteReplicaSetRoutes returns the routes to be added for the PromoteReplicaSet code
func (h *Handlers) PromoteReplicaSetRoutes() []Route {
	return []Route{
		{endpoints.PromoteClusterEndPoint.Route(endpoints.MethodPost), h.PromoteClusterHandler},
	}
}

// PromoteClusterHandler promotes a standby cluster to a primary cluster (POST /v1/cluster/actions/promote); a primary
// cluster stays as it is (200)
func (h *Handlers) PromoteClusterHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(cfg.StoreTimeout))
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
	// without a role in the spec, the cluster has the default role (primary)
	if *cd.Cluster.DefSpec().Role == apiv1.Primary {
		// nothing to do
		w.WriteHeader(http.StatusOK)
		return
	}

	newCd := cd.DeepCopy()
	newCd.Cluster.Spec.Role = util.ToPtr(apiv1.Primary)
	if _, err = h.client.AtomicPutClusterData(ctx, newCd, pair); err != nil {
		if errors.Is(err, consensus.ErrKeyModified) {
			endpoints.WriteProblem(w, http.StatusConflict,
				"the cluster data changed while promoting the cluster; try again")
			return
		}
		handleError(ctx, err, w, "failed to promote this replicaset")
		return
	}
	writeAccepted(w)
}
