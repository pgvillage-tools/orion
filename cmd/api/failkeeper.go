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

package main

import (
	"context"
	"net/http"
	"time"
)

func (h *Handlers) FailKeeperRoutes() []Route {
	return []Route{
		{"PUT /clusterdata/keepers/{id}", h.PutFailKeeperHandlerHandler},
	}
}

// PutFailKeeperHandlerHandler endpoint
func (h *Handlers) PutFailKeeperHandlerHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(context.TODO(), time.Now().Add(time.Second))
	defer cancelFunc()

	keeperID := r.PathValue("id")

	cd, pair, err := h.client.GetClusterData(ctx)
	if err != nil {
		handleError(ctx, err, w, "failed to get cluster data")
	} else if cd.Cluster == nil {
		handleError(ctx, err, w, "cluster is not set")
	} else if cd.Cluster.Spec == nil {
		handleError(ctx, err, w, "cluster spec is not set")
	}
	newCd := cd.DeepCopy()
	keeperInfo := newCd.Keepers[keeperID]
	if keeperInfo == nil {
		handleError(ctx, err, w, "keeper doesn't exist")
	}

	keeperInfo.Status.ForceFail = true

	_, err = h.client.AtomicPutClusterData(ctx, newCd, pair)
	if err != nil {
		handleError(ctx, err, w, "failed to store cluster data")
	}
	w.WriteHeader(http.StatusAccepted)
}
