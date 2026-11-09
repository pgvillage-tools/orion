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
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/golang/mock/gomock"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/consensus"
	mockconsensus "github.com/pgvillage-tools/orion/internal/mock/consensus"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// testSpec returns a valid cluster spec, with work_mem set to 8MB
func testSpec() *apiv1.Spec {
	initMode := apiv1.New
	return &apiv1.Spec{InitMode: &initMode, PGParameters: apiv1.PGParameters{"work_mem": "8MB"}}
}

// testSpecData returns (new) cluster data with spec, or without a cluster when spec is nil
func testSpecData(spec *apiv1.Spec) *apiv1.Data {
	if spec == nil {
		return &apiv1.Data{}
	}
	return &apiv1.Data{Cluster: &apiv1.Cluster{UID: "cluster1", Spec: spec}}
}

// errCloseBody is a request body that fails to close
type errCloseBody struct{ io.Reader }

func (errCloseBody) Close() error { return errors.New("close failed") }

// specCase is a request to one of the cluster spec handlers, with what the store does and what is expected
type specCase struct {
	contentType string
	body        string
	ifMatch     string
	// noCluster makes the store return cluster data without a cluster (spec)
	noCluster bool
	getErr    error
	// gets is the number of times the handler reads the cluster data
	gets int
	// puts are the results of the writes of the cluster data, one per write
	puts   []error
	status int
	// stored checks the spec that is written to the store
	stored func(t *testing.T, spec *apiv1.Spec)
}

// runSpecCase calls handler for tc, and checks its status, its ETag and what it writes to the store
func runSpecCase(t *testing.T, method string, tc specCase, handler func(h *Handlers) http.HandlerFunc) {
	t.Helper()
	store := mockconsensus.NewMockStore(gomock.NewController(t))
	store.EXPECT().GetClusterData(gomock.Any()).DoAndReturn(
		func(context.Context) (*apiv1.Data, *consensus.KVPair, error) {
			if tc.getErr != nil {
				return nil, nil, tc.getErr
			}
			if tc.noCluster {
				return testSpecData(nil), &consensus.KVPair{LastIndex: 1}, nil
			}
			return testSpecData(testSpec()), &consensus.KVPair{LastIndex: 1}, nil
		}).Times(tc.gets)
	var stored *apiv1.Spec
	var calls []*gomock.Call
	for _, putErr := range tc.puts {
		calls = append(calls, store.EXPECT().AtomicPutClusterData(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, cd *apiv1.Data, previous *consensus.KVPair) (*consensus.KVPair, error) {
				if previous == nil || previous.LastIndex != 1 {
					t.Errorf("expected the write to be conditional on the read pair, got %v", previous)
				}
				if cd.Cluster.UID != "cluster1" {
					t.Errorf("expected the rest of the cluster data to be kept, got cluster %q", cd.Cluster.UID)
				}
				stored = cd.Cluster.Spec
				return &consensus.KVPair{LastIndex: 2}, putErr
			}))
	}
	gomock.InOrder(calls...)

	r := httptest.NewRequest(method, "/v1/cluster/spec", strings.NewReader(tc.body))
	if tc.contentType != "" {
		r.Header.Set("Content-Type", tc.contentType)
	}
	if tc.ifMatch != "" {
		r.Header.Set("If-Match", tc.ifMatch)
	}
	w := httptest.NewRecorder()
	var ready atomic.Bool
	handler(NewHandlers(store, mockconsensus.NewMockElection(gomock.NewController(t)), &ready))(w, r)

	if w.Code != tc.status {
		t.Fatalf("expected status %d, got %d: %s", tc.status, w.Code, w.Body)
	}
	if tc.status != http.StatusOK {
		if ct := w.Header().Get("Content-Type"); ct != endpoints.ContentTypeProblem {
			t.Errorf("expected a problem, got Content-Type %q", ct)
		}
		if etag := w.Header().Get("ETag"); etag != "" {
			t.Errorf("expected no ETag on an error, got %s", etag)
		}
		return
	}
	if stored == nil {
		t.Fatal("expected the new spec to be stored")
	}
	if etag := w.Header().Get("ETag"); etag != etagOfSpec(stored) {
		t.Errorf("expected the ETag of the new spec %s, got %s", etagOfSpec(stored), etag)
	}
	if tc.stored != nil {
		tc.stored(t, stored)
	}
}

// workMem checks that the stored spec has work_mem set to value (or unset, when value is "")
func workMem(value string) func(t *testing.T, spec *apiv1.Spec) {
	return func(t *testing.T, spec *apiv1.Spec) {
		t.Helper()
		got, ok := spec.PGParameters["work_mem"]
		if value == "" && ok {
			t.Errorf("expected work_mem to be removed, got %q", got)
		}
		if value != "" && got != value {
			t.Errorf("expected work_mem %q, got %q", value, got)
		}
	}
}

func TestGetClusterSpecHandler(t *testing.T) {
	for name, tc := range map[string]struct {
		cd     *apiv1.Data
		err    error
		status int
	}{
		"the spec":                     {cd: testSpecData(testSpec()), status: http.StatusOK},
		"no cluster data":              {status: http.StatusNotFound},
		"cluster data without cluster": {cd: testSpecData(nil), status: http.StatusNotFound},
		"a cluster without spec": {
			cd: &apiv1.Data{Cluster: &apiv1.Cluster{UID: "cluster1"}}, status: http.StatusNotFound,
		},
		"the store fails": {err: errors.New("boom"), status: http.StatusInternalServerError},
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			store := mockconsensus.NewMockStore(ctrl)
			store.EXPECT().GetClusterData(gomock.Any()).Return(tc.cd, &consensus.KVPair{LastIndex: 1}, tc.err)
			var ready atomic.Bool
			h := NewHandlers(store, mockconsensus.NewMockElection(ctrl), &ready)

			w := httptest.NewRecorder()
			h.GetClusterSpecHandler(w, httptest.NewRequest(http.MethodGet, "/v1/cluster/spec", nil))

			if w.Code != tc.status {
				t.Fatalf("expected status %d, got %d: %s", tc.status, w.Code, w.Body)
			}
			if tc.status != http.StatusOK {
				if ct := w.Header().Get("Content-Type"); ct != endpoints.ContentTypeProblem {
					t.Errorf("expected a problem, got Content-Type %q", ct)
				}
				return
			}
			if ct := w.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("expected Content-Type application/json, got %q", ct)
			}
			if etag := w.Header().Get("ETag"); etag != etagOfSpec(testSpec()) {
				t.Errorf("expected the ETag of the spec %s, got %s", etagOfSpec(testSpec()), etag)
			}
			var spec apiv1.Spec
			if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
				t.Fatalf("expected a spec, got %s: %v", w.Body, err)
			}
			if etagOfSpec(&spec) != etagOfSpec(testSpec()) {
				t.Errorf("expected the spec of the cluster, got %s", w.Body)
			}
		})
	}
}

func TestPatchClusterSpecHandler(t *testing.T) {
	errBoom := errors.New("boom")
	modified := fmt.Errorf("put: %w", consensus.ErrKeyModified)
	tooLarge := `{"pgParameters":{"x":"` + strings.Repeat("x", int(readMaxBytes)) + `"}}`

	for name, tc := range map[string]specCase{
		"a merge patch adds a parameter and keeps the others": {
			contentType: endpoints.ContentTypeMergePatch, body: `{"pgParameters":{"shared_buffers":"1GB"}}`,
			gets: 1, puts: []error{nil}, status: http.StatusOK,
			stored: func(t *testing.T, spec *apiv1.Spec) {
				workMem("8MB")(t, spec)
				if spec.PGParameters["shared_buffers"] != "1GB" {
					t.Errorf("expected shared_buffers 1GB, got %v", spec.PGParameters)
				}
			},
		},
		"a merge patch with parameters in the content type": {
			contentType: endpoints.ContentTypeMergePatch + "; charset=utf-8", body: `{"pgParameters":{"work_mem":"16MB"}}`,
			gets: 1, puts: []error{nil}, status: http.StatusOK, stored: workMem("16MB"),
		},
		"null removes a parameter": {
			contentType: endpoints.ContentTypeMergePatch, body: `{"pgParameters":{"work_mem":null}}`,
			gets: 1, puts: []error{nil}, status: http.StatusOK, stored: workMem(""),
		},
		"the ETag of the spec in If-Match": {
			contentType: endpoints.ContentTypeMergePatch, body: `{"pgParameters":{"work_mem":"16MB"}}`,
			ifMatch: etagOfSpec(testSpec()), gets: 1, puts: []error{nil}, status: http.StatusOK, stored: workMem("16MB"),
		},
		"a stale ETag in If-Match": {
			contentType: endpoints.ContentTypeMergePatch, body: `{"pgParameters":{"work_mem":"16MB"}}`,
			ifMatch: `"stale"`, gets: 1, status: http.StatusPreconditionFailed,
		},
		"a JSON content type": {
			contentType: "application/json", body: `{}`, status: http.StatusUnsupportedMediaType,
		},
		"no content type": {
			body: `{}`, status: http.StatusUnsupportedMediaType,
		},
		"a body that is too large": {
			contentType: endpoints.ContentTypeMergePatch, body: tooLarge, status: http.StatusBadRequest,
		},
		"a patch that is not JSON": {
			contentType: endpoints.ContentTypeMergePatch, body: `{`, gets: 1, status: http.StatusBadRequest,
		},
		"a patch with a value of the wrong type": {
			contentType: endpoints.ContentTypeMergePatch, body: `{"maxStandbys":"many"}`, gets: 1,
			status: http.StatusBadRequest,
		},
		"a patch that makes the spec invalid": {
			contentType: endpoints.ContentTypeMergePatch, body: `{"maxStandbys":0}`, gets: 1,
			status: http.StatusBadRequest,
		},
		"a patch that changes the init mode": {
			contentType: endpoints.ContentTypeMergePatch,
			body:        `{"initMode":"existing","existingConfig":{"keeperUID":"keeper1"}}`,
			gets:        1, status: http.StatusConflict,
		},
		"no cluster": {
			contentType: endpoints.ContentTypeMergePatch, body: `{}`, noCluster: true, gets: 1,
			status: http.StatusNotFound,
		},
		"the cluster data cannot be read": {
			contentType: endpoints.ContentTypeMergePatch, body: `{}`, getErr: errBoom, gets: 1,
			status: http.StatusInternalServerError,
		},
		"the cluster data cannot be written": {
			contentType: endpoints.ContentTypeMergePatch, body: `{}`, gets: 1, puts: []error{errBoom},
			status: http.StatusInternalServerError,
		},
		"the cluster data changed once and the patch is retried": {
			contentType: endpoints.ContentTypeMergePatch, body: `{"pgParameters":{"work_mem":"16MB"}}`,
			gets: 2, puts: []error{modified, nil}, status: http.StatusOK, stored: workMem("16MB"),
		},
		"the cluster data keeps changing": {
			contentType: endpoints.ContentTypeMergePatch, body: `{}`,
			gets: 3, puts: []error{modified, modified, modified}, status: http.StatusInternalServerError,
		},
	} {
		t.Run(name, func(t *testing.T) {
			runSpecCase(t, http.MethodPatch, tc, func(h *Handlers) http.HandlerFunc { return h.PatchClusterSpecHandler })
		})
	}
}

func TestPatchClusterSpecHandlerAcceptPatch(t *testing.T) {
	var ready atomic.Bool
	ctrl := gomock.NewController(t)
	h := NewHandlers(mockconsensus.NewMockStore(ctrl), mockconsensus.NewMockElection(ctrl), &ready)
	r := httptest.NewRequest(http.MethodPatch, "/v1/cluster/spec", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json-patch+json")
	w := httptest.NewRecorder()
	h.PatchClusterSpecHandler(w, r)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected status %d, got %d", http.StatusUnsupportedMediaType, w.Code)
	}
	if got := w.Header().Get("Accept-Patch"); got != endpoints.ContentTypeMergePatch {
		t.Errorf("expected Accept-Patch %s, got %q", endpoints.ContentTypeMergePatch, got)
	}
}

func TestPutClusterSpecHandler(t *testing.T) {
	errBoom := errors.New("boom")
	modified := fmt.Errorf("put: %w", consensus.ErrKeyModified)
	tooLarge := `{"pgParameters":{"x":"` + strings.Repeat("x", int(readMaxBytes)) + `"}}`

	for name, tc := range map[string]specCase{
		"a new spec replaces the spec": {
			body: `{"initMode":"new","pgParameters":{"shared_buffers":"1GB"}}`,
			gets: 1, puts: []error{nil}, status: http.StatusOK,
			stored: func(t *testing.T, spec *apiv1.Spec) {
				workMem("")(t, spec)
				if spec.PGParameters["shared_buffers"] != "1GB" {
					t.Errorf("expected shared_buffers 1GB, got %v", spec.PGParameters)
				}
			},
		},
		"the ETag of the spec in If-Match": {
			body: `{"initMode":"new"}`, ifMatch: etagOfSpec(testSpec()),
			gets: 1, puts: []error{nil}, status: http.StatusOK, stored: workMem(""),
		},
		"any spec in If-Match": {
			body: `{"initMode":"new"}`, ifMatch: "*", gets: 1, puts: []error{nil}, status: http.StatusOK,
		},
		"a stale ETag in If-Match": {
			body: `{"initMode":"new"}`, ifMatch: `"stale"`, gets: 1, status: http.StatusPreconditionFailed,
		},
		"any spec in If-Match without a cluster": {
			body: `{"initMode":"new"}`, ifMatch: "*", noCluster: true, gets: 1, status: http.StatusNotFound,
		},
		"a body that is too large": {
			body: tooLarge, status: http.StatusBadRequest,
		},
		"a spec that is not JSON": {
			body: `{`, gets: 1, status: http.StatusBadRequest,
		},
		"a null spec": {
			body: `null`, gets: 1, status: http.StatusBadRequest,
		},
		"a spec without init mode": {
			body: `{"pgParameters":{"work_mem":"16MB"}}`, gets: 1, status: http.StatusBadRequest,
		},
		"a spec with another init mode": {
			body: `{"initMode":"existing","existingConfig":{"keeperUID":"keeper1"}}`, gets: 1,
			status: http.StatusConflict,
		},
		"no cluster": {
			body: `{"initMode":"new"}`, noCluster: true, gets: 1, status: http.StatusNotFound,
		},
		"the cluster data cannot be read": {
			body: `{"initMode":"new"}`, getErr: errBoom, gets: 1, status: http.StatusInternalServerError,
		},
		"the cluster data cannot be written": {
			body: `{"initMode":"new"}`, gets: 1, puts: []error{errBoom}, status: http.StatusInternalServerError,
		},
		"the cluster data changed twice and the spec is retried": {
			body: `{"initMode":"new"}`, gets: 3, puts: []error{modified, modified, nil}, status: http.StatusOK,
			stored: workMem(""),
		},
		"the cluster data keeps changing": {
			body: `{"initMode":"new"}`, gets: 3, puts: []error{modified, modified, modified},
			status: http.StatusInternalServerError,
		},
	} {
		t.Run(name, func(t *testing.T) {
			runSpecCase(t, http.MethodPut, tc, func(h *Handlers) http.HandlerFunc { return h.PutClusterSpecHandler })
		})
	}
}

func TestClusterSpecHandlersLogABodyThatFailsToClose(t *testing.T) {
	for name, tc := range map[string]struct {
		method      string
		contentType string
		handler     func(h *Handlers) http.HandlerFunc
	}{
		"PATCH": {http.MethodPatch, endpoints.ContentTypeMergePatch,
			func(h *Handlers) http.HandlerFunc { return h.PatchClusterSpecHandler }},
		"PUT": {http.MethodPut, "application/json",
			func(h *Handlers) http.HandlerFunc { return h.PutClusterSpecHandler }},
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			store := mockconsensus.NewMockStore(ctrl)
			store.EXPECT().GetClusterData(gomock.Any()).Return(testSpecData(testSpec()), &consensus.KVPair{}, nil)
			store.EXPECT().AtomicPutClusterData(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)
			var ready atomic.Bool
			h := NewHandlers(store, mockconsensus.NewMockElection(ctrl), &ready)

			r := httptest.NewRequest(tc.method, "/v1/cluster/spec", nil)
			r.Body = errCloseBody{strings.NewReader(`{"initMode":"new"}`)}
			r.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()
			tc.handler(h)(w, r)
			// failing to close the body is only logged: the spec is updated
			if w.Code != http.StatusOK {
				t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body)
			}
		})
	}
}

func TestPatchClusterSpec(t *testing.T) {
	for name, tc := range map[string]struct {
		patch   string
		wantErr bool
		check   func(t *testing.T, spec *apiv1.Spec)
	}{
		"objects are merged": {
			patch: `{"pgParameters":{"shared_buffers":"1GB"}}`,
			check: func(t *testing.T, spec *apiv1.Spec) {
				workMem("8MB")(t, spec)
				if spec.PGParameters["shared_buffers"] != "1GB" {
					t.Errorf("expected shared_buffers 1GB, got %v", spec.PGParameters)
				}
			},
		},
		"null removes a field":           {patch: `{"pgParameters":null}`, check: workMem("")},
		"other values replace the field": {patch: `{"pgParameters":{"work_mem":"16MB"}}`, check: workMem("16MB")},
		"a patch that is not JSON":       {patch: `{`, wantErr: true},
		"a value of the wrong type":      {patch: `{"maxStandbys":-1}`, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			spec, err := patchClusterSpec(testSpec(), []byte(tc.patch))
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected an error, got spec %v", spec)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.check(t, spec)
		})
	}
}

func TestUpdateSpecRetriesStopsAtOtherErrors(t *testing.T) {
	errBoom := errors.New("boom")
	store := mockconsensus.NewMockStore(gomock.NewController(t))
	store.EXPECT().GetClusterData(gomock.Any()).Return(nil, nil, errBoom).Times(1)
	r := httptest.NewRequest(http.MethodPut, "/v1/cluster/spec", nil)
	if _, err := updateSpecRetries(context.Background(), store, []byte(`{}`), true, r); !errors.Is(err, errBoom) {
		t.Errorf("expected %v, got %v", errBoom, err)
	}
}

func TestUpdateSpecRetriesReportsEveryConflict(t *testing.T) {
	store := mockconsensus.NewMockStore(gomock.NewController(t))
	store.EXPECT().GetClusterData(gomock.Any()).DoAndReturn(
		func(context.Context) (*apiv1.Data, *consensus.KVPair, error) {
			return testSpecData(testSpec()), &consensus.KVPair{}, nil
		}).Times(3)
	store.EXPECT().AtomicPutClusterData(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, consensus.ErrKeyModified).Times(3)
	r := httptest.NewRequest(http.MethodPut, "/v1/cluster/spec", nil)
	_, err := updateSpecRetries(context.Background(), store, []byte(`{"initMode":"new"}`), true, r)
	if err == nil || !strings.Contains(err.Error(), "3 retries") {
		t.Fatalf("expected an error after 3 retries, got %v", err)
	}
	if got := strings.Count(err.Error(), consensus.ErrKeyModified.Error()); got != 3 {
		t.Errorf("expected the 3 conflicts in the error, got %d: %v", got, err)
	}
}

func TestTryUpdateSpecErrors(t *testing.T) {
	store := mockconsensus.NewMockStore(gomock.NewController(t))
	store.EXPECT().GetClusterData(gomock.Any()).Return(testSpecData(testSpec()), &consensus.KVPair{}, nil)
	r := httptest.NewRequest(http.MethodPatch, "/v1/cluster/spec", nil)
	_, err := tryUpdateSpec(context.Background(), store, []byte(`{"maxStandbys":0}`), false, r)
	var se statusError
	if !errors.As(err, &se) || se.status != http.StatusBadRequest {
		t.Fatalf("expected a %d status error, got %v", http.StatusBadRequest, err)
	}
	if !strings.HasPrefix(err.Error(), "invalid spec: ") {
		t.Errorf("expected an invalid spec error, got %v", err)
	}
}

func TestStatusError(t *testing.T) {
	err := statusError{http.StatusNotFound, errNoSpec}
	if err.Error() != errNoSpec.Error() {
		t.Errorf("expected the message of the wrapped error, got %q", err.Error())
	}
	if !errors.Is(err, errNoSpec) {
		t.Error("expected the wrapped error to be unwrapped")
	}
	wrapped := fmt.Errorf("wrapped: %w", err)
	var se statusError
	if !errors.As(wrapped, &se) || se.status != http.StatusNotFound {
		t.Errorf("expected the status error in a wrapped error, got %v", wrapped)
	}
}

func TestWriteSpecError(t *testing.T) {
	for name, tc := range map[string]struct {
		err    error
		status int
		detail string
	}{
		"a status error":         {statusError{http.StatusConflict, errors.New("no")}, http.StatusConflict, "no"},
		"a wrapped status error": {fmt.Errorf("w: %w", errPreconditionFailed), http.StatusPreconditionFailed, ""},
		"another error":          {errors.New("secret"), http.StatusInternalServerError, "failed to update"},
	} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeSpecError(context.Background(), w, tc.err, "failed to update")
			if w.Code != tc.status {
				t.Errorf("expected status %d, got %d", tc.status, w.Code)
			}
			if ct := w.Header().Get("Content-Type"); ct != endpoints.ContentTypeProblem {
				t.Errorf("expected a problem, got Content-Type %q", ct)
			}
			var problem endpoints.Problem
			if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
				t.Fatalf("expected a problem, got %s: %v", w.Body, err)
			}
			if tc.detail != "" && problem.Detail != tc.detail {
				t.Errorf("expected detail %q, got %q", tc.detail, problem.Detail)
			}
		})
	}
}
