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

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/consensus"
	mockconsensus "github.com/pgvillage-tools/orion/internal/mock/consensus"
	"github.com/pgvillage-tools/orion/internal/util"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

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

var _ = Describe("/v1/cluster", func() {
	// newClusterTestHandlers returns handlers with a mock store, and no election (the cluster handlers don't need it)
	newClusterTestHandlers := func() (*Handlers, *mockconsensus.MockStore) {
		store := mockconsensus.NewMockStore(gomock.NewController(GinkgoT()))
		var ready atomic.Bool
		return NewHandlers(store, nil, &ready), store
	}

	// checkClusterResponse checks the status of a response, and that an error is a problem+json with that status
	checkClusterResponse := func(w *httptest.ResponseRecorder, status int) {
		GinkgoHelper()
		Expect(w.Code).To(Equal(status), "body: %s", w.Body)
		if status < http.StatusBadRequest {
			return
		}
		Expect(w.Header().Get("Content-Type")).To(Equal(endpoints.ContentTypeProblem))
		var problem endpoints.Problem
		Expect(json.Unmarshal(w.Body.Bytes(), &problem)).To(Succeed(), "expected a problem, got %s", w.Body)
		Expect(problem.Status).To(Equal(status), "expected a problem with status %d", status)
	}

	Describe("GET /v1/cluster", func() {
		errBoom := errors.New("boom")
		cd := apiv1.NewClusterData(apiv1.NewCluster("cluster1", &apiv1.Spec{InitMode: util.ToPtr(apiv1.New)}))

		DescribeTable("GetClusterHandler",
			func(cd *apiv1.Data, pair *consensus.KVPair, storeErr error, status int, etag string) {
				h, store := newClusterTestHandlers()
				store.EXPECT().GetClusterData(gomock.Any()).Return(cd, pair, storeErr)

				w := httptest.NewRecorder()
				h.GetClusterHandler(w, httptest.NewRequest(http.MethodGet, "/v1/cluster", nil))

				checkClusterResponse(w, status)
				Expect(w.Header().Get("ETag")).To(Equal(etag))
				if status != http.StatusOK {
					return
				}
				Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))
				var got apiv1.Data
				Expect(json.Unmarshal(w.Body.Bytes(), &got)).To(Succeed(), "expected cluster data, got %s", w.Body)
				Expect(got.Cluster).NotTo(BeNil(), "expected the cluster data of cluster1, got %s", w.Body)
				Expect(got.Cluster.UID).To(Equal("cluster1"))
			},
			Entry("cluster data returns 200 with its ETag",
				cd, &consensus.KVPair{LastIndex: 7}, nil, http.StatusOK, `"7"`),
			Entry("cluster data without a revision returns 200 without an ETag",
				cd, nil, nil, http.StatusOK, ""),
			Entry("no cluster data yet returns 404",
				nil, nil, nil, http.StatusNotFound, ""),
			Entry("a store failure returns 500",
				nil, nil, errBoom, http.StatusInternalServerError, ""),
		)
	})

	Describe("PUT /v1/cluster", func() {
		errBoom := errors.New("boom")
		const valid = `{"cluster":{"uid":"cluster2","spec":{"initMode":"new"}}}`
		current := apiv1.NewClusterData(apiv1.NewCluster("cluster1", &apiv1.Spec{InitMode: util.ToPtr(apiv1.New)}))
		pair := &consensus.KVPair{LastIndex: 7}

		// putCase is a PUT of body with ifMatch; expect sets the expected store calls (stored is set to the cluster
		// data written to the store)
		type putCase struct {
			body    string
			ifMatch string
			expect  func(store *mockconsensus.MockStore, stored **apiv1.Data)
			status  int
			etag    string
		}

		DescribeTable("PutClusterHandler",
			func(tc putCase) {
				h, store := newClusterTestHandlers()
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

				checkClusterResponse(w, tc.status)
				Expect(w.Header().Get("ETag")).To(Equal(tc.etag))
				if stored != nil {
					Expect(stored.Cluster).NotTo(BeNil(), "expected the cluster data of the body to be written")
					Expect(stored.Cluster.UID).To(Equal("cluster2"))
					Expect(stored.Cluster.Spec).NotTo(BeNil())
					Expect(*stored.Cluster.Spec.InitMode).To(Equal(apiv1.New))
				}
			},
			Entry("a body that is not JSON returns 400", putCase{
				body: `{`, status: http.StatusBadRequest,
			}),
			Entry("a body over 1 MiB returns 400", putCase{
				body:   `{"cluster":{"spec":{"pgHBA":["` + strings.Repeat("a", int(readMaxBytes)) + `"]}}}`,
				status: http.StatusBadRequest,
			}),
			Entry("cluster data without a cluster returns 400", putCase{
				body: `{}`, status: http.StatusBadRequest,
			}),
			Entry("a cluster without a spec returns 400", putCase{
				body: `{"cluster":{"uid":"cluster2"}}`, status: http.StatusBadRequest,
			}),
			Entry("a spec that fails validation returns 400", putCase{
				body: `{"cluster":{"spec":{}}}`, status: http.StatusBadRequest,
			}),
			Entry("without If-Match the cluster data is replaced unconditionally", putCase{
				body: valid, status: http.StatusOK,
				expect: func(store *mockconsensus.MockStore, stored **apiv1.Data) {
					store.EXPECT().PutClusterData(gomock.Any(), storedData{stored}).Return(nil)
				},
			}),
			Entry("without If-Match a store failure returns 500", putCase{
				body: valid, status: http.StatusInternalServerError,
				expect: func(store *mockconsensus.MockStore, stored **apiv1.Data) {
					store.EXPECT().PutClusterData(gomock.Any(), storedData{stored}).Return(errBoom)
				},
			}),
			Entry("with If-Match a failure to read the current cluster data returns 500", putCase{
				body: valid, ifMatch: `"7"`, status: http.StatusInternalServerError,
				expect: func(store *mockconsensus.MockStore, _ **apiv1.Data) {
					store.EXPECT().GetClusterData(gomock.Any()).Return(nil, nil, errBoom)
				},
			}),
			Entry("an If-Match that doesn't match the current ETag returns 412", putCase{
				body: valid, ifMatch: `"6"`, status: http.StatusPreconditionFailed,
				expect: func(store *mockconsensus.MockStore, _ **apiv1.Data) {
					store.EXPECT().GetClusterData(gomock.Any()).Return(current, pair, nil)
				},
			}),
			Entry("an If-Match without cluster data returns 412", putCase{
				body: valid, ifMatch: "*", status: http.StatusPreconditionFailed,
				expect: func(store *mockconsensus.MockStore, _ **apiv1.Data) {
					store.EXPECT().GetClusterData(gomock.Any()).Return(nil, nil, nil)
				},
			}),
			Entry("a matching If-Match replaces the cluster data atomically and returns the new ETag", putCase{
				body: valid, ifMatch: `"7"`, status: http.StatusOK, etag: `"8"`,
				expect: func(store *mockconsensus.MockStore, stored **apiv1.Data) {
					store.EXPECT().GetClusterData(gomock.Any()).Return(current, pair, nil)
					store.EXPECT().AtomicPutClusterData(gomock.Any(), storedData{stored}, pair).
						Return(&consensus.KVPair{LastIndex: 8}, nil)
				},
			}),
			Entry("If-Match * replaces existing cluster data", putCase{
				body: valid, ifMatch: "*", status: http.StatusOK, etag: `"8"`,
				expect: func(store *mockconsensus.MockStore, stored **apiv1.Data) {
					store.EXPECT().GetClusterData(gomock.Any()).Return(current, pair, nil)
					store.EXPECT().AtomicPutClusterData(gomock.Any(), storedData{stored}, pair).
						Return(&consensus.KVPair{LastIndex: 8}, nil)
				},
			}),
			Entry("a change between reading and writing returns 412", putCase{
				body: valid, ifMatch: `"7"`, status: http.StatusPreconditionFailed,
				expect: func(store *mockconsensus.MockStore, stored **apiv1.Data) {
					store.EXPECT().GetClusterData(gomock.Any()).Return(current, pair, nil)
					store.EXPECT().AtomicPutClusterData(gomock.Any(), storedData{stored}, pair).
						Return(nil, fmt.Errorf("writing: %w", consensus.ErrKeyModified))
				},
			}),
			Entry("a failing atomic write returns 500", putCase{
				body: valid, ifMatch: `"7"`, status: http.StatusInternalServerError,
				expect: func(store *mockconsensus.MockStore, stored **apiv1.Data) {
					store.EXPECT().GetClusterData(gomock.Any()).Return(current, pair, nil)
					store.EXPECT().AtomicPutClusterData(gomock.Any(), storedData{stored}, pair).Return(nil, errBoom)
				},
			}),
		)

		It("returns 400 for a body that can't be read and fails to close", func() {
			// the body can't be read (400), and closing it fails too: that is only logged
			h, _ := newClusterTestHandlers()
			r := httptest.NewRequest(http.MethodPut, "/v1/cluster", nil)
			r.Body = failingBody{}
			w := httptest.NewRecorder()
			h.PutClusterHandler(w, r)
			checkClusterResponse(w, http.StatusBadRequest)
		})
	})

	Describe("POST /v1/cluster", func() {
		errBoom := errors.New("boom")
		pair := &consensus.KVPair{LastIndex: 7}
		spec := &apiv1.Spec{InitMode: util.ToPtr(apiv1.New)}

		type postCase struct {
			cd     *apiv1.Data
			pair   *consensus.KVPair
			getErr error
			// body returns the request body (a new one for every run of the spec)
			body func() io.Reader
			// writes is whether the handler writes the cluster data, putErr what that write returns
			writes bool
			putErr error
			status int
			// check checks the cluster data written to the store
			check func(stored *apiv1.Data)
		}
		text := func(s string) func() io.Reader { return func() io.Reader { return strings.NewReader(s) } }

		DescribeTable("PostClusterHandler",
			func(tc postCase) {
				h, store := newClusterTestHandlers()
				store.EXPECT().GetClusterData(gomock.Any()).Return(tc.cd, tc.pair, tc.getErr)
				var stored *apiv1.Data
				if tc.writes {
					// the write is conditional on the revision that was read (none when there was no cluster data)
					store.EXPECT().AtomicPutClusterData(gomock.Any(), storedData{&stored}, gomock.Eq(tc.pair)).
						Return(&consensus.KVPair{LastIndex: 8}, tc.putErr)
				}

				w := httptest.NewRecorder()
				h.PostClusterHandler(w, httptest.NewRequest(http.MethodPost, "/v1/cluster", tc.body()))

				checkClusterResponse(w, tc.status)
				if tc.status != http.StatusCreated {
					return
				}
				Expect(w.Header().Get("Location")).To(Equal("/v1/cluster"))
				Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))
				tc.check(stored)
				var got apiv1.Data
				Expect(json.Unmarshal(w.Body.Bytes(), &got)).To(Succeed(), "expected cluster data, got %s", w.Body)
				Expect(got.Cluster).NotTo(BeNil(), "expected the written cluster data in the response, got %s", w.Body)
				Expect(got.Cluster.UID).To(Equal(stored.Cluster.UID))
			},
			Entry("an empty body without cluster data creates a new cluster with the default spec", postCase{
				body: text(""), writes: true, status: http.StatusCreated,
				check: func(stored *apiv1.Data) {
					Expect(stored.Cluster).NotTo(BeNil(), "expected a new initializing cluster with initMode new")
					Expect(stored.Cluster.UID).NotTo(BeEmpty())
					Expect(stored.Cluster.Spec).NotTo(BeNil())
					Expect(*stored.Cluster.Spec.InitMode).To(Equal(apiv1.New))
					Expect(stored.Cluster.Status.Phase).To(Equal(apiv1.Initializing))
				},
			}),
			Entry("a spec on cluster data without a cluster adds a cluster and keeps the rest", postCase{
				cd: &apiv1.Data{Keepers: apiv1.Keepers{"keeper1": {}}}, pair: pair,
				body: text(`{"initMode":"new","maxStandbys":5}`), writes: true, status: http.StatusCreated,
				check: func(stored *apiv1.Data) {
					Expect(stored.Cluster).NotTo(BeNil(), "expected a new cluster with the spec of the body")
					Expect(stored.Cluster.UID).NotTo(BeEmpty())
					Expect(stored.Cluster.Spec).NotTo(BeNil())
					Expect(*stored.Cluster.Spec.MaxStandbys).To(BeEquivalentTo(5))
					Expect(stored.Keepers).To(HaveKey("keeper1"), "expected the keepers to be kept")
				},
			}),
			Entry("a spec on a cluster without a spec sets its spec and keeps the cluster", postCase{
				cd: apiv1.NewClusterData(apiv1.NewCluster("cluster1", nil)), pair: pair,
				body: text(`{"initMode":"new"}`), writes: true, status: http.StatusCreated,
				check: func(stored *apiv1.Data) {
					Expect(stored.Cluster.UID).To(Equal("cluster1"), "expected cluster1 with the spec of the body")
					Expect(stored.Cluster.Spec).NotTo(BeNil())
					Expect(*stored.Cluster.Spec.InitMode).To(Equal(apiv1.New))
				},
			}),
			Entry("a cluster that already has a spec returns 409", postCase{
				cd: apiv1.NewClusterData(apiv1.NewCluster("cluster1", spec)), pair: pair,
				body: text(""), status: http.StatusConflict,
			}),
			Entry("a failure to read the cluster data returns 500", postCase{
				getErr: errBoom, body: text(""), status: http.StatusInternalServerError,
			}),
			Entry("a body that can't be read returns 400", postCase{
				body: func() io.Reader { return failingBody{} }, status: http.StatusBadRequest,
			}),
			Entry("a body that is not JSON returns 400", postCase{
				body: text(`{`), status: http.StatusBadRequest,
			}),
			Entry("a null spec returns 400", postCase{
				body: text(`null`), status: http.StatusBadRequest,
			}),
			Entry("a spec that fails validation returns 400", postCase{
				body: text(`{}`), status: http.StatusBadRequest,
			}),
			Entry("a cluster created concurrently returns 409", postCase{
				body: text(""), writes: true, status: http.StatusConflict,
				putErr: fmt.Errorf("writing: %w", consensus.ErrKeyModified),
			}),
			Entry("a failing write returns 500", postCase{
				body: text(""), writes: true, putErr: errBoom, status: http.StatusInternalServerError,
			}),
		)
	})
})
