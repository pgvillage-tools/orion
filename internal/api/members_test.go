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

var _ = Describe("the member endpoints", func() {
	// typicalClusterData returns cluster data with a primary db1 on keeper1 (that listens on 10.0.0.1:5432), a standby
	// db2 on keeper2 that is not converged and has not reported its address yet, and keeper3 without a db
	typicalClusterData := func() *apiv1.Data {
		return &apiv1.Data{
			Cluster: &apiv1.Cluster{Spec: &apiv1.Spec{}, Status: apiv1.ClusterStatus{Master: "db1"}},
			Keepers: apiv1.Keepers{
				"keeper3": {UID: "keeper3"},
				"keeper1": {UID: "keeper1", Status: apiv1.KeeperStatus{Healthy: true}},
				"keeper2": {UID: "keeper2", Status: apiv1.KeeperStatus{Healthy: true}},
			},
			DBs: apiv1.DBs{
				"db1": {UID: "db1", Generation: 2, Spec: &apiv1.DBSpec{KeeperUID: "keeper1"},
					Status: apiv1.DBStatus{Healthy: true, CurrentGeneration: 2, ListenAddress: "10.0.0.1",
						Port: "5432"}},
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
	typicalProxies := func() apiv1.ProxiesInfo {
		return apiv1.ProxiesInfo{
			"a": {UID: "proxy2", Generation: 7},
			"b": {UID: "proxy1", Generation: 6},
		}
	}

	// checkResponse checks the status of w, and its body: the JSON body when status is 200, and a problem otherwise
	checkResponse := func(w *httptest.ResponseRecorder, status int, body string) {
		GinkgoHelper()
		Expect(w.Code).To(Equal(status), "body: %s", w.Body)
		if status == http.StatusOK {
			Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))
			Expect(strings.TrimSpace(w.Body.String())).To(Equal(body))
			return
		}
		Expect(w.Header().Get("Content-Type")).To(Equal(endpoints.ContentTypeProblem))
		var problem endpoints.Problem
		Expect(json.Unmarshal(w.Body.Bytes(), &problem)).To(Succeed(), "expected a problem, got %s", w.Body)
		Expect(problem.Status).To(Equal(status), "expected a problem with status %d", status)
	}

	// memberStatus returns the status of a request for one member: 404 when the list was fine but has no such member
	memberStatus := func(status int, body string) int {
		if status == http.StatusOK && body == "" {
			return http.StatusNotFound
		}
		return status
	}

	Describe("keepers", func() {
		errBoom := errors.New("boom")

		// list is the body of GET /v1/cluster/keepers, one the body of GET /v1/cluster/keepers/{uid} (empty when that
		// is 404)
		DescribeTable("KeepersHandler and KeeperHandler",
			func(cd *apiv1.Data, storeErr error, uid string, status int, list, one string) {
				ctrl := gomock.NewController(GinkgoT())
				store := mockconsensus.NewMockStore(ctrl)
				store.EXPECT().GetClusterData(gomock.Any()).Return(cd, &consensus.KVPair{}, storeErr).Times(2)
				var ready atomic.Bool
				h := NewHandlers(store, mockconsensus.NewMockElection(ctrl), &ready)

				w := httptest.NewRecorder()
				h.KeepersHandler(w, httptest.NewRequest(http.MethodGet, "/v1/cluster/keepers", nil))
				checkResponse(w, status, list)

				r := httptest.NewRequest(http.MethodGet, "/v1/cluster/keepers/"+uid, nil)
				r.SetPathValue("uid", uid)
				w = httptest.NewRecorder()
				h.KeeperHandler(w, r)
				checkResponse(w, memberStatus(status, one), one)
			},
			Entry("a typical cluster, sorted on uid",
				typicalClusterData(), nil, "keeper1", http.StatusOK, typicalKeepersJSON, keeper1JSON),
			Entry("a keeper without a db",
				typicalClusterData(), nil, "keeper3", http.StatusOK, typicalKeepersJSON, keeper3JSON),
			Entry("an unknown keeper",
				typicalClusterData(), nil, "keeper4", http.StatusOK, typicalKeepersJSON, ""),
			Entry("no cluster data",
				nil, nil, "keeper1", http.StatusOK, `[]`, ""),
			Entry("cluster data without keepers",
				&apiv1.Data{Cluster: &apiv1.Cluster{}}, nil, "keeper1", http.StatusOK, `[]`, ""),
			Entry("the cluster data cannot be read",
				typicalClusterData(), errBoom, "keeper1", http.StatusInternalServerError, "", ""),
		)
	})

	Describe("proxies", func() {
		errBoom := errors.New("boom")

		// list is the body of GET /v1/cluster/proxies, one the body of GET /v1/cluster/proxies/{uid} (empty when that
		// is 404)
		DescribeTable("ProxiesHandler and ProxyHandler",
			func(proxies apiv1.ProxiesInfo, storeErr error, uid string, status int, list, one string) {
				ctrl := gomock.NewController(GinkgoT())
				store := mockconsensus.NewMockStore(ctrl)
				store.EXPECT().GetProxiesInfo(gomock.Any()).Return(proxies, storeErr).Times(2)
				var ready atomic.Bool
				h := NewHandlers(store, mockconsensus.NewMockElection(ctrl), &ready)

				w := httptest.NewRecorder()
				h.ProxiesHandler(w, httptest.NewRequest(http.MethodGet, "/v1/cluster/proxies", nil))
				checkResponse(w, status, list)

				r := httptest.NewRequest(http.MethodGet, "/v1/cluster/proxies/"+uid, nil)
				r.SetPathValue("uid", uid)
				w = httptest.NewRecorder()
				h.ProxyHandler(w, r)
				checkResponse(w, memberStatus(status, one), one)
			},
			Entry("proxies, sorted on uid",
				typicalProxies(), nil, "proxy2", http.StatusOK, typicalProxiesJSON, `{"uid":"proxy2","generation":7}`),
			Entry("an unknown proxy",
				typicalProxies(), nil, "proxy3", http.StatusOK, typicalProxiesJSON, ""),
			Entry("no proxies",
				nil, nil, "proxy1", http.StatusOK, `[]`, ""),
			Entry("the proxies cannot be read",
				nil, errBoom, "proxy1", http.StatusInternalServerError, "", ""),
		)
	})

	Describe("GET /v1/cluster/status", func() {
		errBoom := errors.New("boom")
		typicalStatusJSON := func(sentinels string) string {
			return `{"proxies":` + typicalProxiesJSON + `,"keepers":` + typicalKeepersJSON +
				`,"sentinels":` + sentinels +
				`,"status":{"available":true,"primaryDB":"db1","primaryKeeper":"keeper1","role":"master"}}`
		}

		// what the handler reads, in this order; it stops at the first read that fails
		const (
			readsClusterData = iota + 1
			readsProxies
			readsSentinels
			readsLeader
		)
		type statusCase struct {
			cd                                    *apiv1.Data
			proxies                               apiv1.ProxiesInfo
			sentinels                             apiv1.SentinelsInfo
			cdErr, proxiesErr, sentinelsErr, lErr error
			reads                                 int
			status                                int
			body                                  string
		}

		DescribeTable("StatusHandler",
			func(tc statusCase) {
				ctrl := gomock.NewController(GinkgoT())
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
				checkResponse(w, tc.status, tc.body)
				// the store error is logged, not returned: it can name store internals (endpoints, keys)
				if tc.status == http.StatusInternalServerError {
					Expect(w.Body.String()).To(ContainSubstring("failed to get the cluster status"),
						"expected a fixed problem detail")
					Expect(w.Body.String()).NotTo(ContainSubstring(errBoom.Error()),
						"expected a problem detail without the store error")
				}
			},
			Entry("a typical cluster", statusCase{
				cd: typicalClusterData(), proxies: typicalProxies(),
				sentinels: apiv1.SentinelsInfo{{UID: "sentinel2"}, {UID: "sentinel1"}},
				reads:     readsLeader, status: http.StatusOK,
				body: typicalStatusJSON(`[{"uid":"sentinel1","leader":true},{"uid":"sentinel2","leader":false}]`),
			}),
			Entry("no leader", statusCase{
				cd: typicalClusterData(), proxies: typicalProxies(),
				sentinels: apiv1.SentinelsInfo{{UID: "sentinel2"}, {UID: "sentinel1"}},
				lErr:      consensus.ErrElectionNoLeader, reads: readsLeader, status: http.StatusOK,
				body: typicalStatusJSON(`[{"uid":"sentinel1","leader":false},{"uid":"sentinel2","leader":false}]`),
			}),
			Entry("no cluster data and no members", statusCase{
				reads: readsLeader, status: http.StatusOK,
				body: `{"proxies":[],"keepers":[],"sentinels":[],"status":{"available":false}}`,
			}),
			Entry("the cluster data cannot be read", statusCase{
				cdErr: errBoom, reads: readsClusterData, status: http.StatusInternalServerError,
			}),
			Entry("the proxies cannot be read", statusCase{
				cd: typicalClusterData(), proxiesErr: errBoom, reads: readsProxies,
				status: http.StatusInternalServerError,
			}),
			Entry("the sentinels cannot be read", statusCase{
				cd: typicalClusterData(), sentinelsErr: errBoom, reads: readsSentinels,
				status: http.StatusInternalServerError,
			}),
			Entry("the leader cannot be read", statusCase{
				cd: typicalClusterData(), lErr: errBoom, reads: readsLeader, status: http.StatusInternalServerError,
			}),
		)
	})
})
