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
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

var _ = Describe("canFailOver", func() {
	// data returns cluster data with a primary db1 on keeper1 (timeline 3), and a standby db2 on keeper2 that is
	// changed by standby
	data := func(standby func(k *apiv1.Keeper, db *apiv1.DB)) *apiv1.Data {
		k2 := &apiv1.Keeper{UID: "keeper2", Status: apiv1.KeeperStatus{Healthy: true}}
		db2 := &apiv1.DB{UID: "db2", Generation: 5, Spec: &apiv1.DBSpec{KeeperUID: "keeper2"},
			Status: apiv1.DBStatus{Healthy: true, CurrentGeneration: 5, TimelineID: 3}}
		if standby != nil {
			standby(k2, db2)
		}
		return &apiv1.Data{
			Cluster: &apiv1.Cluster{Status: apiv1.ClusterStatus{Master: "db1"}},
			Keepers: apiv1.Keepers{
				"keeper1": {UID: "keeper1", Status: apiv1.KeeperStatus{Healthy: true}},
				"keeper2": k2,
			},
			DBs: apiv1.DBs{
				"db1": {UID: "db1", Spec: &apiv1.DBSpec{KeeperUID: "keeper1"}, Status: apiv1.DBStatus{TimelineID: 3}},
				"db2": db2,
			},
		}
	}
	unhealthyKeeper := func(k *apiv1.Keeper, _ *apiv1.DB) { k.Status.Healthy = false }

	DescribeTable("tells whether a standby can take over from the primary when a keeper fails",
		func(cd *apiv1.Data, keeper string, expected bool) {
			Expect(canFailOver(cd, cd.Keepers[keeper])).To(Equal(expected))
		},
		Entry("a standby can take over", data(nil), "keeper1", true),
		Entry("failing a standby is fine", data(unhealthyKeeper), "keeper2", true),
		Entry("the standby's keeper is unhealthy", data(unhealthyKeeper), "keeper1", false),
		Entry("the standby's db is unhealthy",
			data(func(_ *apiv1.Keeper, db *apiv1.DB) { db.Status.Healthy = false }), "keeper1", false),
		Entry("the standby's db isn't converged",
			data(func(_ *apiv1.Keeper, db *apiv1.DB) { db.Status.CurrentGeneration = 4 }), "keeper1", false),
		Entry("the standby is on another timeline",
			data(func(_ *apiv1.Keeper, db *apiv1.DB) { db.Status.TimelineID = 2 }), "keeper1", false),
		Entry("the only keeper", func() *apiv1.Data {
			cd := data(nil)
			delete(cd.DBs, "db2")
			delete(cd.Keepers, "keeper2")
			return cd
		}(), "keeper1", false),
		Entry("no cluster", func() *apiv1.Data {
			cd := data(nil)
			cd.Cluster = nil
			return cd
		}(), "keeper1", true),
		Entry("the standby's keeper is not in the cluster data", func() *apiv1.Data {
			cd := data(nil)
			delete(cd.Keepers, "keeper2")
			return cd
		}(), "keeper1", false),
		Entry("a db without a spec cannot take over", func() *apiv1.Data {
			cd := data(nil)
			cd.DBs["db2"].Spec = nil
			return cd
		}(), "keeper1", false),
		Entry("the primary db has no spec", func() *apiv1.Data {
			cd := data(nil)
			cd.DBs["db1"].Spec = nil
			return cd
		}(), "keeper1", true),
		Entry("no primary", func() *apiv1.Data {
			cd := data(nil)
			cd.Cluster.Status.Master = ""
			return cd
		}(), "keeper1", true),
	)
})

var _ = Describe("POST /v1/cluster/keepers/{uid}/actions/fail", func() {
	errBoom := errors.New("boom")
	pair := &consensus.KVPair{Key: "clusterdata", LastIndex: 42}
	// data returns cluster data with a primary db1 on keeper1, and a standby db2 on keeper2 that can take over, changed
	// by change
	data := func(change func(cd *apiv1.Data)) *apiv1.Data {
		cd := &apiv1.Data{
			Cluster: &apiv1.Cluster{Spec: &apiv1.Spec{}, Status: apiv1.ClusterStatus{Master: "db1"}},
			Keepers: apiv1.Keepers{
				"keeper1": {UID: "keeper1", Status: apiv1.KeeperStatus{Healthy: true}},
				"keeper2": {UID: "keeper2", Status: apiv1.KeeperStatus{Healthy: true}},
			},
			DBs: apiv1.DBs{
				"db1": {UID: "db1", Spec: &apiv1.DBSpec{KeeperUID: "keeper1"}, Status: apiv1.DBStatus{TimelineID: 3}},
				"db2": {UID: "db2", Generation: 5, Spec: &apiv1.DBSpec{KeeperUID: "keeper2"},
					Status: apiv1.DBStatus{Healthy: true, CurrentGeneration: 5, TimelineID: 3}},
			},
		}
		if change != nil {
			change(cd)
		}
		return cd
	}

	type failCase struct {
		cd     *apiv1.Data
		getErr error
		keeper string
		// stores is true when the handler must write the cluster data; putErr is what that write returns
		stores bool
		putErr error
		status int
	}

	DescribeTable("FailKeeperHandler",
		func(tc failCase) {
			store := mockconsensus.NewMockStore(gomock.NewController(GinkgoT()))
			store.EXPECT().GetClusterData(gomock.Any()).Return(tc.cd, pair, tc.getErr)
			var stored *apiv1.Data
			if tc.stores {
				store.EXPECT().AtomicPutClusterData(gomock.Any(), gomock.Any(), pair).DoAndReturn(
					func(_ any, cd *apiv1.Data, _ *consensus.KVPair) (*consensus.KVPair, error) {
						stored = cd
						return &consensus.KVPair{Key: pair.Key, LastIndex: pair.LastIndex + 1}, tc.putErr
					})
			}
			var ready atomic.Bool
			h := NewHandlers(store, mockconsensus.NewMockElection(gomock.NewController(GinkgoT())), &ready)

			r := httptest.NewRequest(http.MethodPost, "/v1/cluster/keepers/"+tc.keeper+"/actions/fail", nil)
			r.SetPathValue("uid", tc.keeper)
			w := httptest.NewRecorder()
			h.FailKeeperHandler(w, r)

			Expect(w.Code).To(Equal(tc.status), "body: %s", w.Body)
			if tc.status == http.StatusAccepted {
				Expect(w.Header().Get("Location")).To(Equal("/v1/cluster/status"))
			} else {
				var problem endpoints.Problem
				Expect(w.Header().Get("Content-Type")).To(Equal(endpoints.ContentTypeProblem))
				Expect(json.Unmarshal(w.Body.Bytes(), &problem)).To(Succeed(), "expected a problem, got %s", w.Body)
				Expect(problem.Status).To(Equal(tc.status), "expected a problem with status %d", tc.status)
			}
			if !tc.stores {
				return
			}
			// only the failed keeper is forced to fail, in a copy of the cluster data that was read
			for uid, k := range stored.Keepers {
				Expect(k.Status.ForceFail).To(Equal(uid == tc.keeper), "keeper %s: ForceFail", uid)
			}
			Expect(tc.cd.Keepers[tc.keeper].Status.ForceFail).To(BeFalse(),
				"expected the cluster data that was read to be left alone")
		},
		Entry("failing the primary when a standby can take over", failCase{
			cd: data(nil), keeper: "keeper1", stores: true, status: http.StatusAccepted,
		}),
		Entry("failing a standby", failCase{
			cd: data(nil), keeper: "keeper2", stores: true, status: http.StatusAccepted,
		}),
		Entry("failing the primary when no standby can take over", failCase{
			cd: data(func(cd *apiv1.Data) { cd.Keepers["keeper2"].Status.Healthy = false }), keeper: "keeper1",
			status: http.StatusConflict,
		}),
		Entry("an unknown keeper", failCase{
			cd: data(nil), keeper: "keeper3", status: http.StatusNotFound,
		}),
		Entry("no cluster data", failCase{
			keeper: "keeper1", status: http.StatusNotFound,
		}),
		Entry("no cluster", failCase{
			cd: data(func(cd *apiv1.Data) { cd.Cluster = nil }), keeper: "keeper1", status: http.StatusNotFound,
		}),
		Entry("a cluster without a spec", failCase{
			cd: data(func(cd *apiv1.Data) { cd.Cluster.Spec = nil }), keeper: "keeper1", status: http.StatusNotFound,
		}),
		Entry("the cluster data cannot be read", failCase{
			getErr: errBoom, keeper: "keeper1", status: http.StatusInternalServerError,
		}),
		Entry("the cluster data changed in the meantime", failCase{
			cd: data(nil), keeper: "keeper1", stores: true, putErr: fmt.Errorf("put: %w", consensus.ErrKeyModified),
			status: http.StatusConflict,
		}),
		Entry("the cluster data cannot be stored", failCase{
			cd: data(nil), keeper: "keeper1", stores: true, putErr: errBoom, status: http.StatusInternalServerError,
		}),
	)
})
