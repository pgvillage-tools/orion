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

const specPath = "/v1/cluster/spec"

var _ = Describe("spec", func() {
	// handleSpec lets api answer GET /v1/cluster/spec with spec and ETag "abc"
	handleSpec := func(api *fakeAPI, spec string) {
		api.handle(endpoints.ClusterSpecEndPoint.Route(endpoints.MethodGet),
			func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("ETag", `"abc"`)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(spec))
			})
	}

	var api *fakeAPI

	BeforeEach(func() {
		api = newFakeAPI()
	})

	It("prints the set values and the ETag", func() {
		handleSpec(api, `{"maxStandbys": 4, "pgParameters": {"max_connections": "200"}}`)
		stdout, stderr, err := api.run("spec")
		Ω(err).NotTo(HaveOccurred())
		Ω(stderr).To(Equal("ETag: \"abc\"\n"), "expected the ETag on stderr")
		expected := "{\n\t\"maxStandbys\": 4,\n\t\"pgParameters\": {\n\t\t\"max_connections\": \"200\"\n\t}\n}\n"
		Ω(stdout).To(Equal(expected))
	})

	It("prints the defaults", func() {
		handleSpec(api, `{"maxStandbys": 4}`)
		stdout, _, err := api.run("spec", "--defaults")
		Ω(err).NotTo(HaveOccurred())
		var spec map[string]any
		Ω(json.Unmarshal([]byte(stdout), &spec)).To(Succeed())
		Ω(spec).To(HaveKeyWithValue("maxStandbys", float64(4)), "expected the set value")
		defaults := (&apiv1.Spec{}).WithDefaults()
		Ω(spec).To(HaveKeyWithValue("sleepInterval", defaults.SleepInterval.String()), "expected the defaults")
		Ω(spec).To(HaveKeyWithValue("role", string(*defaults.Role)), "expected the defaults")
		Ω(spec).To(HaveKey("standbyConfig"), "expected every field, also the unset ones")
	})

	It("reports an api error", func() {
		api.handle(endpoints.ClusterSpecEndPoint.Route(endpoints.MethodGet),
			func(w http.ResponseWriter, _ *http.Request) {
				endpoints.WriteProblem(w, http.StatusNotFound, "there is no cluster spec yet")
			})
		stdout, stderr, err := api.run("spec")
		expectError(err, "failed to get the cluster spec: GET "+api.url(specPath)+
			": 404 Not Found: there is no cluster spec yet", httpCodeSuffix(http.StatusNotFound))
		expectNoUsage(stdout, stderr)
	})
})

var _ = Describe("update", func() {
	// handleSpecUpdates lets api accept PUT and PATCH of /v1/cluster/spec
	handleSpecUpdates := func(api *fakeAPI) {
		for _, method := range []endpoints.Method{endpoints.MethodPut, endpoints.MethodPatch} {
			api.handle(endpoints.ClusterSpecEndPoint.Route(method), accepted)
		}
	}

	// expectSpecUpdate fails the spec unless api got one method request of the spec, and returns it
	expectSpecUpdate := func(api *fakeAPI, method string) recordedRequest {
		GinkgoHelper()
		reqs := api.requests()
		Ω(reqs).To(HaveLen(1), "expected one %s of %s, got %v", method, specPath, reqs)
		Ω(reqs[0].Method).To(Equal(method))
		Ω(reqs[0].Path).To(Equal(specPath))
		return reqs[0]
	}

	// expectMaxStandbys fails the spec unless req has a spec with maxStandbys expected
	expectMaxStandbys := func(req recordedRequest, expected uint16) {
		GinkgoHelper()
		var spec apiv1.Spec
		Ω(json.Unmarshal([]byte(req.Body), &spec)).To(Succeed())
		Ω(spec.MaxStandbys).To(HaveValue(Equal(expected)), "expected maxStandbys %d, got %s", expected, req.Body)
	}

	var api *fakeAPI

	BeforeEach(func() {
		api = newFakeAPI()
	})

	It("replaces the spec", func() {
		handleSpecUpdates(api)
		stdout, _, err := api.run("update", `{"maxStandbys": 2}`)
		Ω(err).NotTo(HaveOccurred())
		Ω(stdout).To(BeEmpty(), "expected no output without --wait")
		req := expectSpecUpdate(api, http.MethodPut)
		Ω(req.ContentType).To(Equal("application/json"))
		Ω(req.IfMatch).To(BeEmpty())
		expectMaxStandbys(req, 2)
	})

	It("patches the spec with If-Match", func() {
		handleSpecUpdates(api)
		// a patch is sent as it is: null removes a value
		patch := `{"maxStandbys": null}`
		_, _, err := api.run("update", "--patch", "--if-match", `"abc"`, patch)
		Ω(err).NotTo(HaveOccurred())
		req := expectSpecUpdate(api, http.MethodPatch)
		Ω(req.Body).To(Equal(patch))
		Ω(req.ContentType).To(Equal(endpoints.ContentTypeMergePatch))
		Ω(req.IfMatch).To(Equal(`"abc"`))
	})

	It("reads a file", func() {
		handleSpecUpdates(api)
		file := filepath.Join(GinkgoT().TempDir(), "patch.json")
		Ω(os.WriteFile(file, []byte(`{"usePgrewind": true}`), 0o600)).To(Succeed())
		_, _, err := api.run("update", "-P", "-f", file)
		Ω(err).NotTo(HaveOccurred())
		Ω(expectSpecUpdate(api, http.MethodPatch).Body).To(Equal(`{"usePgrewind": true}`))
	})

	It("reads stdin", func() {
		handleSpecUpdates(api)
		_, _, err := api.runWith(context.Background(), `{"maxStandbys": 6}`, "update", "--file", "-",
			"--if-match", `"abc"`)
		Ω(err).NotTo(HaveOccurred())
		req := expectSpecUpdate(api, http.MethodPut)
		Ω(req.IfMatch).To(Equal(`"abc"`))
		expectMaxStandbys(req, 6)
	})

	It("waits", func() {
		handleSpecUpdates(api)
		handleStatusSequence(api, &apiv1.InfoCluster{
			Keepers: apiv1.InfoKeepers{{UID: "keeper1", Healthy: true, PgWantedGeneration: 3, PgCurrentGeneration: 2}},
			Status:  apiv1.InfoGenericStatus{"available": true, "primaryKeeper": "keeper1"},
		}, available("keeper1"))
		stdout, _, err := api.run("update", "--patch", "--wait", `{"maxStandbys": 2}`)
		Ω(err).NotTo(HaveOccurred())
		Ω(stdout).To(Equal("the cluster applied the change\n"))
		Ω(api.requestsTo(http.MethodGet, statusPath)).To(HaveLen(2))
	})

	It("reports an api error", func() {
		api.handle(endpoints.ClusterSpecEndPoint.Route(endpoints.MethodPatch),
			func(w http.ResponseWriter, _ *http.Request) {
				endpoints.WriteProblem(w, http.StatusPreconditionFailed, "it changed since you read it")
			})
		stdout, stderr, err := api.run("update", "--patch", "--if-match", `"old"`, `{}`)
		expectError(err, "failed to update the cluster spec: PATCH "+api.url(specPath)+
			": 412 Precondition Failed: it changed since you read it", httpCodeSuffix(http.StatusPreconditionFailed))
		expectNoUsage(stdout, stderr)
	})

	DescribeTable("refuses invalid input",
		func(args []string, expected string) {
			missing := filepath.Join(GinkgoT().TempDir(), "missing.json")
			stdout, stderr, err := api.run(append([]string{"update"}, withMissingFile(missing, args...)...)...)
			expectError(err, withMissingFile(missing, expected)[0], "")
			expectNoUsage(stdout, stderr)
			expectNoRequests(api)
		},
		Entry("no spec", nil, "no cluster spec provided as argument and no file provided (--file/-f option)"),
		Entry("argument and file", []string{"--file", missingFile, "{}"},
			"only one of cluster spec provided as argument or file"),
		Entry("two arguments", []string{"{}", "{}"}, "too many arguments"),
		Entry("invalid json", []string{"{"}, "invalid cluster spec: "),
		Entry("missing file", []string{"--file", missingFile}, "cannot read "+missingFile+": "),
	)
})
