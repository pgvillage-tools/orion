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
	"context"
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
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// errCloseBody is a request body that fails to close
type errCloseBody struct{ io.Reader }

func (errCloseBody) Close() error { return errors.New("close failed") }

var _ = Describe("/v1/cluster/spec", func() {
	// testSpec returns a valid cluster spec, with work_mem set to 8MB
	testSpec := func() *apiv1.Spec {
		initMode := apiv1.New
		return &apiv1.Spec{InitMode: &initMode, PGParameters: apiv1.PGParameters{"work_mem": "8MB"}}
	}

	// testSpecData returns (new) cluster data with spec, or without a cluster when spec is nil
	testSpecData := func(spec *apiv1.Spec) *apiv1.Data {
		if spec == nil {
			return &apiv1.Data{}
		}
		return &apiv1.Data{Cluster: &apiv1.Cluster{UID: "cluster1", Spec: spec}}
	}

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
		stored func(spec *apiv1.Spec)
	}

	// runSpecCase calls handler for tc, and checks its status, its ETag and what it writes to the store
	runSpecCase := func(method string, tc specCase, handler func(h *Handlers) http.HandlerFunc) {
		GinkgoHelper()
		store := mockconsensus.NewMockStore(gomock.NewController(GinkgoT()))
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
			put := store.EXPECT().AtomicPutClusterData(gomock.Any(), gomock.Any(), gomock.Any())
			calls = append(calls, put.DoAndReturn(
				func(_ context.Context, cd *apiv1.Data, previous *consensus.KVPair) (*consensus.KVPair, error) {
					Expect(previous).NotTo(BeNil(), "expected the write to be conditional on the read pair")
					Expect(previous.LastIndex).To(BeEquivalentTo(1),
						"expected the write to be conditional on the read pair")
					Expect(cd.Cluster.UID).To(Equal("cluster1"), "expected the rest of the cluster data to be kept")
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
		handler(NewHandlers(store, mockconsensus.NewMockElection(gomock.NewController(GinkgoT())), &ready))(w, r)

		Expect(w.Code).To(Equal(tc.status), "body: %s", w.Body)
		if tc.status != http.StatusOK {
			Expect(w.Header().Get("Content-Type")).To(Equal(endpoints.ContentTypeProblem), "expected a problem")
			Expect(w.Header().Get("ETag")).To(BeEmpty(), "expected no ETag on an error")
			return
		}
		Expect(stored).NotTo(BeNil(), "expected the new spec to be stored")
		Expect(w.Header().Get("ETag")).To(Equal(etagOfSpec(stored)), "expected the ETag of the new spec")
		if tc.stored != nil {
			tc.stored(stored)
		}
	}

	// workMem checks that the stored spec has work_mem set to value (or unset, when value is "")
	workMem := func(value string) func(spec *apiv1.Spec) {
		return func(spec *apiv1.Spec) {
			GinkgoHelper()
			if value == "" {
				Expect(spec.PGParameters).NotTo(HaveKey("work_mem"), "expected work_mem to be removed")
				return
			}
			Expect(spec.PGParameters["work_mem"]).To(Equal(value))
		}
	}

	Describe("GET /v1/cluster/spec", func() {
		DescribeTable("GetClusterSpecHandler",
			func(cd *apiv1.Data, err error, status int) {
				ctrl := gomock.NewController(GinkgoT())
				store := mockconsensus.NewMockStore(ctrl)
				store.EXPECT().GetClusterData(gomock.Any()).Return(cd, &consensus.KVPair{LastIndex: 1}, err)
				var ready atomic.Bool
				h := NewHandlers(store, mockconsensus.NewMockElection(ctrl), &ready)

				w := httptest.NewRecorder()
				h.GetClusterSpecHandler(w, httptest.NewRequest(http.MethodGet, "/v1/cluster/spec", nil))

				Expect(w.Code).To(Equal(status), "body: %s", w.Body)
				if status != http.StatusOK {
					Expect(w.Header().Get("Content-Type")).To(Equal(endpoints.ContentTypeProblem), "expected a problem")
					return
				}
				Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))
				Expect(w.Header().Get("ETag")).To(Equal(etagOfSpec(testSpec())), "expected the ETag of the spec")
				var spec apiv1.Spec
				Expect(json.Unmarshal(w.Body.Bytes(), &spec)).To(Succeed(), "expected a spec, got %s", w.Body)
				Expect(etagOfSpec(&spec)).To(Equal(etagOfSpec(testSpec())), "expected the spec of the cluster, got %s",
					w.Body)
			},
			Entry("the spec", testSpecData(testSpec()), nil, http.StatusOK),
			Entry("no cluster data", nil, nil, http.StatusNotFound),
			Entry("cluster data without cluster", testSpecData(nil), nil, http.StatusNotFound),
			Entry("a cluster without spec", &apiv1.Data{Cluster: &apiv1.Cluster{UID: "cluster1"}}, nil,
				http.StatusNotFound),
			Entry("the store fails", nil, errors.New("boom"), http.StatusInternalServerError),
		)
	})

	Describe("PATCH /v1/cluster/spec", func() {
		errBoom := errors.New("boom")
		modified := fmt.Errorf("put: %w", consensus.ErrKeyModified)
		tooLarge := `{"pgParameters":{"x":"` + strings.Repeat("x", int(readMaxBytes)) + `"}}`

		DescribeTable("PatchClusterSpecHandler",
			func(tc specCase) {
				runSpecCase(http.MethodPatch, tc,
					func(h *Handlers) http.HandlerFunc { return h.PatchClusterSpecHandler })
			},
			Entry("a merge patch adds a parameter and keeps the others", specCase{
				contentType: endpoints.ContentTypeMergePatch, body: `{"pgParameters":{"shared_buffers":"1GB"}}`,
				gets: 1, puts: []error{nil}, status: http.StatusOK,
				stored: func(spec *apiv1.Spec) {
					workMem("8MB")(spec)
					Expect(spec.PGParameters["shared_buffers"]).To(Equal("1GB"))
				},
			}),
			Entry("a merge patch with parameters in the content type", specCase{
				contentType: endpoints.ContentTypeMergePatch + "; charset=utf-8",
				body:        `{"pgParameters":{"work_mem":"16MB"}}`,
				gets:        1, puts: []error{nil}, status: http.StatusOK, stored: workMem("16MB"),
			}),
			Entry("null removes a parameter", specCase{
				contentType: endpoints.ContentTypeMergePatch, body: `{"pgParameters":{"work_mem":null}}`,
				gets: 1, puts: []error{nil}, status: http.StatusOK, stored: workMem(""),
			}),
			Entry("the ETag of the spec in If-Match", specCase{
				contentType: endpoints.ContentTypeMergePatch, body: `{"pgParameters":{"work_mem":"16MB"}}`,
				ifMatch: etagOfSpec(testSpec()), gets: 1, puts: []error{nil}, status: http.StatusOK,
				stored: workMem("16MB"),
			}),
			Entry("a stale ETag in If-Match", specCase{
				contentType: endpoints.ContentTypeMergePatch, body: `{"pgParameters":{"work_mem":"16MB"}}`,
				ifMatch: `"stale"`, gets: 1, status: http.StatusPreconditionFailed,
			}),
			Entry("a JSON content type", specCase{
				contentType: "application/json", body: `{}`, status: http.StatusUnsupportedMediaType,
			}),
			Entry("no content type", specCase{
				body: `{}`, status: http.StatusUnsupportedMediaType,
			}),
			Entry("a body that is too large", specCase{
				contentType: endpoints.ContentTypeMergePatch, body: tooLarge, status: http.StatusBadRequest,
			}),
			Entry("a patch that is not JSON", specCase{
				contentType: endpoints.ContentTypeMergePatch, body: `{`, gets: 1, status: http.StatusBadRequest,
			}),
			Entry("a patch with a value of the wrong type", specCase{
				contentType: endpoints.ContentTypeMergePatch, body: `{"maxStandbys":"many"}`, gets: 1,
				status: http.StatusBadRequest,
			}),
			Entry("a patch that makes the spec invalid", specCase{
				contentType: endpoints.ContentTypeMergePatch, body: `{"maxStandbys":0}`, gets: 1,
				status: http.StatusBadRequest,
			}),
			Entry("a patch that changes the init mode", specCase{
				contentType: endpoints.ContentTypeMergePatch,
				body:        `{"initMode":"existing","existingConfig":{"keeperUID":"keeper1"}}`,
				gets:        1, status: http.StatusConflict,
			}),
			Entry("no cluster", specCase{
				contentType: endpoints.ContentTypeMergePatch, body: `{}`, noCluster: true, gets: 1,
				status: http.StatusNotFound,
			}),
			Entry("the cluster data cannot be read", specCase{
				contentType: endpoints.ContentTypeMergePatch, body: `{}`, getErr: errBoom, gets: 1,
				status: http.StatusInternalServerError,
			}),
			Entry("the cluster data cannot be written", specCase{
				contentType: endpoints.ContentTypeMergePatch, body: `{}`, gets: 1, puts: []error{errBoom},
				status: http.StatusInternalServerError,
			}),
			Entry("the cluster data changed once and the patch is retried", specCase{
				contentType: endpoints.ContentTypeMergePatch, body: `{"pgParameters":{"work_mem":"16MB"}}`,
				gets: 2, puts: []error{modified, nil}, status: http.StatusOK, stored: workMem("16MB"),
			}),
			Entry("the cluster data keeps changing", specCase{
				contentType: endpoints.ContentTypeMergePatch, body: `{}`,
				gets: 3, puts: []error{modified, modified, modified}, status: http.StatusInternalServerError,
			}),
		)

		It("answers a JSON patch with the patch formats it accepts", func() {
			var ready atomic.Bool
			ctrl := gomock.NewController(GinkgoT())
			h := NewHandlers(mockconsensus.NewMockStore(ctrl), mockconsensus.NewMockElection(ctrl), &ready)
			r := httptest.NewRequest(http.MethodPatch, "/v1/cluster/spec", strings.NewReader(`{}`))
			r.Header.Set("Content-Type", "application/json-patch+json")
			w := httptest.NewRecorder()
			h.PatchClusterSpecHandler(w, r)
			Expect(w.Code).To(Equal(http.StatusUnsupportedMediaType))
			Expect(w.Header().Get("Accept-Patch")).To(Equal(endpoints.ContentTypeMergePatch))
		})
	})

	Describe("PUT /v1/cluster/spec", func() {
		errBoom := errors.New("boom")
		modified := fmt.Errorf("put: %w", consensus.ErrKeyModified)
		tooLarge := `{"pgParameters":{"x":"` + strings.Repeat("x", int(readMaxBytes)) + `"}}`

		DescribeTable("PutClusterSpecHandler",
			func(tc specCase) {
				runSpecCase(http.MethodPut, tc, func(h *Handlers) http.HandlerFunc { return h.PutClusterSpecHandler })
			},
			Entry("a new spec replaces the spec", specCase{
				body: `{"initMode":"new","pgParameters":{"shared_buffers":"1GB"}}`,
				gets: 1, puts: []error{nil}, status: http.StatusOK,
				stored: func(spec *apiv1.Spec) {
					workMem("")(spec)
					Expect(spec.PGParameters["shared_buffers"]).To(Equal("1GB"))
				},
			}),
			Entry("the ETag of the spec in If-Match", specCase{
				body: `{"initMode":"new"}`, ifMatch: etagOfSpec(testSpec()),
				gets: 1, puts: []error{nil}, status: http.StatusOK, stored: workMem(""),
			}),
			Entry("any spec in If-Match", specCase{
				body: `{"initMode":"new"}`, ifMatch: "*", gets: 1, puts: []error{nil}, status: http.StatusOK,
			}),
			Entry("a stale ETag in If-Match", specCase{
				body: `{"initMode":"new"}`, ifMatch: `"stale"`, gets: 1, status: http.StatusPreconditionFailed,
			}),
			Entry("any spec in If-Match without a cluster", specCase{
				body: `{"initMode":"new"}`, ifMatch: "*", noCluster: true, gets: 1, status: http.StatusNotFound,
			}),
			Entry("a body that is too large", specCase{
				body: tooLarge, status: http.StatusBadRequest,
			}),
			Entry("a spec that is not JSON", specCase{
				body: `{`, gets: 1, status: http.StatusBadRequest,
			}),
			Entry("a null spec", specCase{
				body: `null`, gets: 1, status: http.StatusBadRequest,
			}),
			Entry("a spec without init mode", specCase{
				body: `{"pgParameters":{"work_mem":"16MB"}}`, gets: 1, status: http.StatusBadRequest,
			}),
			Entry("a spec with another init mode", specCase{
				body: `{"initMode":"existing","existingConfig":{"keeperUID":"keeper1"}}`, gets: 1,
				status: http.StatusConflict,
			}),
			Entry("no cluster", specCase{
				body: `{"initMode":"new"}`, noCluster: true, gets: 1, status: http.StatusNotFound,
			}),
			Entry("the cluster data cannot be read", specCase{
				body: `{"initMode":"new"}`, getErr: errBoom, gets: 1, status: http.StatusInternalServerError,
			}),
			Entry("the cluster data cannot be written", specCase{
				body: `{"initMode":"new"}`, gets: 1, puts: []error{errBoom}, status: http.StatusInternalServerError,
			}),
			Entry("the cluster data changed twice and the spec is retried", specCase{
				body: `{"initMode":"new"}`, gets: 3, puts: []error{modified, modified, nil}, status: http.StatusOK,
				stored: workMem(""),
			}),
			Entry("the cluster data keeps changing", specCase{
				body: `{"initMode":"new"}`, gets: 3, puts: []error{modified, modified, modified},
				status: http.StatusInternalServerError,
			}),
		)
	})

	Describe("the cluster spec handlers", func() {
		DescribeTable("log a body that fails to close",
			func(method, contentType string, handler func(h *Handlers) http.HandlerFunc) {
				ctrl := gomock.NewController(GinkgoT())
				store := mockconsensus.NewMockStore(ctrl)
				store.EXPECT().GetClusterData(gomock.Any()).Return(testSpecData(testSpec()), &consensus.KVPair{}, nil)
				store.EXPECT().AtomicPutClusterData(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)
				var ready atomic.Bool
				h := NewHandlers(store, mockconsensus.NewMockElection(ctrl), &ready)

				r := httptest.NewRequest(method, "/v1/cluster/spec", nil)
				r.Body = errCloseBody{strings.NewReader(`{"initMode":"new"}`)}
				r.Header.Set("Content-Type", contentType)
				w := httptest.NewRecorder()
				handler(h)(w, r)
				// failing to close the body is only logged: the spec is updated
				Expect(w.Code).To(Equal(http.StatusOK), "body: %s", w.Body)
			},
			Entry("PATCH", http.MethodPatch, endpoints.ContentTypeMergePatch,
				func(h *Handlers) http.HandlerFunc { return h.PatchClusterSpecHandler }),
			Entry("PUT", http.MethodPut, "application/json",
				func(h *Handlers) http.HandlerFunc { return h.PutClusterSpecHandler }),
		)
	})

	Describe("patchClusterSpec", func() {
		DescribeTable("merges a patch into a spec",
			func(patch string, check func(spec *apiv1.Spec)) {
				spec, err := patchClusterSpec(testSpec(), []byte(patch))
				Expect(err).NotTo(HaveOccurred())
				check(spec)
			},
			Entry("objects are merged", `{"pgParameters":{"shared_buffers":"1GB"}}`, func(spec *apiv1.Spec) {
				workMem("8MB")(spec)
				Expect(spec.PGParameters["shared_buffers"]).To(Equal("1GB"))
			}),
			Entry("null removes a field", `{"pgParameters":null}`, workMem("")),
			Entry("other values replace the field", `{"pgParameters":{"work_mem":"16MB"}}`, workMem("16MB")),
		)

		DescribeTable("fails",
			func(patch string) {
				spec, err := patchClusterSpec(testSpec(), []byte(patch))
				Expect(err).To(HaveOccurred(), "expected an error, got spec %v", spec)
			},
			Entry("a patch that is not JSON", `{`),
			Entry("a value of the wrong type", `{"maxStandbys":-1}`),
		)
	})

	Describe("updating the spec", func() {
		It("stops retrying at other errors than conflicts", func(ctx SpecContext) {
			errBoom := errors.New("boom")
			store := mockconsensus.NewMockStore(gomock.NewController(GinkgoT()))
			store.EXPECT().GetClusterData(gomock.Any()).Return(nil, nil, errBoom).Times(1)
			r := httptest.NewRequest(http.MethodPut, "/v1/cluster/spec", nil)
			_, err := updateSpecRetries(ctx, store, []byte(`{}`), true, r)
			Expect(err).To(MatchError(errBoom))
		})

		It("reports every conflict", func(ctx SpecContext) {
			store := mockconsensus.NewMockStore(gomock.NewController(GinkgoT()))
			store.EXPECT().GetClusterData(gomock.Any()).DoAndReturn(
				func(context.Context) (*apiv1.Data, *consensus.KVPair, error) {
					return testSpecData(testSpec()), &consensus.KVPair{}, nil
				}).Times(3)
			store.EXPECT().AtomicPutClusterData(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(nil, consensus.ErrKeyModified).Times(3)
			r := httptest.NewRequest(http.MethodPut, "/v1/cluster/spec", nil)
			_, err := updateSpecRetries(ctx, store, []byte(`{"initMode":"new"}`), true, r)
			Expect(err).To(MatchError(ContainSubstring("3 retries")), "expected an error after 3 retries")
			Expect(strings.Count(err.Error(), consensus.ErrKeyModified.Error())).To(Equal(3),
				"expected the 3 conflicts in the error: %v", err)
		})

		It("returns a 400 status error for an invalid spec", func(ctx SpecContext) {
			store := mockconsensus.NewMockStore(gomock.NewController(GinkgoT()))
			store.EXPECT().GetClusterData(gomock.Any()).Return(testSpecData(testSpec()), &consensus.KVPair{}, nil)
			r := httptest.NewRequest(http.MethodPatch, "/v1/cluster/spec", nil)
			_, err := tryUpdateSpec(ctx, store, []byte(`{"maxStandbys":0}`), false, r)
			var se statusError
			Expect(errors.As(err, &se)).To(BeTrue(), "expected a status error, got %v", err)
			Expect(se.status).To(Equal(http.StatusBadRequest))
			Expect(err.Error()).To(HavePrefix("invalid spec: "))
		})
	})

	Describe("statusError", func() {
		It("wraps an error with a status", func() {
			err := statusError{http.StatusNotFound, errNoSpec}
			Expect(err.Error()).To(Equal(errNoSpec.Error()), "expected the message of the wrapped error")
			Expect(errors.Is(err, errNoSpec)).To(BeTrue(), "expected the wrapped error to be unwrapped")
			wrapped := fmt.Errorf("wrapped: %w", err)
			var se statusError
			Expect(errors.As(wrapped, &se)).To(BeTrue(), "expected the status error in a wrapped error")
			Expect(se.status).To(Equal(http.StatusNotFound))
		})
	})

	Describe("writeSpecError", func() {
		DescribeTable("writes a problem",
			func(ctx SpecContext, err error, status int, detail string) {
				w := httptest.NewRecorder()
				writeSpecError(ctx, w, err, "failed to update")
				Expect(w.Code).To(Equal(status))
				Expect(w.Header().Get("Content-Type")).To(Equal(endpoints.ContentTypeProblem), "expected a problem")
				var problem endpoints.Problem
				Expect(json.Unmarshal(w.Body.Bytes(), &problem)).To(Succeed(), "expected a problem, got %s", w.Body)
				if detail != "" {
					Expect(problem.Detail).To(Equal(detail))
				}
			},
			Entry("a status error", statusError{http.StatusConflict, errors.New("no")}, http.StatusConflict, "no"),
			Entry("a wrapped status error", fmt.Errorf("w: %w", errPreconditionFailed),
				http.StatusPreconditionFailed, ""),
			Entry("another error", errors.New("secret"), http.StatusInternalServerError, "failed to update"),
		)
	})
})
