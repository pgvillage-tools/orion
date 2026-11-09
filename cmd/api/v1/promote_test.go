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
	"github.com/pgvillage-tools/orion/internal/util"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

func TestPromoteCluster(t *testing.T) {
	errBoom := errors.New("boom")
	pair := &consensus.KVPair{LastIndex: 7}
	standby := func() *apiv1.Data {
		return apiv1.NewClusterData(apiv1.NewCluster("cluster1", &apiv1.Spec{
			InitMode: util.ToPtr(apiv1.New), Role: util.ToPtr(apiv1.Replica), MaxStandbys: util.ToPtr[uint16](5),
		}))
	}

	for name, tc := range map[string]struct {
		cd     *apiv1.Data
		getErr error
		// writes is whether the handler writes the promoted cluster data, putErr what that write returns
		writes bool
		putErr error
		status int
	}{
		"a standby cluster is promoted and returns 202": {
			cd: standby(), writes: true, status: http.StatusAccepted,
		},
		"a primary cluster stays as it is and returns 200": {
			cd: apiv1.NewClusterData(apiv1.NewCluster("cluster1",
				&apiv1.Spec{InitMode: util.ToPtr(apiv1.New), Role: util.ToPtr(apiv1.Primary)})),
			status: http.StatusOK,
		},
		"a cluster without a role is a primary cluster and returns 200": {
			cd:     apiv1.NewClusterData(apiv1.NewCluster("cluster1", &apiv1.Spec{InitMode: util.ToPtr(apiv1.New)})),
			status: http.StatusOK,
		},
		"no cluster data returns 404": {
			status: http.StatusNotFound,
		},
		"cluster data without a cluster returns 404": {
			cd: &apiv1.Data{}, status: http.StatusNotFound,
		},
		"a cluster without a spec returns 404": {
			cd: apiv1.NewClusterData(apiv1.NewCluster("cluster1", nil)), status: http.StatusNotFound,
		},
		"a failure to read the cluster data returns 500": {
			getErr: errBoom, status: http.StatusInternalServerError,
		},
		"a change while promoting returns 409": {
			cd: standby(), writes: true, putErr: fmt.Errorf("writing: %w", consensus.ErrKeyModified),
			status: http.StatusConflict,
		},
		"a failing write returns 500": {
			cd: standby(), writes: true, putErr: errBoom, status: http.StatusInternalServerError,
		},
	} {
		t.Run(name, func(t *testing.T) {
			store := mockconsensus.NewMockStore(gomock.NewController(t))
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

			if w.Code != tc.status {
				t.Fatalf("expected status %d, got %d: %s", tc.status, w.Code, w.Body)
			}
			if tc.status >= http.StatusBadRequest {
				var problem endpoints.Problem
				if ct := w.Header().Get("Content-Type"); ct != endpoints.ContentTypeProblem {
					t.Errorf("expected Content-Type %s, got %q", endpoints.ContentTypeProblem, ct)
				}
				if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil || problem.Status != tc.status {
					t.Errorf("expected a problem with status %d, got %s", tc.status, w.Body)
				}
			}
			if tc.status != http.StatusAccepted {
				if got := w.Header().Get("Location"); got != "" {
					t.Errorf("expected no Location, got %q", got)
				}
				return
			}
			if got := w.Header().Get("Location"); got != "/v1/cluster/status" {
				t.Errorf("expected Location /v1/cluster/status, got %q", got)
			}
			// the stored spec is the old one with the primary role; the cluster data that was read is unchanged
			if stored == nil || stored.Cluster.UID != "cluster1" || *stored.Cluster.Spec.Role != apiv1.Primary ||
				*stored.Cluster.Spec.MaxStandbys != 5 {
				t.Errorf("expected cluster1 with the primary role and its other settings, got %+v", stored)
			}
			if *tc.cd.Cluster.Spec.Role != apiv1.Replica {
				t.Error("expected the cluster data that was read to stay unchanged")
			}
		})
	}
}
