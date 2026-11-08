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
	"mime"
	"net/http"
	"time"

	jsonpatch "github.com/evanphx/json-patch"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/consensus"
	"github.com/pgvillage-tools/orion/internal/logging"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// errNoSpec is the error when there is no cluster spec (yet)
var errNoSpec = errors.New("there is no cluster spec yet: create the cluster first")

// statusError is an error with the status code the api answers it with
type statusError struct {
	status int
	err    error
}

func (e statusError) Error() string { return e.err.Error() }
func (e statusError) Unwrap() error { return e.err }

// writeSpecError responds with the status of err when it is a statusError, and with 500 (and logs it) otherwise
func writeSpecError(ctx context.Context, w http.ResponseWriter, err error, msg string) {
	var se statusError
	if errors.As(err, &se) {
		endpoints.WriteProblem(w, se.status, se.Error())
		return
	}
	handleError(ctx, err, w, msg)
}

// ClusterSpecRoutes adds all Cluster routes to the list of all routes
func (h *Handlers) ClusterSpecRoutes() []Route {
	return []Route{
		{endpoints.ClusterSpecEndPoint.Route(endpoints.MethodGet), h.GetClusterSpecHandler},
		{endpoints.ClusterSpecEndPoint.Route(endpoints.MethodPatch), h.PatchClusterSpecHandler},
		{endpoints.ClusterSpecEndPoint.Route(endpoints.MethodPut), h.PutClusterSpecHandler},
	}
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
	if cd == nil || cd.Cluster == nil || cd.Cluster.Spec == nil {
		endpoints.WriteProblem(w, http.StatusNotFound, errNoSpec.Error())
		return
	}
	setETag(w, etagOfSpec(cd.Cluster.Spec))
	h.writeJSON(ctx, w, cd.Cluster.Spec)
}

// PatchClusterSpecHandler applies a JSON merge patch (RFC 7386, Content-Type application/merge-patch+json) to the
// cluster spec
func (h *Handlers) PatchClusterSpecHandler(w http.ResponseWriter, r *http.Request) {
	ctx, logger := logging.GetLogComponent(r.Context(), logging.WebApiComponent)
	ctx, cancelFunc := context.WithDeadline(ctx, time.Now().Add(time.Second))
	defer cancelFunc()

	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType != endpoints.ContentTypeMergePatch {
		w.Header().Set("Accept-Patch", endpoints.ContentTypeMergePatch)
		endpoints.WriteProblem(w, http.StatusUnsupportedMediaType,
			"the patch must be a JSON merge patch (Content-Type: "+endpoints.ContentTypeMergePatch+")")
		return
	}

	patch, err := io.ReadAll(http.MaxBytesReader(w, r.Body, readMaxBytes))
	if err != nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, "failed to read the body: "+err.Error())
		return
	}
	defer func() {
		err := r.Body.Close()
		if err != nil {
			logger.Error().AnErr("error", err).Msg("Failed to close Body")
		}
	}()

	spec, err := updateSpecRetries(ctx, h.client, patch, false, r)
	if err != nil {
		writeSpecError(ctx, w, err, "failed to patch cluster spec")
		return
	}
	setETag(w, etagOfSpec(spec))
}

// PutClusterSpecHandler endpoint
func (h *Handlers) PutClusterSpecHandler(w http.ResponseWriter, r *http.Request) {
	ctx, logger := logging.GetLogComponent(r.Context(), logging.WebApiComponent)
	ctx, cancelFunc := context.WithDeadline(ctx, time.Now().Add(time.Second))
	defer cancelFunc()

	patch, err := io.ReadAll(http.MaxBytesReader(w, r.Body, readMaxBytes))
	if err != nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, "failed to read the body: "+err.Error())
		return
	}
	defer func() {
		if err := r.Body.Close(); err != nil {
			logger.Error().AnErr("error", err).Msg("Failed to close Body")
		}
	}()

	spec, err := updateSpecRetries(ctx, h.client, patch, true, r)
	if err != nil {
		writeSpecError(ctx, w, err, "failed to put cluster spec")
		return
	}
	setETag(w, etagOfSpec(spec))
}

func patchClusterSpec(cs *apiv1.Spec, p []byte) (*apiv1.Spec, error) {
	csj, err := json.Marshal(cs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cluster spec: %v", err)
	}

	// a JSON merge patch (RFC 7386): objects are merged, null removes a field, everything else replaces it
	newcsj, err := jsonpatch.MergePatch(csj, p)
	if err != nil {
		return nil, fmt.Errorf("failed to merge patch cluster spec: %v", err)
	}
	var newcs *apiv1.Spec
	if err := json.Unmarshal(newcsj, &newcs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal patched cluster spec: %v", err)
	}
	return newcs, nil
}

func tryUpdateSpec(ctx context.Context, client consensus.Store, patch []byte, replace bool,
	r *http.Request) (*apiv1.Spec, error) {
	ctx, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	logger.Debug().Str("patch", string(patch)).Bool("update", replace).Msg("")
	cd, pair, err := client.GetClusterData(ctx)
	if err != nil {
		return nil, err
	}
	if cd == nil || cd.Cluster == nil || cd.Cluster.Spec == nil {
		return nil, statusError{http.StatusNotFound, errNoSpec}
	}
	// the ETag of the spec, not of all cluster data: the sentinel writes the rest of it often
	if !ifMatches(r, etagOfSpec(cd.Cluster.Spec)) {
		return nil, errPreconditionFailed
	}

	logger.Debug().Any("cluster data", cd).Msg("")
	var newcs *apiv1.Spec
	if replace {
		if err = json.Unmarshal(patch, &newcs); err != nil {
			return nil, statusError{http.StatusBadRequest, fmt.Errorf("failed to parse the spec: %w", err)}
		}
		if newcs == nil {
			return nil, statusError{http.StatusBadRequest, errors.New("the spec can't be null")}
		}
		logger.Debug().Any("new cluster spec", newcs).Msg("replaced")
	} else {
		newcs, err = patchClusterSpec(cd.Cluster.Spec, patch)
		if err != nil {
			return nil, statusError{http.StatusBadRequest, err}
		}
		logger.Debug().Any("patched cluster spec", newcs).Msg("patched")
	}
	if err = newcs.Validate(); err != nil {
		return nil, statusError{http.StatusBadRequest, fmt.Errorf("invalid spec: %w", err)}
	}
	// a valid spec that UpdateSpec refuses is a change that isn't allowed (e.g. another init mode)
	if err = cd.Cluster.UpdateSpec(newcs); err != nil {
		return nil, statusError{http.StatusConflict, err}
	}
	logger.Debug().Any("new cluster data", cd).Msg("updated")

	// updateSpecRetries retries when cd has been modified between reading and writing (ErrKeyModified)
	if _, err = client.AtomicPutClusterData(ctx, cd, pair); err != nil {
		return nil, fmt.Errorf("cannot store updated cluster spec: %w", err)
	}
	logger.Debug().Msg("stored")
	return newcs, nil
}

// updateSpecRetries updates the spec, and retries (3 times) when the cluster data changed between reading and writing.
// With an If-Match header in r, it only updates the spec when the spec still has that ETag. It returns the new spec.
func updateSpecRetries(ctx context.Context, client consensus.Store, patch []byte, update bool,
	r *http.Request) (*apiv1.Spec, error) {
	const maxRetries = 3
	var errs []error
	ctx, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	for i := 0; i < maxRetries; i++ {
		logger.Debug().Int("try", i).Msg("trying")
		spec, err := tryUpdateSpec(ctx, client, patch, update, r)
		if err == nil {
			logger.Debug().Int("try", i).Msg("succeeded")
			return spec, nil
		}
		logger.Debug().AnErr("error", err).Msg("failed")
		if !errors.Is(err, consensus.ErrKeyModified) {
			// only a conflicting write is worth another try
			return nil, err
		}
		errs = append(errs, err)
	}
	return nil, fmt.Errorf("failed to update in %d retries: %v", maxRetries, errors.Join(errs...))
}
