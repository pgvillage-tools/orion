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
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// expectOnlyRequest fails the spec unless api received one request, with method to path
func expectOnlyRequest(api *fakeAPI, method, path string) {
	GinkgoHelper()
	Ω(api.requests()).To(ConsistOf(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Method": Equal(method),
		"Path":   Equal(path),
	})), "expected one %s of %s", method, path)
}

var _ = Describe("keeper commands", func() {
	const statusPath = "/v1/cluster/status"
	removeKeeperRoute := endpoints.KeeperEndPoint.Route(endpoints.MethodDelete)

	const (
		failKeeper1Path = "/v1/cluster/keepers/keeper1/actions/fail"
		keeper1Path     = "/v1/cluster/keepers/keeper1"
	)

	failKeeperRoute := endpoints.FailKeeperEndPoint.Route(endpoints.MethodPost)

	// keeperArgsEntries returns an entry for failkeeper and removekeeper with no and with two keepers
	keeperArgsEntries := func() []TableEntry {
		var entries []TableEntry
		for _, command := range []string{"failkeeper", "removekeeper"} {
			entries = append(entries,
				Entry(command+" no keeper", command, nil, "keeper uid required"),
				Entry(command+" two keepers", command, []string{"keeper1", "keeper2"}, "too many arguments"),
			)
		}
		return entries
	}

	var api *fakeAPI

	BeforeEach(func() {
		api = newFakeAPI()
	})

	Describe("failkeeper", func() {
		It("fails the keeper", func() {
			api.handle(failKeeperRoute, accepted)
			stdout, _, err := api.run("failkeeper", "keeper1")
			Ω(err).NotTo(HaveOccurred())
			Ω(stdout).To(BeEmpty(), "expected no output without --wait")
			expectOnlyRequest(api, http.MethodPost, failKeeper1Path)
		})

		It("waits until another keeper runs the primary", func() {
			api.handle(failKeeperRoute, accepted)
			handleStatusSequence(api, available("keeper1"), unavailable(), available("keeper2"))
			stdout, _, err := api.run("failkeeper", "keeper1", "--wait")
			Ω(err).NotTo(HaveOccurred())
			Ω(stdout).To(Equal("the cluster applied the change\n"))
			Ω(api.requestsTo(http.MethodGet, statusPath)).To(HaveLen(3))
		})

		It("reports an api error", func() {
			api.handle(failKeeperRoute, func(w http.ResponseWriter, _ *http.Request) {
				endpoints.WriteProblem(w, http.StatusNotFound, "keeper keeper1 doesn't exist")
			})
			stdout, stderr, err := api.run("failkeeper", "keeper1", "--wait")
			expectError(err, "failed to fail the keeper: POST "+api.url(failKeeper1Path)+
				": 404 Not Found: keeper keeper1 doesn't exist", httpCodeSuffix(http.StatusNotFound))
			expectNoUsage(stdout, stderr)
			Ω(api.requestsTo(http.MethodGet, statusPath)).To(BeEmpty(), "expected no wait after a failure")
		})
	})

	Describe("removekeeper", func() {
		It("removes the keeper", func() {
			api.handle(removeKeeperRoute, accepted)
			_, _, err := api.run("removekeeper", "keeper1")
			Ω(err).NotTo(HaveOccurred())
			expectOnlyRequest(api, http.MethodDelete, keeper1Path)
		})

		It("waits", func() {
			api.handle(removeKeeperRoute, accepted)
			handleStatusSequence(api, available("keeper2"))
			stdout, _, err := api.run("removekeeper", "keeper1", "--wait")
			Ω(err).NotTo(HaveOccurred())
			Ω(stdout).To(Equal("the cluster applied the change\n"))
		})

		It("reports an api error", func() {
			api.handle(removeKeeperRoute, func(w http.ResponseWriter, _ *http.Request) {
				endpoints.WriteProblem(w, http.StatusConflict, "keeper keeper1 runs the primary: fail it first")
			})
			_, _, err := api.run("removekeeper", "keeper1")
			expectError(err, "failed to remove the keeper: DELETE "+api.url(keeper1Path)+
				": 409 Conflict: keeper keeper1 runs the primary: fail it first", httpCodeSuffix(http.StatusConflict))
		})
	})

	DescribeTable("need one keeper",
		func(command string, args []string, expected string) {
			stdout, stderr, err := api.run(append([]string{command}, args...)...)
			expectError(err, expected, expected)
			expectNoUsage(stdout, stderr)
			expectNoRequests(api)
		},
		keeperArgsEntries(),
	)
})

var _ = Describe("promote", func() {
	const promotePath = "/v1/cluster/actions/promote"

	var api *fakeAPI

	BeforeEach(func() {
		api = newFakeAPI()
	})

	It("promotes the cluster", func() {
		api.handle(endpoints.PromoteClusterEndPoint.Route(endpoints.MethodPost), accepted)
		_, _, err := api.run("promote")
		Ω(err).NotTo(HaveOccurred())
		expectOnlyRequest(api, http.MethodPost, promotePath)
	})

	It("waits", func() {
		api.handle(endpoints.PromoteClusterEndPoint.Route(endpoints.MethodPost), accepted)
		handleStatusSequence(api, available("keeper1"))
		stdout, _, err := api.run("promote", "--wait")
		Ω(err).NotTo(HaveOccurred())
		Ω(stdout).To(Equal("the cluster applied the change\n"))
	})

	It("reports an api error", func() {
		api.handle(endpoints.PromoteClusterEndPoint.Route(endpoints.MethodPost),
			func(w http.ResponseWriter, _ *http.Request) {
				endpoints.WriteProblem(w, http.StatusNotFound, "there is no cluster yet")
			})
		_, _, err := api.run("promote")
		expectError(err, "promote failed: POST "+api.url(promotePath)+": 404 Not Found: there is no cluster yet",
			httpCodeSuffix(http.StatusNotFound))
	})

	It("takes no arguments", func() {
		_, _, err := api.run("promote", "now")
		expectError(err, "too many arguments", "too many arguments")
		expectNoRequests(api)
	})
})
