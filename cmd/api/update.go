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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/consensus"
	"github.com/pgvillage-tools/orion/internal/logging"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

func (h *Handlers) UpdateRoutes() []Route {
	return []Route{
		{"GET /cluster/spec", h.clusterSpecGetHandler},
		{"PATCH /cluster/spec", h.clusterSpecPatchHandler},
		{"PUT /cluster/spec", h.clusterSpecPutHandler},
	}
}

// clusterSpecGetHandler endpoint
func (h *Handlers) clusterSpecGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx, logger := logging.GetLogComponent(r.Context(), logging.WebApiComponent)
	ctx, cancelFunc := context.WithDeadline(ctx, time.Now().Add(time.Second))
	defer cancelFunc()

	cd, _, err := h.client.GetClusterData(ctx)
	if err != nil {
		logger.Error().AnErr("error", err).Msg("failed to get cluster")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if cd.Cluster == nil {
		logger.Error().Msg("no cluster available")
		http.Error(w, "no cluster available", http.StatusInternalServerError)
		return
	}
	if cd.Cluster.Spec == nil {
		logger.Error().Msg("no cluster spec available")
		http.Error(w, "no cluster spec available", http.StatusInternalServerError)
		return
	}
	h.writeJSON(ctx, w, cd.Cluster.Spec)
}

// clusterSpecPatchHandler endpoint
func (h *Handlers) clusterSpecPatchHandler(w http.ResponseWriter, r *http.Request) {
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
		logger.Error().AnErr("error", err).Msg("failed to patch cluster spec")
		if _, err := w.Write([]byte("ERROR")); err != nil {
			logger.Error().AnErr("error", err).Msg("unable to return cluster data")
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

// clusterSpecPutHandler endpoint
func (h *Handlers) clusterSpecPutHandler(w http.ResponseWriter, r *http.Request) {
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
		logger.Error().AnErr("error", err).Msg("failed to put cluster spec")
		if _, err := w.Write([]byte("ERROR")); err != nil {
			logger.Error().AnErr("error", err).Msg("unable to return cluster data")
		}
		w.WriteHeader(http.StatusInternalServerError)
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
		return fmt.Errorf("Cannot update cluster spec: %v", err)
	}
	logger.Debug().Any("new cluster data", cd).Msg("updated")

	// retry if cd has been modified between reading and writing
	_, err = client.AtomicPutClusterData(ctx, cd, pair)
	if err != nil {
		return fmt.Errorf("Cannot store updated cluster spec: %v", err)
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
