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

package cmd

import (
	"encoding/json"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

const statusPath = "/v1/cluster/status"

var _ = Describe("status", func() {
	// testStatus is the status of a cluster with a primary on keeper1 and a standby on keeper2, as the api returns it
	testStatus := func() *apiv1.InfoCluster {
		return &apiv1.InfoCluster{
			Sentinels: apiv1.InfoSentinels{{UID: "sentinel1", Leader: true}, {UID: "sentinel2"}},
			Proxies:   apiv1.InfoProxies{{UID: "proxy1", Generation: 4}},
			Keepers: apiv1.InfoKeepers{
				{UID: "keeper1", ListenAddress: "10.0.0.1:5432", Healthy: true, PgHealthy: true,
					PgWantedGeneration: 3, PgCurrentGeneration: 3},
				{UID: "keeper2", ListenAddress: "10.0.0.2:5432", Healthy: false, PgHealthy: false,
					PgWantedGeneration: 2, PgCurrentGeneration: 1},
			},
			Status: apiv1.InfoGenericStatus{"available": true, "primaryDB": "db1", "primaryKeeper": "keeper1",
				"role": "primary"},
		}
	}

	// handleStatus lets api answer GET /v1/cluster/status with ic
	handleStatus := func(api *fakeAPI, ic *apiv1.InfoCluster) {
		api.handle(endpoints.StatusEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, ic, "")
		})
	}

	// lines returns the lines of s, with the trailing spaces and tabs of every line removed
	lines := func(s string) []string {
		result := strings.Split(s, "\n")
		for i, l := range result {
			result[i] = strings.TrimRight(l, " \t")
		}
		return result
	}

	// expectLines fails the spec unless output has expected as consecutive lines
	expectLines := func(output string, expected ...string) {
		GinkgoHelper()
		Ω(strings.Join(lines(output), "\n")).To(ContainSubstring(strings.Join(expected, "\n")))
	}

	// columns returns the line of output that starts with first, split on whitespace
	columns := func(output, first string) []string {
		GinkgoHelper()
		for _, l := range lines(output) {
			if f := strings.Fields(l); len(f) > 0 && f[0] == first {
				return f
			}
		}
		Fail("no line starting with " + first + " in\n" + output)
		return nil
	}

	// expandTabs replaces the tabs in line with spaces up to the next tab stop, like a terminal
	expandTabs := func(line string, width int) string {
		var b strings.Builder
		for _, r := range line {
			if r == '\t' {
				b.WriteString(strings.Repeat(" ", width-b.Len()%width))
				continue
			}
			b.WriteRune(r)
		}
		return b.String()
	}

	var api *fakeAPI

	BeforeEach(func() {
		api = newFakeAPI()
	})

	Context("as text", func() {
		It("prints the cluster", func() {
			handleStatus(api, testStatus())
			handleClusterData(api, testClusterData())
			stdout, _, err := api.run("status")
			Ω(err).NotTo(HaveOccurred())
			expectLines(stdout, "=== Active sentinels ===", "")
			Ω(columns(stdout, "ID")).To(Equal([]string{"ID", "LEADER"}), "unexpected sentinel header")
			Ω(columns(stdout, "sentinel1")).To(Equal([]string{"sentinel1", "true"}))
			Ω(columns(stdout, "sentinel2")).To(Equal([]string{"sentinel2", "false"}))
			expectLines(stdout, "=== Active proxies ===", "", "ID", "proxy1")
			expectLines(stdout, "=== Keepers ===", "")
			Ω(strings.Join(columns(stdout, "UID"), " ")).To(
				Equal("UID HEALTHY PG LISTENADDRESS PG HEALTHY PG WANTEDGENERATION PG CURRENTGENERATION"),
				"unexpected keeper header")
			Ω(columns(stdout, "keeper1")).To(Equal([]string{"keeper1", "true", "10.0.0.1:5432", "true", "3", "3"}))
			Ω(columns(stdout, "keeper2")).To(Equal([]string{"keeper2", "false", "10.0.0.2:5432", "false", "2", "1"}))
			expectLines(stdout, "=== Cluster Info ===", "", "Cluster role: primary", "Master Keeper: keeper1")
			expectLines(stdout, "===== Keepers/DB tree =====", "", "keeper1 (master)", "└─keeper2", "")
		})

		It("prints an empty cluster", func() {
			handleStatus(api, &apiv1.InfoCluster{Status: apiv1.InfoGenericStatus{"available": false}})
			handleClusterData(api, &apiv1.Data{})
			stdout, _, err := api.run("status", "--format", "text")
			Ω(err).NotTo(HaveOccurred())
			expectLines(stdout,
				"=== Active sentinels ===", "", "No active sentinels", "",
				"=== Active proxies ===", "", "No active proxies", "",
				"=== Keepers ===", "", "No keepers available", "",
				"No cluster available", "")
			Ω(stdout).NotTo(ContainSubstring("Keepers/DB tree"), "expected no tree without a primary")
		})

		It("prints the tree of followers", func() {
			cd := testClusterData()
			// db1 has followers db2 and db3, db2 has db7 and db8, db3 has db4, which has db6; db5 isn't in the
			// cluster data (a removed keeper)
			cd.DBs["db1"].Spec.Followers = []string{"db2", "db5", "db3"}
			cd.DBs["db3"] = &apiv1.DB{UID: "db3", Spec: &apiv1.DBSpec{KeeperUID: "keeper3", Followers: []string{"db4"}}}
			cd.DBs["db4"] = &apiv1.DB{UID: "db4", Spec: &apiv1.DBSpec{KeeperUID: "keeper4", Followers: []string{"db6"}}}
			cd.DBs["db6"] = &apiv1.DB{UID: "db6", Spec: &apiv1.DBSpec{KeeperUID: "keeper6"}}
			cd.DBs["db2"].Spec.Followers = []string{"db7", "db8"}
			cd.DBs["db7"] = &apiv1.DB{UID: "db7", Spec: &apiv1.DBSpec{KeeperUID: "keeper7"}}
			cd.DBs["db8"] = &apiv1.DB{UID: "db8", Spec: &apiv1.DBSpec{KeeperUID: "keeper8"}}
			handleStatus(api, testStatus())
			handleClusterData(api, cd)
			stdout, _, err := api.run("status")
			Ω(err).NotTo(HaveOccurred())
			expectLines(stdout, "===== Keepers/DB tree =====", "",
				"keeper1 (master)",
				"├─keeper2",
				"│ ├─keeper7",
				"│ └─keeper8",
				"└─keeper3",
				"  └─keeper4",
				"    └─keeper6",
				"")
		})

		It("ends the tree at the last printed follower", func() {
			cd := testClusterData()
			// the last follower (db5) isn't in the cluster data, and db9 has no spec: keeper3 is the last one printed
			cd.DBs["db1"].Spec.Followers = []string{"db2", "db3", "db9", "db5"}
			cd.DBs["db3"] = &apiv1.DB{UID: "db3", Spec: &apiv1.DBSpec{KeeperUID: "keeper3"}}
			cd.DBs["db9"] = &apiv1.DB{UID: "db9"}
			handleStatus(api, testStatus())
			handleClusterData(api, cd)
			stdout, _, err := api.run("status")
			Ω(err).NotTo(HaveOccurred())
			expectLines(stdout, "===== Keepers/DB tree =====", "",
				"keeper1 (master)",
				"├─keeper2",
				"└─keeper3",
				"")
		})

		DescribeTable("prints a tree without cluster or specs",
			func(change func(cd *apiv1.Data)) {
				cd := testClusterData()
				change(cd)
				handleStatus(api, testStatus())
				handleClusterData(api, cd)
				_, _, err := api.run("status")
				Ω(err).NotTo(HaveOccurred())
			},
			Entry("cluster data without a cluster", func(cd *apiv1.Data) { cd.Cluster = nil }),
			Entry("a primary db without a spec", func(cd *apiv1.Data) { cd.DBs["db1"].Spec = nil }),
			Entry("a primary db that is missing", func(cd *apiv1.Data) { delete(cd.DBs, "db1") }),
		)

		It("aligns the columns of a table", func() {
			status := testStatus()
			status.Keepers = []apiv1.InfoKeeper{
				{UID: "k1", ListenAddress: "keeper1:5432", Healthy: true},
				{UID: "a-much-longer-keeper-uid", ListenAddress: "10.0.0.2:5432", Healthy: true},
			}
			handleStatus(api, status)
			handleClusterData(api, testClusterData())
			stdout, _, err := api.run("status")
			Ω(err).NotTo(HaveOccurred())
			// every row of the keepers table starts its second column (HEALTHY) at the same position on a terminal
			var positions []int
			for _, line := range strings.Split(stdout, "\n") {
				for _, first := range []string{"UID\t", "k1\t", "a-much-longer-keeper-uid\t"} {
					if strings.HasPrefix(line, first) {
						expanded := expandTabs(line, tabWidth)
						rest := expanded[len(first)-1:]
						positions = append(positions, len(expanded)-len(strings.TrimLeft(rest, " ")))
					}
				}
			}
			Ω(positions).To(HaveLen(3), "expected the 3 keeper lines in\n%s", stdout)
			Ω(positions).To(HaveEach(positions[0]),
				"expected the second column of the 3 keeper lines at the same position in\n%s", stdout)
		})
	})

	It("prints the cluster as json", func() {
		handleStatus(api, testStatus())
		stdout, _, err := api.run("status", "-f", "json")
		Ω(err).NotTo(HaveOccurred())
		// the status as the api returned it; it used to marshal the status function instead, and failed
		var got apiv1.InfoCluster
		Ω(json.Unmarshal([]byte(stdout), &got)).To(Succeed(), "expected the status as json, got %q", stdout)
		expectedJSON, err := json.Marshal(testStatus())
		Ω(err).NotTo(HaveOccurred())
		gotJSON, err := json.Marshal(got)
		Ω(err).NotTo(HaveOccurred())
		Ω(string(gotJSON)).To(Equal(string(expectedJSON)))
		expectContains(stdout, "\n\t\"proxies\": [")
		Ω(api.requestsTo(http.MethodGet, clusterPath)).To(BeEmpty(), "expected json not to need the cluster data")
	})

	It("refuses an unknown format", func() {
		stdout, stderr, err := api.run("status", "--format", "yaml")
		expectError(err, "unrecognised output format yaml", "unrecognised output format yaml")
		expectNoUsage(stdout, stderr)
		expectNoRequests(api)
	})

	It("reports a failed status", func() {
		api.handle(endpoints.StatusEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
			endpoints.WriteProblem(w, http.StatusServiceUnavailable, "the store doesn't answer")
		})
		_, _, err := api.run("status")
		expectError(err, "failed to get the cluster status: GET "+api.url(statusPath)+
			": 503 Service Unavailable: the store doesn't answer", httpCodeSuffix(http.StatusServiceUnavailable))
	})

	It("reports failed cluster data", func() {
		handleStatus(api, testStatus())
		api.handle(endpoints.ClusterEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
			endpoints.WriteProblem(w, http.StatusInternalServerError, "")
		})
		_, _, err := api.run("status")
		expectError(err, "failed to get the cluster data: GET "+api.url(clusterPath)+": 500 Internal Server Error",
			httpCodeSuffix(http.StatusInternalServerError))
	})
})
