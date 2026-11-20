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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// newInitAPI returns a fake api without a cluster, that accepts POST /v1/cluster
func newInitAPI() *fakeAPI {
	api := newFakeAPI()
	api.handle(endpoints.ClusterEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
		endpoints.WriteProblem(w, http.StatusNotFound, "there is no cluster data yet")
	})
	api.handle(endpoints.ClusterEndPoint.Route(endpoints.MethodPost), accepted)
	return api
}

// expectPostedSpec fails the spec unless api got one POST of a spec, and returns that spec
func expectPostedSpec(api *fakeAPI) apiv1.Spec {
	GinkgoHelper()
	posts := api.requestsTo(http.MethodPost, clusterPath)
	Ω(posts).To(HaveLen(1), "expected one POST, got %v", api.requests())
	var spec apiv1.Spec
	Ω(json.Unmarshal([]byte(posts[0].Body), &spec)).To(Succeed())
	return spec
}

var _ = Describe("init", func() {
	Context("without a cluster", func() {
		var api *fakeAPI

		BeforeEach(func() {
			api = newInitAPI()
		})

		It("posts the spec of the argument", func() {
			stdout, _, err := api.run("init", `{"initMode": "new", "maxStandbys": 3}`)
			Ω(err).NotTo(HaveOccurred())
			Ω(stdout).To(BeEmpty(), "expected no output without --wait")
			spec := expectPostedSpec(api)
			Ω(spec.InitMode).To(HaveValue(Equal(apiv1.New)))
			Ω(spec.MaxStandbys).To(HaveValue(BeEquivalentTo(3)))
		})

		It("reads the spec from stdin with a dash", func() {
			_, _, err := api.runWith(context.Background(), `{"maxStandbys": 5}`, "init", "--file", "-")
			Ω(err).NotTo(HaveOccurred())
			Ω(expectPostedSpec(api).MaxStandbys).To(HaveValue(BeEquivalentTo(5)))
		})

		DescribeTable("creates a new cluster with defaults without a spec",
			func(stdin string, args []string) {
				_, _, err := api.runWith(context.Background(), stdin, append([]string{"init"}, args...)...)
				Ω(err).NotTo(HaveOccurred())
				spec := expectPostedSpec(api)
				Ω(spec.InitMode).To(HaveValue(Equal(apiv1.New)), "expected a new cluster")
				Ω(spec.MaxStandbys).To(BeNil(), "expected the default settings")
			},
			// it doesn't read stdin without --file -: at a terminal it would wait for input
			Entry("no arguments, stdin ignored", `{"maxStandbys": 5}`, nil),
			Entry("empty stdin", "", []string{"--file", "-"}),
			Entry("stdin with only whitespace", " \n", []string{"--file", "-"}),
		)

		It("defaults the init mode to new", func() {
			_, _, err := api.run("init", `{"maxStandbys": 3}`)
			Ω(err).NotTo(HaveOccurred())
			Ω(expectPostedSpec(api).InitMode).To(HaveValue(Equal(apiv1.New)))
		})

		It("keeps another init mode", func() {
			_, _, err := api.run("init", `{"initMode": "existing", "existingConfig": {"keeperUID": "keeper1"}}`)
			Ω(err).NotTo(HaveOccurred())
			Ω(expectPostedSpec(api).InitMode).To(HaveValue(Equal(apiv1.ExistingCluster)))
		})

		It("reads the spec from a file", func() {
			file := filepath.Join(GinkgoT().TempDir(), "spec.json")
			Ω(os.WriteFile(file, []byte(`{"maxStandbys": 7}`), 0o600)).To(Succeed())
			_, _, err := api.run("init", "--file", file)
			Ω(err).NotTo(HaveOccurred())
			Ω(expectPostedSpec(api).MaxStandbys).To(HaveValue(BeEquivalentTo(7)))
		})

		DescribeTable("refuses invalid input",
			func(args []string, expected string) {
				missing := filepath.Join(GinkgoT().TempDir(), "missing.json")
				_, _, err := api.run(append([]string{"init"}, withMissingFile(missing, args...)...)...)
				expectError(err, withMissingFile(missing, expected)[0], "")
				expectNoRequests(api)
			},
			Entry("two arguments", []string{"{}", "{}"}, "too many arguments"),
			Entry("invalid json", []string{"{"}, "invalid cluster spec: "),
			Entry("missing file", []string{"--file", missingFile}, "cannot read "+missingFile+": "),
			Entry("an argument and a file", []string{"{}", "--file", missingFile},
				"pass the cluster spec as an argument or with --file, not both"),
		)

		It("waits until the cluster is available", func() {
			handleStatusSequence(api, unavailable(), available("keeper1"))
			stdout, _, err := api.run("init", "--wait", `{}`)
			Ω(err).NotTo(HaveOccurred())
			Ω(stdout).To(Equal("the cluster applied the change\n"))
			Ω(api.requestsTo(http.MethodGet, statusPath)).To(HaveLen(2))
		})
	})

	It("refuses an existing cluster", func() {
		api := newFakeAPI()
		handleClusterData(api, testClusterData())
		stdout, stderr, err := api.run("init", `{}`)
		expectError(err, "cannot initialize an already existing cluster",
			"cannot initialize an already existing cluster")
		expectNoUsage(stdout, stderr)
		Ω(api.requestsTo(http.MethodPost, clusterPath)).To(BeEmpty(), "expected no POST for an existing cluster")
	})

	It("reports a failed read", func() {
		api := newFakeAPI()
		api.handle(endpoints.ClusterEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
			endpoints.WriteProblem(w, http.StatusInternalServerError, "the store doesn't answer")
		})
		_, _, err := api.run("init", `{}`)
		expectError(err, "failed to get the cluster data: GET "+api.url(clusterPath)+
			": 500 Internal Server Error: the store doesn't answer", httpCodeSuffix(http.StatusInternalServerError))
	})

	It("reports a failed post", func() {
		api := newFakeAPI()
		api.handle(endpoints.ClusterEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
			endpoints.WriteProblem(w, http.StatusNotFound, "there is no cluster data yet")
		})
		api.handle(endpoints.ClusterEndPoint.Route(endpoints.MethodPost), func(w http.ResponseWriter, _ *http.Request) {
			endpoints.WriteProblem(w, http.StatusBadRequest, "invalid spec: maxStandbys can't be 0")
		})
		_, _, err := api.run("init", `{"maxStandbys": 0}`)
		expectError(err, "init failed: POST "+api.url(clusterPath)+
			": 400 Bad Request: invalid spec: maxStandbys can't be 0", httpCodeSuffix(http.StatusBadRequest))
	})
})
