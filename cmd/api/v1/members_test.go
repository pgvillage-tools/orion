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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/golang/mock/gomock"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/consensus"
	mockconsensus "github.com/pgvillage-tools/orion/internal/mock/consensus"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// typicalClusterData returns cluster data with a primary db1 on keeper1 (that listens on 10.0.0.1:5432), a standby
// db2 on keeper2 that is not converged and has not reported its address yet, and keeper3 without a db
func typicalClusterData() *apiv1.Data {
	return &apiv1.Data{
		Cluster: &apiv1.Cluster{Spec: &apiv1.Spec{}, Status: apiv1.ClusterStatus{Master: "db1"}},
		Keepers: apiv1.Keepers{
			"keeper3": {UID: "keeper3"},
			"keeper1": {UID: "keeper1", Status: apiv1.KeeperStatus{Healthy: true}},
			"keeper2": {UID: "keeper2", Status: apiv1.KeeperStatus{Healthy: true}},
		},
		DBs: apiv1.DBs{
			"db1": {UID: "db1", Generation: 2, Spec: &apiv1.DBSpec{KeeperUID: "keeper1"},
				Status: apiv1.DBStatus{Healthy: true, CurrentGeneration: 2, ListenAddress: "10.0.0.1", Port: "5432"}},
			"db2": {UID: "db2", Generation: 3, Spec: &apiv1.DBSpec{KeeperUID: "keeper2"},
				Status: apiv1.DBStatus{CurrentGeneration: 2}},
		},
	}
}

const (
	keeper1JSON = `{"uid":"keeper1","listenAddress":"10.0.0.1:5432","healthy":true,"pgHealthy":true,` +
		`"pgWantedGeneration":2,"pgCurrentGeneration":2}`
	keeper2JSON = `{"uid":"keeper2","listenAddress":"(unknown)","healthy":true,"pgHealthy":false,` +
		`"pgWantedGeneration":3,"pgCurrentGeneration":2}`
	keeper3JSON = `{"uid":"keeper3","listenAddress":"(no db assigned)","healthy":false,"pgHealthy":false,` +
		`"pgWantedGeneration":0,"pgCurrentGeneration":0}`
	typicalKeepersJSON = `[` + keeper1JSON + `,` + keeper2JSON + `,` + keeper3JSON + `]`
	typicalProxiesJSON = `[{"uid":"proxy1","generation":6},{"uid":"proxy2","generation":7}]`
)

// typicalProxies returns two proxies, keyed so that the map order is not the uid order
func typicalProxies() apiv1.ProxiesInfo {
	return apiv1.ProxiesInfo{
		"a": {UID: "proxy2", Generation: 7},
		"b": {UID: "proxy1", Generation: 6},
	}
}

// checkResponse checks the status of w, and its body: the JSON body when status is 200, and a problem otherwise
func checkResponse(t *testing.T, w *httptest.ResponseRecorder, status int, body string) {
	t.Helper()
	if w.Code != status {
		t.Errorf("expected status %d, got %d: %s", status, w.Code, w.Body)
		return
	}
	if status == http.StatusOK {
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
		if got := strings.TrimSpace(w.Body.String()); got != body {
			t.Errorf("expected body %s, got %s", body, got)
		}
		return
	}
	if ct := w.Header().Get("Content-Type"); ct != endpoints.ContentTypeProblem {
		t.Errorf("expected Content-Type %s, got %q", endpoints.ContentTypeProblem, ct)
	}
	var problem endpoints.Problem
	if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil || problem.Status != status {
		t.Errorf("expected a problem with status %d, got %s (%v)", status, w.Body, err)
	}
}

// memberStatus returns the status of a request for one member: 404 when the list was fine but has no such member
func memberStatus(status int, body string) int {
	if status == http.StatusOK && body == "" {
		return http.StatusNotFound
	}
	return status
}

func TestKeeperHandlers(t *testing.T) {
	errBoom := errors.New("boom")

	for name, tc := range map[string]struct {
		cd       *apiv1.Data
		storeErr error
		uid      string
		status   int
		list     string // body of GET /v1/cluster/keepers
		one      string // body of GET /v1/cluster/keepers/{uid}; empty when that is 404
	}{
		"a typical cluster, sorted on uid": {
			cd: typicalClusterData(), uid: "keeper1", status: http.StatusOK, list: typicalKeepersJSON, one: keeper1JSON,
		},
		"a keeper without a db": {
			cd: typicalClusterData(), uid: "keeper3", status: http.StatusOK, list: typicalKeepersJSON, one: keeper3JSON,
		},
		"an unknown keeper": {
			cd: typicalClusterData(), uid: "keeper4", status: http.StatusOK, list: typicalKeepersJSON,
		},
		"no cluster data": {
			uid: "keeper1", status: http.StatusOK, list: `[]`,
		},
		"cluster data without keepers": {
			cd: &apiv1.Data{Cluster: &apiv1.Cluster{}}, uid: "keeper1", status: http.StatusOK, list: `[]`,
		},
		"the cluster data cannot be read": {
			cd: typicalClusterData(), storeErr: errBoom, uid: "keeper1", status: http.StatusInternalServerError,
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			store := mockconsensus.NewMockStore(ctrl)
			store.EXPECT().GetClusterData(gomock.Any()).Return(tc.cd, &consensus.KVPair{}, tc.storeErr).Times(2)
			var ready atomic.Bool
			h := NewHandlers(store, mockconsensus.NewMockElection(ctrl), &ready)

			w := httptest.NewRecorder()
			h.KeepersHandler(w, httptest.NewRequest(http.MethodGet, "/v1/cluster/keepers", nil))
			checkResponse(t, w, tc.status, tc.list)

			r := httptest.NewRequest(http.MethodGet, "/v1/cluster/keepers/"+tc.uid, nil)
			r.SetPathValue("uid", tc.uid)
			w = httptest.NewRecorder()
			h.KeeperHandler(w, r)
			checkResponse(t, w, memberStatus(tc.status, tc.one), tc.one)
		})
	}
}

func TestProxyHandlers(t *testing.T) {
	errBoom := errors.New("boom")

	for name, tc := range map[string]struct {
		proxies  apiv1.ProxiesInfo
		storeErr error
		uid      string
		status   int
		list     string // body of GET /v1/cluster/proxies
		one      string // body of GET /v1/cluster/proxies/{uid}; empty when that is 404
	}{
		"proxies, sorted on uid": {
			proxies: typicalProxies(), uid: "proxy2", status: http.StatusOK, list: typicalProxiesJSON,
			one: `{"uid":"proxy2","generation":7}`,
		},
		"an unknown proxy": {
			proxies: typicalProxies(), uid: "proxy3", status: http.StatusOK, list: typicalProxiesJSON,
		},
		"no proxies": {
			uid: "proxy1", status: http.StatusOK, list: `[]`,
		},
		"the proxies cannot be read": {
			storeErr: errBoom, uid: "proxy1", status: http.StatusInternalServerError,
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			store := mockconsensus.NewMockStore(ctrl)
			store.EXPECT().GetProxiesInfo(gomock.Any()).Return(tc.proxies, tc.storeErr).Times(2)
			var ready atomic.Bool
			h := NewHandlers(store, mockconsensus.NewMockElection(ctrl), &ready)

			w := httptest.NewRecorder()
			h.ProxiesHandler(w, httptest.NewRequest(http.MethodGet, "/v1/cluster/proxies", nil))
			checkResponse(t, w, tc.status, tc.list)

			r := httptest.NewRequest(http.MethodGet, "/v1/cluster/proxies/"+tc.uid, nil)
			r.SetPathValue("uid", tc.uid)
			w = httptest.NewRecorder()
			h.ProxyHandler(w, r)
			checkResponse(t, w, memberStatus(tc.status, tc.one), tc.one)
		})
	}
}

func TestStatusHandler(t *testing.T) {
	errBoom := errors.New("boom")
	typicalStatusJSON := func(sentinels string) string {
		return `{"proxies":` + typicalProxiesJSON + `,"keepers":` + typicalKeepersJSON + `,"sentinels":` + sentinels +
			`,"status":{"available":true,"primaryDB":"db1","primaryKeeper":"keeper1","role":"master"}}`
	}

	// what the handler reads, in this order; it stops at the first read that fails
	const (
		readsClusterData = iota + 1
		readsProxies
		readsSentinels
		readsLeader
	)
	for name, tc := range map[string]struct {
		cd                                    *apiv1.Data
		proxies                               apiv1.ProxiesInfo
		sentinels                             apiv1.SentinelsInfo
		cdErr, proxiesErr, sentinelsErr, lErr error
		reads                                 int
		status                                int
		body                                  string
	}{
		"a typical cluster": {
			cd: typicalClusterData(), proxies: typicalProxies(),
			sentinels: apiv1.SentinelsInfo{{UID: "sentinel2"}, {UID: "sentinel1"}},
			reads:     readsLeader, status: http.StatusOK,
			body: typicalStatusJSON(`[{"uid":"sentinel1","leader":true},{"uid":"sentinel2","leader":false}]`),
		},
		"no leader": {
			cd: typicalClusterData(), proxies: typicalProxies(),
			sentinels: apiv1.SentinelsInfo{{UID: "sentinel2"}, {UID: "sentinel1"}},
			lErr:      consensus.ErrElectionNoLeader, reads: readsLeader, status: http.StatusOK,
			body: typicalStatusJSON(`[{"uid":"sentinel1","leader":false},{"uid":"sentinel2","leader":false}]`),
		},
		"no cluster data and no members": {
			reads: readsLeader, status: http.StatusOK,
			body: `{"proxies":[],"keepers":[],"sentinels":[],"status":{"available":false}}`,
		},
		"the cluster data cannot be read": {
			cdErr: errBoom, reads: readsClusterData, status: http.StatusInternalServerError,
		},
		"the proxies cannot be read": {
			cd: typicalClusterData(), proxiesErr: errBoom, reads: readsProxies, status: http.StatusInternalServerError,
		},
		"the sentinels cannot be read": {
			cd: typicalClusterData(), sentinelsErr: errBoom, reads: readsSentinels,
			status: http.StatusInternalServerError,
		},
		"the leader cannot be read": {
			cd: typicalClusterData(), lErr: errBoom, reads: readsLeader, status: http.StatusInternalServerError,
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			store := mockconsensus.NewMockStore(ctrl)
			election := mockconsensus.NewMockElection(ctrl)
			store.EXPECT().GetClusterData(gomock.Any()).Return(tc.cd, &consensus.KVPair{}, tc.cdErr)
			if tc.reads >= readsProxies {
				store.EXPECT().GetProxiesInfo(gomock.Any()).Return(tc.proxies, tc.proxiesErr)
			}
			if tc.reads >= readsSentinels {
				store.EXPECT().GetSentinelsInfo(gomock.Any()).Return(tc.sentinels, tc.sentinelsErr)
			}
			if tc.reads >= readsLeader {
				leader := "sentinel1"
				if tc.lErr != nil {
					leader = ""
				}
				election.EXPECT().Leader(gomock.Any()).Return(leader, tc.lErr)
			}
			var ready atomic.Bool
			h := NewHandlers(store, election, &ready)

			w := httptest.NewRecorder()
			h.StatusHandler(w, httptest.NewRequest(http.MethodGet, "/v1/cluster/status", nil))
			checkResponse(t, w, tc.status, tc.body)
			if tc.status == http.StatusInternalServerError && !strings.Contains(w.Body.String(), errBoom.Error()) {
				t.Errorf("expected the problem to name the error, got %s", w.Body)
			}
		})
	}
}
