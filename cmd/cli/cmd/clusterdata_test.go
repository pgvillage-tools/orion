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
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// testClusterData is cluster data with a primary (db1 on keeper1) and a standby (db2 on keeper2)
func testClusterData() *apiv1.Data {
	return &apiv1.Data{
		FormatVersion: 1,
		Cluster:       &apiv1.Cluster{UID: "cluster1", Status: apiv1.ClusterStatus{Master: "db1"}},
		Keepers: apiv1.Keepers{
			"keeper1": &apiv1.Keeper{UID: "keeper1"},
			"keeper2": &apiv1.Keeper{UID: "keeper2"},
		},
		DBs: apiv1.DBs{
			"db1": {UID: "db1", Spec: &apiv1.DBSpec{KeeperUID: "keeper1", Followers: []string{"db2"}}},
			"db2": {UID: "db2", Spec: &apiv1.DBSpec{KeeperUID: "keeper2"}},
		},
	}
}

// handleClusterData lets api answer GET /v1/cluster with cd and ETag "7"
func handleClusterData(api *fakeAPI, cd *apiv1.Data) {
	api.handle(endpoints.ClusterEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, cd, `"7"`)
	})
}

var _ = Describe("clusterdata", func() {
	const clusterPath = "/v1/cluster"

	// handlePutClusterData lets api accept PUT /v1/cluster
	handlePutClusterData := func(api *fakeAPI) {
		api.handle(endpoints.ClusterEndPoint.Route(endpoints.MethodPut), func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("ETag", `"8"`)
			w.WriteHeader(http.StatusNoContent)
		})
	}

	// expectPutClusterData fails the spec unless api got one PUT of cluster data with cluster uid and ifMatch
	expectPutClusterData := func(api *fakeAPI, uid, ifMatch string) {
		GinkgoHelper()
		puts := api.requestsTo(http.MethodPut, clusterPath)
		Ω(puts).To(HaveLen(1), "expected one PUT, got %v", api.requests())
		Ω(puts[0].IfMatch).To(Equal(ifMatch))
		var cd apiv1.Data
		Ω(json.Unmarshal([]byte(puts[0].Body), &cd)).To(Succeed())
		Ω(cd.Cluster).NotTo(BeNil(), "expected cluster %s, got %s", uid, puts[0].Body)
		Ω(cd.Cluster.UID).To(Equal(uid))
	}

	var api *fakeAPI

	BeforeEach(func() {
		api = newFakeAPI()
	})

	Context("read", func() {
		It("prints the data and its ETag", func() {
			handleClusterData(api, testClusterData())
			stdout, stderr, err := api.run("clusterdata", "read")
			Ω(err).NotTo(HaveOccurred())
			Ω(stderr).To(Equal("ETag: \"7\"\n"), "expected the ETag on stderr")
			Ω(strings.Count(stdout, "\n")).To(Equal(1), "expected the data on one line, got %q", stdout)
			var cd apiv1.Data
			Ω(json.Unmarshal([]byte(stdout), &cd)).To(Succeed())
			Ω(cd.Cluster.UID).To(Equal("cluster1"))
			Ω(cd.DBs["db1"].Spec.KeeperUID).To(Equal("keeper1"))
		})

		It("pretty prints", func() {
			handleClusterData(api, testClusterData())
			stdout, _, err := api.run("clusterdata", "read", "--pretty")
			Ω(err).NotTo(HaveOccurred())
			expectContains(stdout, "{\n\t\"formatVersion\": 1,\n")
		})

		It("reports an api error", func() {
			api.handle(endpoints.ClusterEndPoint.Route(endpoints.MethodGet),
				func(w http.ResponseWriter, _ *http.Request) {
					endpoints.WriteProblem(w, http.StatusNotFound, "there is no cluster data yet")
				})
			stdout, stderr, err := api.run("clusterdata", "read")
			expectError(err, "failed to get the cluster data: GET "+api.url(clusterPath)+
				": 404 Not Found: there is no cluster data yet", httpCodeSuffix(http.StatusNotFound))
			expectNoUsage(stdout, stderr)
			Ω(stdout).To(BeEmpty(), "expected no output")
		})
	})

	Context("write", func() {
		It("sends the argument", func() {
			handlePutClusterData(api)
			_, _, err := api.run("clusterdata", "write", `{"cluster": {"uid": "arg"}}`)
			Ω(err).NotTo(HaveOccurred())
			expectPutClusterData(api, "arg", "")
		})

		It("sends If-Match", func() {
			handlePutClusterData(api)
			_, _, err := api.run("clusterdata", "write", "--if-match", `"7"`, `{"cluster": {"uid": "arg"}}`)
			Ω(err).NotTo(HaveOccurred())
			expectPutClusterData(api, "arg", `"7"`)
		})

		It("reads a file", func() {
			handlePutClusterData(api)
			file := filepath.Join(GinkgoT().TempDir(), "cd.json")
			Ω(os.WriteFile(file, []byte(`{"cluster": {"uid": "file"}}`), 0o600)).To(Succeed())
			_, _, err := api.run("clusterdata", "write", "-f", file)
			Ω(err).NotTo(HaveOccurred())
			expectPutClusterData(api, "file", "")
		})

		It("reads stdin", func() {
			handlePutClusterData(api)
			_, _, err := api.runWith(context.Background(), `{"cluster": {"uid": "stdin"}}`, "clusterdata", "write",
				"--file", "-")
			Ω(err).NotTo(HaveOccurred())
			expectPutClusterData(api, "stdin", "")
		})

		DescribeTable("refuses invalid input",
			func(args []string, expected string) {
				missing := filepath.Join(GinkgoT().TempDir(), "missing.json")
				stdout, stderr, err := api.run(append([]string{"clusterdata", "write"},
					withMissingFile(missing, args...)...)...)
				expectError(err, withMissingFile(missing, expected)[0], "")
				expectNoUsage(stdout, stderr)
				expectNoRequests(api)
			},
			Entry("no data", nil, "no cluster data provided: pass it as an argument or via --file"),
			Entry("two arguments", []string{"{}", "{}"}, "clusterdata write accepts at most one positional argument"),
			Entry("invalid json", []string{"{"}, "invalid cluster data: "),
			Entry("missing file", []string{"--file", missingFile}, "cannot read "+missingFile+": "),
		)

		It("reports an api error", func() {
			api.handle(endpoints.ClusterEndPoint.Route(endpoints.MethodPut),
				func(w http.ResponseWriter, _ *http.Request) {
					endpoints.WriteProblem(w, http.StatusPreconditionFailed, "it changed since you read it")
				})
			_, _, err := api.run("clusterdata", "write", "--if-match", `"6"`, `{}`)
			expectError(err, "failed to write the cluster data: PUT "+api.url(clusterPath)+
				": 412 Precondition Failed: it changed since you read it",
				httpCodeSuffix(http.StatusPreconditionFailed))
		})
	})
})
