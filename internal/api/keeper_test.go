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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
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

// keeperTestData returns cluster data with 3 keepers: keeper1 runs the primary (db1), keeper2 a standby (db2), and
// keeper3 has no db
func keeperTestData() *apiv1.Data {
	initMode := apiv1.New
	cd := apiv1.NewClusterData(&apiv1.Cluster{
		UID:    "cluster1",
		Spec:   &apiv1.Spec{InitMode: &initMode},
		Status: apiv1.ClusterStatus{Master: "db1"},
	})
	for _, uid := range []string{"keeper1", "keeper2", "keeper3"} {
		cd.Keepers[uid] = &apiv1.Keeper{UID: uid}
	}
	cd.DBs["db1"] = &apiv1.DB{UID: "db1", Spec: &apiv1.DBSpec{KeeperUID: "keeper1"}}
	cd.DBs["db2"] = &apiv1.DB{UID: "db2", Spec: &apiv1.DBSpec{KeeperUID: "keeper2"}}
	return cd
}

// sortedKeys returns the keys of m, sorted
func sortedKeys[V any](m map[string]V) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

var _ = Describe("DELETE /v1/cluster/keepers/{uid}", func() {
	errBoom := errors.New("boom")
	modified := fmt.Errorf("put: %w", consensus.ErrKeyModified)

	type deleteCase struct {
		keeper    string
		cd        *apiv1.Data
		getErr    error
		writes    bool
		putErr    error
		status    int
		keepers   string // the keepers in the stored cluster data
		dbs       string // the dbs in the stored cluster data
		detailHas string
	}

	DescribeTable("DeleteKeeperHandler",
		func(tc deleteCase) {
			ctrl := gomock.NewController(GinkgoT())
			store := mockconsensus.NewMockStore(ctrl)
			pair := &consensus.KVPair{LastIndex: 7}
			store.EXPECT().GetClusterData(gomock.Any()).Return(tc.cd, pair, tc.getErr)
			var stored *apiv1.Data
			if tc.writes {
				store.EXPECT().AtomicPutClusterData(gomock.Any(), gomock.Any(), pair).DoAndReturn(
					func(_ context.Context, cd *apiv1.Data, _ *consensus.KVPair) (*consensus.KVPair, error) {
						stored = cd
						return &consensus.KVPair{LastIndex: 8}, tc.putErr
					})
			}
			var ready atomic.Bool
			h := NewHandlers(store, mockconsensus.NewMockElection(ctrl), &ready)

			r := httptest.NewRequest(http.MethodDelete, "/v1/cluster/keepers/"+tc.keeper, nil)
			r.SetPathValue("uid", tc.keeper)
			w := httptest.NewRecorder()
			h.DeleteKeeperHandler(w, r)

			Expect(w.Code).To(Equal(tc.status), "body: %s", w.Body)
			if tc.status == http.StatusAccepted {
				Expect(w.Header().Get("Location")).To(Equal("/v1/cluster/status"))
			} else {
				Expect(w.Header().Get("Content-Type")).To(Equal(endpoints.ContentTypeProblem), "expected a problem")
			}
			Expect(w.Body.String()).To(ContainSubstring(tc.detailHas))
			if !tc.writes {
				return
			}
			Expect(sortedKeys(stored.Keepers)).To(Equal(tc.keepers), "the stored keepers")
			Expect(sortedKeys(stored.DBs)).To(Equal(tc.dbs), "the stored dbs")
			Expect(stored.Cluster.Status.Master).To(Equal("db1"), "expected the primary to be kept")
			// the handler changes a copy: the cluster data it read stays as it was
			Expect(sortedKeys(tc.cd.Keepers)).To(Equal("keeper1,keeper2,keeper3"),
				"expected the cluster data that was read to be unchanged")
		},
		Entry("a keeper with a standby is removed with its db", deleteCase{
			keeper: "keeper2", cd: keeperTestData(), writes: true, status: http.StatusAccepted,
			keepers: "keeper1,keeper3", dbs: "db1",
		}),
		Entry("a keeper without db is removed", deleteCase{
			keeper: "keeper3", cd: keeperTestData(), writes: true, status: http.StatusAccepted,
			keepers: "keeper1,keeper2", dbs: "db1,db2",
		}),
		Entry("the keeper of the primary", deleteCase{
			keeper: "keeper1", cd: keeperTestData(), status: http.StatusConflict,
			detailHas: "/v1/cluster/keepers/keeper1/actions/fail",
		}),
		Entry("an unknown keeper", deleteCase{
			keeper: "gone", cd: keeperTestData(), status: http.StatusNotFound, detailHas: "keeper gone doesn't exist",
		}),
		Entry("no cluster data", deleteCase{
			keeper: "keeper2", status: http.StatusNotFound,
		}),
		Entry("cluster data without cluster", deleteCase{
			keeper: "keeper2", cd: &apiv1.Data{}, status: http.StatusNotFound,
		}),
		Entry("a cluster without spec", deleteCase{
			keeper: "keeper2", cd: &apiv1.Data{Cluster: &apiv1.Cluster{UID: "cluster1"}}, status: http.StatusNotFound,
		}),
		Entry("the cluster data cannot be read", deleteCase{
			keeper: "keeper2", getErr: errBoom, status: http.StatusInternalServerError,
		}),
		Entry("the cluster data changed while removing", deleteCase{
			keeper: "keeper2", cd: keeperTestData(), writes: true, putErr: modified, status: http.StatusConflict,
			keepers: "keeper1,keeper3", dbs: "db1", detailHas: "try again",
		}),
		Entry("the cluster data cannot be written", deleteCase{
			keeper: "keeper2", cd: keeperTestData(), writes: true, putErr: errBoom,
			status: http.StatusInternalServerError, keepers: "keeper1,keeper3", dbs: "db1",
		}),
	)
})
