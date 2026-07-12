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
	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
	"github.com/pgvillage-tools/orion/internal/util"
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
	} else if cd.Cluster == nil {
		handleError(ctx, errors.New(""), w, "cluster is not set")
		return
	} else if cd.Cluster.Spec == nil {
		handleError(ctx, err, w, "cluster spec is not set")
		return
	}
	newCd := cd.DeepCopy()

	if *newCd.Cluster.Spec.Role == apiv1.Primary {
		return
	}

	newCd.Cluster.Spec.Role = util.ToPtr(apiv1.Primary)
	_, err = h.client.AtomicPutClusterData(ctx, newCd, pair)
	if err != nil {
		handleError(ctx, err, w, "failed to promote this replicaset")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
