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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/consensus"
	"github.com/pgvillage-tools/orion/internal/logging"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

// ClusterRoutes adds all Cluster routes to the list of all routes
func (h *Handlers) ClusterSpecRoutes() []Route {
	return []Route{
		{endpoints.ClusterEndPoint.Route(endpoints.MethodPost), h.PostClusterSpecHandler},
		{endpoints.ClusterSpecEndPoint.Route(endpoints.MethodGet), h.GetClusterSpecHandler},
		{endpoints.ClusterSpecEndPoint.Route(endpoints.MethodPatch), h.PatchClusterSpecHandler},
		{endpoints.ClusterSpecEndPoint.Route(endpoints.MethodPut), h.PutClusterSpecHandler},
	}
}

/*
// GetClusterHandler endpoint
func (h *Handlers) GetClusterSpecHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(cfg.StoreTimeout))
	defer cancelFunc()

	if cd, _, err := h.client.GetClusterData(ctx); err != nil {
		handleError(ctx, err, w, "failed to get cluster data")
	} else {
		if cd == nil {
			w.WriteHeader(http.StatusNotFound)
		} else {
			h.writeJSON(ctx, w, cd)
		}
	}
}

// PutClusterHandler endpoint
func (h *Handlers) PutClusterSpecHandler(w http.ResponseWriter, r *http.Request) {
	ctx, logger := logging.GetLogComponent(r.Context(), logging.WebApiComponent)
	ctx, cancelFunc := context.WithDeadline(ctx, time.Now().Add(cfg.StoreTimeout))
	defer cancelFunc()

	reader := http.MaxBytesReader(w, r.Body, readMaxBytes)
	defer func() {
		if err := reader.Close(); err != nil {
			logger.Error().AnErr("error", err).Msg("Failed to close Body")
		}
	}()
	var newCD apiv1.Data
	err := json.NewDecoder(reader).Decode(&newCD)
	if err != nil {
		handleError(ctx, err, w, "failed to parse new cluster data definition")
		return
	}
	_, err = h.client.AtomicPutClusterData(ctx, &newCD, nil)
	if err != nil {
		handleError(ctx, err, w, "failed to write new cluster data definition")
	}
}
*/

// PostClusterHandler endpoint is used by init
func (h *Handlers) PostClusterSpecHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(time.Second))
	defer cancelFunc()
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)

	cd, _, err := h.client.GetClusterData(ctx)
	if err != nil {
		handleError(ctx, err, w, "failed to get cluster data")
	} else if cd == nil {
		logger.Debug().Msg("Clusterdata is nil")
	} else if cd.Cluster == nil {
		logger.Debug().Msg("Cluster is nil")
	} else if cd.Cluster.Spec != nil {
		handleError(ctx, err, w, "cluster spec is already set")
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

	if cd == nil {
		c := apiv1.NewCluster(common.UID(), cs)
		cd = apiv1.NewClusterData(c)
	} else if cd.Cluster == nil {
		cd.Cluster = apiv1.NewCluster(common.UID(), cs)
	} else {
		cd.Cluster.Spec = cs
	}

	// We ignore if cd has been modified between reading and writing
	if err := h.client.PutClusterData(ctx, cd); err != nil {
		handleError(ctx, err, w, "cannot update spec")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// GetClusterSpecHandler endpoint
func (h *Handlers) GetClusterSpecHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(time.Second))
	defer cancelFunc()

	cd, _, err := h.client.GetClusterData(ctx)
	if err != nil {
		handleError(ctx, err, w, "failed to get cluster")
		return
	}
	if cd == nil {
		handleError(ctx, err, w, "no clusterdata")
		return
	}
	if cd.Cluster == nil {
		handleError(ctx, err, w, "no cluster available")
		return
	}
	if cd.Cluster.Spec == nil {
		handleError(ctx, err, w, "no cluster spec available")
		return
	}
	h.writeJSON(ctx, w, cd.Cluster.Spec)
}

// PatchClusterSpecHandler endpoint
func (h *Handlers) PatchClusterSpecHandler(w http.ResponseWriter, r *http.Request) {
	ctx, logger := logging.GetLogComponent(r.Context(), logging.WebApiComponent)
	ctx, cancelFunc := context.WithDeadline(ctx, time.Now().Add(time.Second))
	defer cancelFunc()

	patch, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer func() {
		err := r.Body.Close()
		if err != nil {
			logger.Error().AnErr("error", err).Msg("Failed to close Body")
		}
	}()

	err = updateSpecRetries(ctx, h.client, patch, false)
	if err != nil {
		handleError(ctx, err, w, "failed to patch cluster spec")
		return
	}
}

// PutClusterSpecHandler endpoint
func (h *Handlers) PutClusterSpecHandler(w http.ResponseWriter, r *http.Request) {
	ctx, logger := logging.GetLogComponent(r.Context(), logging.WebApiComponent)
	ctx, cancelFunc := context.WithDeadline(ctx, time.Now().Add(time.Second))
	defer cancelFunc()

	patch, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer func() {
		if err := r.Body.Close(); err != nil {
			logger.Error().AnErr("error", err).Msg("Failed to close Body")
		}
	}()

	err = updateSpecRetries(ctx, h.client, patch, true)
	if err != nil {
		handleError(ctx, err, w, "failed to put cluster spec")
		return
	}
}

func patchClusterSpec(cs *apiv1.Spec, p []byte) (*apiv1.Spec, error) {
	csj, err := json.Marshal(cs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cluster spec: %v", err)
	}

	newcsj, err := strategicpatch.StrategicMergePatch(csj, p, &apiv1.Spec{})
	if err != nil {
		return nil, fmt.Errorf("failed to merge patch cluster spec: %v", err)
	}
	var newcs *apiv1.Spec
	if err := json.Unmarshal(newcsj, &newcs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal patched cluster spec: %v", err)
	}
	return newcs, nil
}

func tryUpdateSpec(ctx context.Context, client consensus.Store, patch []byte, replace bool) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	logger.Debug().Str("patch", string(patch)).Bool("update", replace).Msg("")
	cd, pair, err := client.GetClusterData(ctx)
	if err != nil {
		return err
	}
	if cd == nil {
		return errors.New("no cluster data available")
	}
	if cd.Cluster == nil {
		return errors.New("no cluster available")
	}
	if cd.Cluster.Spec == nil {
		return errors.New("no cluster spec available")
	}

	logger.Debug().Any("cluster data", cd).Msg("")
	var newcs *apiv1.Spec
	if replace {
		if err = json.Unmarshal(patch, &newcs); err != nil {
			return fmt.Errorf("failed to unmarshal cluster spec: %v", err)
		}
		if newcs == nil {
			return errors.New("cluster spec cannot be null")
		}
		logger.Debug().Any("new cluster spec", newcs).Msg("replaced")
	} else {
		newcs, err = patchClusterSpec(cd.Cluster.Spec, patch)
		if err != nil {
			return fmt.Errorf("failed to patch cluster spec: %v", err)
		}
		logger.Debug().Any("patched cluster spec", newcs).Msg("patched")
	}
	if err = cd.Cluster.UpdateSpec(newcs); err != nil {
		return fmt.Errorf("cannot update cluster spec: %v", err)
	}
	logger.Debug().Any("new cluster data", cd).Msg("updated")

	// retry if cd has been modified between reading and writing
	_, err = client.AtomicPutClusterData(ctx, cd, pair)
	if err != nil {
		return fmt.Errorf("cannot store updated cluster spec: %v", err)
	}
	logger.Debug().Msg("stored")
	return nil
}

func updateSpecRetries(ctx context.Context, client consensus.Store, patch []byte, update bool) error {
	const maxRetries = 3
	var errs []error
	ctx, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	for i := 0; i < maxRetries; i++ {
		logger.Debug().Int("try", i).Msg("trying")
		err := tryUpdateSpec(ctx, client, patch, update)
		if err == nil {
			logger.Debug().Int("try", i).Msg("succeeded")
			return nil
		}
		logger.Debug().AnErr("error", err).Msg("failed")
		errs = append(errs, err)
	}
	return fmt.Errorf("failed to update in %d retries: %v", maxRetries, errors.Join(errs...))
}
