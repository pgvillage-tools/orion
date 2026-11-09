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
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/golang/mock/gomock"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/consensus"
	mockconsensus "github.com/pgvillage-tools/orion/internal/mock/consensus"
)

func TestSentinelHandlers(t *testing.T) {
	errBoom := errors.New("boom")
	sentinels := apiv1.SentinelsInfo{{UID: "sentinel2"}, {UID: "sentinel1"}}

	for name, tc := range map[string]struct {
		sentinels apiv1.SentinelsInfo
		storeErr  error
		leader    string
		leaderErr error
		// readsLeader is false when the handler must stop before it asks for the leader
		readsLeader bool
		status      int
		list, one   string // body of GET /v1/cluster/sentinels and of GET /v1/cluster/sentinels/sentinel1
	}{
		"a leader": {
			sentinels: sentinels, leader: "sentinel1", readsLeader: true, status: http.StatusOK,
			list: `[{"uid":"sentinel1","leader":true},{"uid":"sentinel2","leader":false}]`,
			one:  `{"uid":"sentinel1","leader":true}`,
		},
		"no leader": {
			sentinels: sentinels, leaderErr: consensus.ErrElectionNoLeader, readsLeader: true, status: http.StatusOK,
			list: `[{"uid":"sentinel1","leader":false},{"uid":"sentinel2","leader":false}]`,
			one:  `{"uid":"sentinel1","leader":false}`,
		},
		"a leader that is not a known sentinel": {
			sentinels: sentinels, leader: "gone", readsLeader: true, status: http.StatusOK,
			list: `[{"uid":"sentinel1","leader":false},{"uid":"sentinel2","leader":false}]`,
			one:  `{"uid":"sentinel1","leader":false}`,
		},
		"no sentinels": {
			readsLeader: true, status: http.StatusOK, list: `[]`,
		},
		"the leader cannot be read": {
			sentinels: sentinels, leaderErr: errBoom, readsLeader: true, status: http.StatusInternalServerError,
		},
		"the sentinels cannot be read": {
			storeErr: errBoom, status: http.StatusInternalServerError,
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			store := mockconsensus.NewMockStore(ctrl)
			store.EXPECT().GetSentinelsInfo(gomock.Any()).Return(tc.sentinels, tc.storeErr).Times(2)
			election := mockconsensus.NewMockElection(ctrl)
			if tc.readsLeader {
				// the election client is shared: every request reads the leader through it, nothing else
				election.EXPECT().Leader().Return(tc.leader, tc.leaderErr).Times(2)
			}
			var ready atomic.Bool
			h := NewHandlers(store, election, &ready)

			for _, req := range []struct {
				handler http.HandlerFunc
				uid     string
				body    string
			}{
				{h.SentinelsHandler, "", tc.list},
				{h.SentinelHandler, "sentinel1", tc.one},
			} {
				r := httptest.NewRequest(http.MethodGet, "/v1/cluster/sentinels", nil)
				r.SetPathValue("uid", req.uid)
				w := httptest.NewRecorder()
				req.handler(w, r)

				status := tc.status
				if status == http.StatusOK && req.uid != "" && req.body == "" {
					status = http.StatusNotFound
				}
				if w.Code != status {
					t.Errorf("expected status %d, got %d: %s", status, w.Code, w.Body)
				}
				if status == http.StatusOK && strings.TrimSpace(w.Body.String()) != req.body {
					t.Errorf("expected body %s, got %s", req.body, w.Body)
				}
			}
		})
	}
}
