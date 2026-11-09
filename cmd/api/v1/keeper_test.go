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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/golang/mock/gomock"
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

func TestDeleteKeeperHandler(t *testing.T) {
	errBoom := errors.New("boom")
	modified := fmt.Errorf("put: %w", consensus.ErrKeyModified)

	for name, tc := range map[string]struct {
		keeper    string
		cd        *apiv1.Data
		getErr    error
		writes    bool
		putErr    error
		status    int
		keepers   string // the keepers in the stored cluster data
		dbs       string // the dbs in the stored cluster data
		detailHas string
	}{
		"a keeper with a standby is removed with its db": {
			keeper: "keeper2", cd: keeperTestData(), writes: true, status: http.StatusAccepted,
			keepers: "keeper1,keeper3", dbs: "db1",
		},
		"a keeper without db is removed": {
			keeper: "keeper3", cd: keeperTestData(), writes: true, status: http.StatusAccepted,
			keepers: "keeper1,keeper2", dbs: "db1,db2",
		},
		"the keeper of the primary": {
			keeper: "keeper1", cd: keeperTestData(), status: http.StatusConflict,
			detailHas: "/v1/cluster/keepers/keeper1/actions/fail",
		},
		"an unknown keeper": {
			keeper: "gone", cd: keeperTestData(), status: http.StatusNotFound, detailHas: "keeper gone doesn't exist",
		},
		"no cluster data": {
			keeper: "keeper2", status: http.StatusNotFound,
		},
		"cluster data without cluster": {
			keeper: "keeper2", cd: &apiv1.Data{}, status: http.StatusNotFound,
		},
		"a cluster without spec": {
			keeper: "keeper2", cd: &apiv1.Data{Cluster: &apiv1.Cluster{UID: "cluster1"}}, status: http.StatusNotFound,
		},
		"the cluster data cannot be read": {
			keeper: "keeper2", getErr: errBoom, status: http.StatusInternalServerError,
		},
		"the cluster data changed while removing": {
			keeper: "keeper2", cd: keeperTestData(), writes: true, putErr: modified, status: http.StatusConflict,
			keepers: "keeper1,keeper3", dbs: "db1", detailHas: "try again",
		},
		"the cluster data cannot be written": {
			keeper: "keeper2", cd: keeperTestData(), writes: true, putErr: errBoom,
			status: http.StatusInternalServerError, keepers: "keeper1,keeper3", dbs: "db1",
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
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
			r.SetPathValue("id", tc.keeper)
			w := httptest.NewRecorder()
			h.DeleteKeeperHandler(w, r)

			if w.Code != tc.status {
				t.Fatalf("expected status %d, got %d: %s", tc.status, w.Code, w.Body)
			}
			if tc.status == http.StatusAccepted {
				if got := w.Header().Get("Location"); got != "/v1/cluster/status" {
					t.Errorf("expected Location /v1/cluster/status, got %q", got)
				}
			} else if ct := w.Header().Get("Content-Type"); ct != endpoints.ContentTypeProblem {
				t.Errorf("expected a problem, got Content-Type %q", ct)
			}
			if !strings.Contains(w.Body.String(), tc.detailHas) {
				t.Errorf("expected %q in the body, got %s", tc.detailHas, w.Body)
			}
			if !tc.writes {
				return
			}
			if got := sortedKeys(stored.Keepers); got != tc.keepers {
				t.Errorf("expected keepers %s to be stored, got %s", tc.keepers, got)
			}
			if got := sortedKeys(stored.DBs); got != tc.dbs {
				t.Errorf("expected dbs %s to be stored, got %s", tc.dbs, got)
			}
			if stored.Cluster.Status.Master != "db1" {
				t.Errorf("expected the primary to be kept, got %q", stored.Cluster.Status.Master)
			}
			// the handler changes a copy: the cluster data it read stays as it was
			if got := sortedKeys(tc.cd.Keepers); got != "keeper1,keeper2,keeper3" {
				t.Errorf("expected the cluster data that was read to be unchanged, got keepers %s", got)
			}
		})
	}
}
