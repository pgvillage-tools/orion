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

package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/logging"
)

const (
	// InitEndPoint defines the endpoint for initialization
	InitEndPoint EndPoint = "clusterdata/spec"
)

// InitRoutes collect and return all Init Routes
func (h *Handlers) InitRoutes() []Route {
	return []Route{
		{InitEndPoint.Route(MethodPost), h.PostInitHandler},
	}
}

// PostInitHandler endpoint
func (h *Handlers) PostInitHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(time.Second))
	defer cancelFunc()

	cd, _, err := h.client.GetClusterData(ctx)
	if err != nil {
		handleError(ctx, err, w, "failed to get cluster data")
	} else if cd == nil {
		_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
		logger.Debug().Msg("clusterdata is nil")
	} else if cd.Cluster != nil {
		handleError(ctx, err, w, "cluster is already set")
	}

	var cs *apiv1.Spec

	newCSBytes, err := io.ReadAll(r.Body)
	if err != nil {
		handleError(ctx, err, w, "failed to get spec definition")
		return
	}
	defer func() {
		if err := r.Body.Close(); err != nil {
			handleError(ctx, err, w, "Failed to close Body")
		}
	}()
	if len(newCSBytes) == 0 {
		cs = &apiv1.Spec{}
		newCluster := apiv1.New
		cs.InitMode = &newCluster
	} else {
		err = json.Unmarshal(newCSBytes, &cs)
		if err != nil {
			handleError(ctx, err, w, "failed to parse spec definition")
			return
		}
	}

	if err := cs.Validate(); err != nil {
		handleError(ctx, err, w, "invalid spec")
		return
	}

	c := apiv1.NewCluster(common.UID(), cs)
	cd = apiv1.NewClusterData(c)

	// We ignore if cd has been modified between reading and writing
	if err := h.client.PutClusterData(ctx, cd); err != nil {
		handleError(ctx, err, w, "cannot update spec")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
