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
	"net/http"
	"net/http/httptest"
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

var _ = Describe("POST /v1/cluster/actions/promote", func() {
	errBoom := errors.New("boom")
	pair := &consensus.KVPair{LastIndex: 7}
	standby := func() *apiv1.Data {
		return apiv1.NewClusterData(apiv1.NewCluster("cluster1", &apiv1.Spec{
			InitMode: util.ToPtr(apiv1.New), Role: util.ToPtr(apiv1.Replica), MaxStandbys: util.ToPtr[uint16](5),
		}))
	}

	type promoteCase struct {
		cd     *apiv1.Data
		getErr error
		// writes is whether the handler writes the promoted cluster data, putErr what that write returns
		writes bool
		putErr error
		status int
	}

	DescribeTable("PromoteClusterHandler",
		func(tc promoteCase) {
			store := mockconsensus.NewMockStore(gomock.NewController(GinkgoT()))
			var ready atomic.Bool
			h := NewHandlers(store, nil, &ready)
			store.EXPECT().GetClusterData(gomock.Any()).Return(tc.cd, pair, tc.getErr)
			var stored *apiv1.Data
			if tc.writes {
				// only written when the cluster data didn't change since it was read
				store.EXPECT().AtomicPutClusterData(gomock.Any(), gomock.Any(), pair).DoAndReturn(
					func(_ any, cd *apiv1.Data, _ *consensus.KVPair) (*consensus.KVPair, error) {
						stored = cd
						return &consensus.KVPair{LastIndex: 8}, tc.putErr
					})
			}

			w := httptest.NewRecorder()
			h.PromoteClusterHandler(w, httptest.NewRequest(http.MethodPost, "/v1/cluster/actions/promote", nil))

			Expect(w.Code).To(Equal(tc.status), "body: %s", w.Body)
			if tc.status >= http.StatusBadRequest {
				var problem endpoints.Problem
				Expect(w.Header().Get("Content-Type")).To(Equal(endpoints.ContentTypeProblem))
				Expect(json.Unmarshal(w.Body.Bytes(), &problem)).To(Succeed(), "expected a problem, got %s", w.Body)
				Expect(problem.Status).To(Equal(tc.status), "expected a problem with status %d", tc.status)
			}
			if tc.status != http.StatusAccepted {
				Expect(w.Header().Get("Location")).To(BeEmpty(), "expected no Location")
				return
			}
			Expect(w.Header().Get("Location")).To(Equal("/v1/cluster/status"))
			// the stored spec is the old one with the primary role; the cluster data that was read is unchanged
			Expect(stored).NotTo(BeNil(), "expected cluster1 with the primary role and its other settings")
			Expect(stored.Cluster.UID).To(Equal("cluster1"))
			Expect(*stored.Cluster.Spec.Role).To(Equal(apiv1.Primary))
			Expect(*stored.Cluster.Spec.MaxStandbys).To(BeEquivalentTo(5))
			Expect(*tc.cd.Cluster.Spec.Role).To(Equal(apiv1.Replica),
				"expected the cluster data that was read to stay unchanged")
		},
		Entry("a standby cluster is promoted and returns 202", promoteCase{
			cd: standby(), writes: true, status: http.StatusAccepted,
		}),
		Entry("a primary cluster stays as it is and returns 200", promoteCase{
			cd: apiv1.NewClusterData(apiv1.NewCluster("cluster1",
				&apiv1.Spec{InitMode: util.ToPtr(apiv1.New), Role: util.ToPtr(apiv1.Primary)})),
			status: http.StatusOK,
		}),
		Entry("a cluster without a role is a primary cluster and returns 200", promoteCase{
			cd:     apiv1.NewClusterData(apiv1.NewCluster("cluster1", &apiv1.Spec{InitMode: util.ToPtr(apiv1.New)})),
			status: http.StatusOK,
		}),
		Entry("no cluster data returns 404", promoteCase{
			status: http.StatusNotFound,
		}),
		Entry("cluster data without a cluster returns 404", promoteCase{
			cd: &apiv1.Data{}, status: http.StatusNotFound,
		}),
		Entry("a cluster without a spec returns 404", promoteCase{
			cd: apiv1.NewClusterData(apiv1.NewCluster("cluster1", nil)), status: http.StatusNotFound,
		}),
		Entry("a failure to read the cluster data returns 500", promoteCase{
			getErr: errBoom, status: http.StatusInternalServerError,
		}),
		Entry("a change while promoting returns 409", promoteCase{
			cd: standby(), writes: true, putErr: fmt.Errorf("writing: %w", consensus.ErrKeyModified),
			status: http.StatusConflict,
		}),
		Entry("a failing write returns 500", promoteCase{
			cd: standby(), writes: true, putErr: errBoom, status: http.StatusInternalServerError,
		}),
	)
})
