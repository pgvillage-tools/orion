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
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/golang/mock/gomock"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/consensus"
	mockconsensus "github.com/pgvillage-tools/orion/internal/mock/consensus"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

func TestCanFailOver(t *testing.T) {
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
			Keepers: apiv1.Keepers{"keeper1": {UID: "keeper1", Status: apiv1.KeeperStatus{Healthy: true}}, "keeper2": k2},
			DBs: apiv1.DBs{
				"db1": {UID: "db1", Spec: &apiv1.DBSpec{KeeperUID: "keeper1"}, Status: apiv1.DBStatus{TimelineID: 3}},
				"db2": db2,
			},
		}
	}
	for name, tc := range map[string]struct {
		cd       *apiv1.Data
		keeper   string
		expected bool
	}{
		"a standby can take over":            {data(nil), "keeper1", true},
		"failing a standby is fine":          {data(func(k *apiv1.Keeper, _ *apiv1.DB) { k.Status.Healthy = false }), "keeper2", true},
		"the standby's keeper is unhealthy":  {data(func(k *apiv1.Keeper, _ *apiv1.DB) { k.Status.Healthy = false }), "keeper1", false},
		"the standby's db is unhealthy":      {data(func(_ *apiv1.Keeper, db *apiv1.DB) { db.Status.Healthy = false }), "keeper1", false},
		"the standby's db isn't converged":   {data(func(_ *apiv1.Keeper, db *apiv1.DB) { db.Status.CurrentGeneration = 4 }), "keeper1", false},
		"the standby is on another timeline": {data(func(_ *apiv1.Keeper, db *apiv1.DB) { db.Status.TimelineID = 2 }), "keeper1", false},
		"the only keeper": {func() *apiv1.Data {
			cd := data(nil)
			delete(cd.DBs, "db2")
			delete(cd.Keepers, "keeper2")
			return cd
		}(), "keeper1", false},
		"no cluster": {func() *apiv1.Data {
			cd := data(nil)
			cd.Cluster = nil
			return cd
		}(), "keeper1", true},
		"the standby's keeper is not in the cluster data": {func() *apiv1.Data {
			cd := data(nil)
			delete(cd.Keepers, "keeper2")
			return cd
		}(), "keeper1", false},
		"a db without a spec cannot take over": {func() *apiv1.Data {
			cd := data(nil)
			cd.DBs["db2"].Spec = nil
			return cd
		}(), "keeper1", false},
		"the primary db has no spec": {func() *apiv1.Data {
			cd := data(nil)
			cd.DBs["db1"].Spec = nil
			return cd
		}(), "keeper1", true},
		"no primary": {func() *apiv1.Data {
			cd := data(nil)
			cd.Cluster.Status.Master = ""
			return cd
		}(), "keeper1", true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := canFailOver(tc.cd, tc.cd.Keepers[tc.keeper]); got != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}

func TestFailKeeperHandler(t *testing.T) {
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

	for name, tc := range map[string]struct {
		cd     *apiv1.Data
		getErr error
		keeper string
		// stores is true when the handler must write the cluster data; putErr is what that write returns
		stores bool
		putErr error
		status int
	}{
		"failing the primary when a standby can take over": {
			cd: data(nil), keeper: "keeper1", stores: true, status: http.StatusAccepted,
		},
		"failing a standby": {
			cd: data(nil), keeper: "keeper2", stores: true, status: http.StatusAccepted,
		},
		"failing the primary when no standby can take over": {
			cd: data(func(cd *apiv1.Data) { cd.Keepers["keeper2"].Status.Healthy = false }), keeper: "keeper1",
			status: http.StatusConflict,
		},
		"an unknown keeper": {
			cd: data(nil), keeper: "keeper3", status: http.StatusNotFound,
		},
		"no cluster data": {
			keeper: "keeper1", status: http.StatusNotFound,
		},
		"no cluster": {
			cd: data(func(cd *apiv1.Data) { cd.Cluster = nil }), keeper: "keeper1", status: http.StatusNotFound,
		},
		"a cluster without a spec": {
			cd: data(func(cd *apiv1.Data) { cd.Cluster.Spec = nil }), keeper: "keeper1", status: http.StatusNotFound,
		},
		"the cluster data cannot be read": {
			getErr: errBoom, keeper: "keeper1", status: http.StatusInternalServerError,
		},
		"the cluster data changed in the meantime": {
			cd: data(nil), keeper: "keeper1", stores: true, putErr: fmt.Errorf("put: %w", consensus.ErrKeyModified),
			status: http.StatusConflict,
		},
		"the cluster data cannot be stored": {
			cd: data(nil), keeper: "keeper1", stores: true, putErr: errBoom, status: http.StatusInternalServerError,
		},
	} {
		t.Run(name, func(t *testing.T) {
			store := mockconsensus.NewMockStore(gomock.NewController(t))
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
			h := NewHandlers(store, mockconsensus.NewMockElection(gomock.NewController(t)), &ready)

			r := httptest.NewRequest(http.MethodPost, "/v1/cluster/keepers/"+tc.keeper+"/actions/fail", nil)
			r.SetPathValue("uid", tc.keeper)
			w := httptest.NewRecorder()
			h.FailKeeperHandler(w, r)

			if w.Code != tc.status {
				t.Fatalf("expected status %d, got %d: %s", tc.status, w.Code, w.Body)
			}
			if tc.status == http.StatusAccepted {
				if location := w.Header().Get("Location"); location != "/v1/cluster/status" {
					t.Errorf("expected Location /v1/cluster/status, got %q", location)
				}
			} else {
				var problem endpoints.Problem
				if ct := w.Header().Get("Content-Type"); ct != endpoints.ContentTypeProblem {
					t.Errorf("expected Content-Type %s, got %q", endpoints.ContentTypeProblem, ct)
				}
				if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil || problem.Status != tc.status {
					t.Errorf("expected a problem with status %d, got %s (%v)", tc.status, w.Body, err)
				}
			}
			if !tc.stores {
				return
			}
			// only the failed keeper is forced to fail, in a copy of the cluster data that was read
			for uid, k := range stored.Keepers {
				if k.Status.ForceFail != (uid == tc.keeper) {
					t.Errorf("keeper %s: expected ForceFail %v, got %v", uid, uid == tc.keeper, k.Status.ForceFail)
				}
			}
			if tc.cd.Keepers[tc.keeper].Status.ForceFail {
				t.Error("expected the cluster data that was read to be left alone")
			}
		})
	}
}
