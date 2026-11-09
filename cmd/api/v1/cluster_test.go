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
	"github.com/pgvillage-tools/orion/internal/util"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// newClusterTestHandlers returns handlers with a mock store, and no election (the cluster handlers don't need it)
func newClusterTestHandlers(t *testing.T) (*Handlers, *mockconsensus.MockStore) {
	t.Helper()
	store := mockconsensus.NewMockStore(gomock.NewController(t))
	var ready atomic.Bool
	return NewHandlers(store, nil, &ready), store
}

// checkClusterResponse checks the status of a response, and that an error is a problem+json with that status
func checkClusterResponse(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("expected status %d, got %d: %s", status, w.Code, w.Body)
	}
	if status < http.StatusBadRequest {
		return
	}
	if ct := w.Header().Get("Content-Type"); ct != endpoints.ContentTypeProblem {
		t.Errorf("expected Content-Type %s, got %q", endpoints.ContentTypeProblem, ct)
	}
	var problem endpoints.Problem
	if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
		t.Fatalf("expected a problem, got %s: %v", w.Body, err)
	}
	if problem.Status != status {
		t.Errorf("expected a problem with status %d, got %d", status, problem.Status)
	}
}

// failingBody is a request body that can't be read nor closed
type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("connection reset") }
func (failingBody) Close() error             { return errors.New("already closed") }

// storedData is a gomock matcher that keeps the cluster data written to the store, to check it afterward
type storedData struct{ cd **apiv1.Data }

func (m storedData) Matches(x any) bool {
	cd, ok := x.(*apiv1.Data)
	if ok {
		*m.cd = cd
	}
	return ok
}

func (storedData) String() string { return "is cluster data" }

func TestGetCluster(t *testing.T) {
	errBoom := errors.New("boom")
	cd := apiv1.NewClusterData(apiv1.NewCluster("cluster1", &apiv1.Spec{InitMode: util.ToPtr(apiv1.New)}))

	for name, tc := range map[string]struct {
		cd       *apiv1.Data
		pair     *consensus.KVPair
		storeErr error
		status   int
		etag     string
	}{
		"cluster data returns 200 with its ETag": {
			cd: cd, pair: &consensus.KVPair{LastIndex: 7}, status: http.StatusOK, etag: `"7"`,
		},
		"cluster data without a revision returns 200 without an ETag": {
			cd: cd, status: http.StatusOK,
		},
		"no cluster data yet returns 404": {
			status: http.StatusNotFound,
		},
		"a store failure returns 500": {
			storeErr: errBoom, status: http.StatusInternalServerError,
		},
	} {
		t.Run(name, func(t *testing.T) {
			h, store := newClusterTestHandlers(t)
			store.EXPECT().GetClusterData(gomock.Any()).Return(tc.cd, tc.pair, tc.storeErr)

			w := httptest.NewRecorder()
			h.GetClusterHandler(w, httptest.NewRequest(http.MethodGet, "/v1/cluster", nil))

			checkClusterResponse(t, w, tc.status)
			if got := w.Header().Get("ETag"); got != tc.etag {
				t.Errorf("expected ETag %q, got %q", tc.etag, got)
			}
			if tc.status != http.StatusOK {
				return
			}
			if ct := w.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("expected Content-Type application/json, got %q", ct)
			}
			var got apiv1.Data
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("expected cluster data, got %s: %v", w.Body, err)
			}
			if got.Cluster == nil || got.Cluster.UID != "cluster1" {
				t.Errorf("expected the cluster data of cluster1, got %s", w.Body)
			}
		})
	}
}

func TestPutCluster(t *testing.T) {
	errBoom := errors.New("boom")
	const valid = `{"cluster":{"uid":"cluster2","spec":{"initMode":"new"}}}`
	current := apiv1.NewClusterData(apiv1.NewCluster("cluster1", &apiv1.Spec{InitMode: util.ToPtr(apiv1.New)}))
	pair := &consensus.KVPair{LastIndex: 7}

	for name, tc := range map[string]struct {
		body    string
		ifMatch string
		// expect sets the expected store calls; stored is set to the cluster data written to the store
		expect func(store *mockconsensus.MockStore, stored **apiv1.Data)
		status int
		etag   string
	}{
		"a body that is not JSON returns 400": {
			body: `{`, status: http.StatusBadRequest,
		},
		"a body over 1 MiB returns 400": {
			body:   `{"cluster":{"spec":{"pgHBA":["` + strings.Repeat("a", int(readMaxBytes)) + `"]}}}`,
			status: http.StatusBadRequest,
		},
		"cluster data without a cluster returns 400": {
			body: `{}`, status: http.StatusBadRequest,
		},
		"a cluster without a spec returns 400": {
			body: `{"cluster":{"uid":"cluster2"}}`, status: http.StatusBadRequest,
		},
		"a spec that fails validation returns 400": {
			body: `{"cluster":{"spec":{}}}`, status: http.StatusBadRequest,
		},
		"without If-Match the cluster data is replaced unconditionally": {
			body: valid, status: http.StatusOK,
			expect: func(store *mockconsensus.MockStore, stored **apiv1.Data) {
				store.EXPECT().PutClusterData(gomock.Any(), storedData{stored}).Return(nil)
			},
		},
		"without If-Match a store failure returns 500": {
			body: valid, status: http.StatusInternalServerError,
			expect: func(store *mockconsensus.MockStore, stored **apiv1.Data) {
				store.EXPECT().PutClusterData(gomock.Any(), storedData{stored}).Return(errBoom)
			},
		},
		"with If-Match a failure to read the current cluster data returns 500": {
			body: valid, ifMatch: `"7"`, status: http.StatusInternalServerError,
			expect: func(store *mockconsensus.MockStore, _ **apiv1.Data) {
				store.EXPECT().GetClusterData(gomock.Any()).Return(nil, nil, errBoom)
			},
		},
		"an If-Match that doesn't match the current ETag returns 412": {
			body: valid, ifMatch: `"6"`, status: http.StatusPreconditionFailed,
			expect: func(store *mockconsensus.MockStore, _ **apiv1.Data) {
				store.EXPECT().GetClusterData(gomock.Any()).Return(current, pair, nil)
			},
		},
		"an If-Match without cluster data returns 412": {
			body: valid, ifMatch: "*", status: http.StatusPreconditionFailed,
			expect: func(store *mockconsensus.MockStore, _ **apiv1.Data) {
				store.EXPECT().GetClusterData(gomock.Any()).Return(nil, nil, nil)
			},
		},
		"a matching If-Match replaces the cluster data atomically and returns the new ETag": {
			body: valid, ifMatch: `"7"`, status: http.StatusOK, etag: `"8"`,
			expect: func(store *mockconsensus.MockStore, stored **apiv1.Data) {
				store.EXPECT().GetClusterData(gomock.Any()).Return(current, pair, nil)
				store.EXPECT().AtomicPutClusterData(gomock.Any(), storedData{stored}, pair).
					Return(&consensus.KVPair{LastIndex: 8}, nil)
			},
		},
		"If-Match * replaces existing cluster data": {
			body: valid, ifMatch: "*", status: http.StatusOK, etag: `"8"`,
			expect: func(store *mockconsensus.MockStore, stored **apiv1.Data) {
				store.EXPECT().GetClusterData(gomock.Any()).Return(current, pair, nil)
				store.EXPECT().AtomicPutClusterData(gomock.Any(), storedData{stored}, pair).
					Return(&consensus.KVPair{LastIndex: 8}, nil)
			},
		},
		"a change between reading and writing returns 412": {
			body: valid, ifMatch: `"7"`, status: http.StatusPreconditionFailed,
			expect: func(store *mockconsensus.MockStore, stored **apiv1.Data) {
				store.EXPECT().GetClusterData(gomock.Any()).Return(current, pair, nil)
				store.EXPECT().AtomicPutClusterData(gomock.Any(), storedData{stored}, pair).
					Return(nil, fmt.Errorf("writing: %w", consensus.ErrKeyModified))
			},
		},
		"a failing atomic write returns 500": {
			body: valid, ifMatch: `"7"`, status: http.StatusInternalServerError,
			expect: func(store *mockconsensus.MockStore, stored **apiv1.Data) {
				store.EXPECT().GetClusterData(gomock.Any()).Return(current, pair, nil)
				store.EXPECT().AtomicPutClusterData(gomock.Any(), storedData{stored}, pair).Return(nil, errBoom)
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			h, store := newClusterTestHandlers(t)
			var stored *apiv1.Data
			if tc.expect != nil {
				tc.expect(store, &stored)
			}

			r := httptest.NewRequest(http.MethodPut, "/v1/cluster", strings.NewReader(tc.body))
			if tc.ifMatch != "" {
				r.Header.Set("If-Match", tc.ifMatch)
			}
			w := httptest.NewRecorder()
			h.PutClusterHandler(w, r)

			checkClusterResponse(t, w, tc.status)
			if got := w.Header().Get("ETag"); got != tc.etag {
				t.Errorf("expected ETag %q, got %q", tc.etag, got)
			}
			if stored != nil && (stored.Cluster == nil || stored.Cluster.UID != "cluster2" ||
				stored.Cluster.Spec == nil || *stored.Cluster.Spec.InitMode != apiv1.New) {
				t.Errorf("expected the cluster data of the body to be written, got %+v", stored)
			}
		})
	}
}

func TestPutClusterWithABodyThatFailsToClose(t *testing.T) {
	// the body can't be read (400), and closing it fails too: that is only logged
	h, _ := newClusterTestHandlers(t)
	r := httptest.NewRequest(http.MethodPut, "/v1/cluster", nil)
	r.Body = failingBody{}
	w := httptest.NewRecorder()
	h.PutClusterHandler(w, r)
	checkClusterResponse(t, w, http.StatusBadRequest)
}

func TestPostCluster(t *testing.T) {
	errBoom := errors.New("boom")
	pair := &consensus.KVPair{LastIndex: 7}
	spec := &apiv1.Spec{InitMode: util.ToPtr(apiv1.New)}

	for name, tc := range map[string]struct {
		cd     *apiv1.Data
		pair   *consensus.KVPair
		getErr error
		body   io.Reader
		// writes is whether the handler writes the cluster data, putErr what that write returns
		writes bool
		putErr error
		status int
		// check checks the cluster data written to the store
		check func(t *testing.T, stored *apiv1.Data)
	}{
		"an empty body without cluster data creates a new cluster with the default spec": {
			body: strings.NewReader(""), writes: true, status: http.StatusCreated,
			check: func(t *testing.T, stored *apiv1.Data) {
				if stored.Cluster == nil || stored.Cluster.UID == "" || stored.Cluster.Spec == nil ||
					*stored.Cluster.Spec.InitMode != apiv1.New || stored.Cluster.Status.Phase != apiv1.Initializing {
					t.Errorf("expected a new initializing cluster with initMode new, got %+v", stored.Cluster)
				}
			},
		},
		"a spec on cluster data without a cluster adds a cluster and keeps the rest": {
			cd: &apiv1.Data{Keepers: apiv1.Keepers{"keeper1": {}}}, pair: pair,
			body: strings.NewReader(`{"initMode":"new","maxStandbys":5}`), writes: true, status: http.StatusCreated,
			check: func(t *testing.T, stored *apiv1.Data) {
				if stored.Cluster == nil || stored.Cluster.UID == "" || stored.Cluster.Spec == nil ||
					*stored.Cluster.Spec.MaxStandbys != 5 {
					t.Errorf("expected a new cluster with the spec of the body, got %+v", stored.Cluster)
				}
				if _, ok := stored.Keepers["keeper1"]; !ok {
					t.Errorf("expected the keepers to be kept, got %+v", stored.Keepers)
				}
			},
		},
		"a spec on a cluster without a spec sets its spec and keeps the cluster": {
			cd: apiv1.NewClusterData(apiv1.NewCluster("cluster1", nil)), pair: pair,
			body: strings.NewReader(`{"initMode":"new"}`), writes: true, status: http.StatusCreated,
			check: func(t *testing.T, stored *apiv1.Data) {
				if stored.Cluster.UID != "cluster1" || stored.Cluster.Spec == nil ||
					*stored.Cluster.Spec.InitMode != apiv1.New {
					t.Errorf("expected cluster1 with the spec of the body, got %+v", stored.Cluster)
				}
			},
		},
		"a cluster that already has a spec returns 409": {
			cd: apiv1.NewClusterData(apiv1.NewCluster("cluster1", spec)), pair: pair,
			body: strings.NewReader(""), status: http.StatusConflict,
		},
		"a failure to read the cluster data returns 500": {
			getErr: errBoom, body: strings.NewReader(""), status: http.StatusInternalServerError,
		},
		"a body that can't be read returns 400": {
			body: failingBody{}, status: http.StatusBadRequest,
		},
		"a body that is not JSON returns 400": {
			body: strings.NewReader(`{`), status: http.StatusBadRequest,
		},
		"a null spec returns 400": {
			body: strings.NewReader(`null`), status: http.StatusBadRequest,
		},
		"a spec that fails validation returns 400": {
			body: strings.NewReader(`{}`), status: http.StatusBadRequest,
		},
		"a cluster created concurrently returns 409": {
			body: strings.NewReader(""), writes: true, status: http.StatusConflict,
			putErr: fmt.Errorf("writing: %w", consensus.ErrKeyModified),
		},
		"a failing write returns 500": {
			body: strings.NewReader(""), writes: true, putErr: errBoom, status: http.StatusInternalServerError,
		},
	} {
		t.Run(name, func(t *testing.T) {
			h, store := newClusterTestHandlers(t)
			store.EXPECT().GetClusterData(gomock.Any()).Return(tc.cd, tc.pair, tc.getErr)
			var stored *apiv1.Data
			if tc.writes {
				// the write is conditional on the revision that was read (none when there was no cluster data)
				store.EXPECT().AtomicPutClusterData(gomock.Any(), storedData{&stored}, gomock.Eq(tc.pair)).
					Return(&consensus.KVPair{LastIndex: 8}, tc.putErr)
			}

			w := httptest.NewRecorder()
			h.PostClusterHandler(w, httptest.NewRequest(http.MethodPost, "/v1/cluster", tc.body))

			checkClusterResponse(t, w, tc.status)
			if tc.status != http.StatusCreated {
				return
			}
			if got := w.Header().Get("Location"); got != "/v1/cluster" {
				t.Errorf("expected Location /v1/cluster, got %q", got)
			}
			if ct := w.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("expected Content-Type application/json, got %q", ct)
			}
			tc.check(t, stored)
			var got apiv1.Data
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("expected cluster data, got %s: %v", w.Body, err)
			}
			if got.Cluster == nil || got.Cluster.UID != stored.Cluster.UID {
				t.Errorf("expected the written cluster data in the response, got %s", w.Body)
			}
		})
	}
}
