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
	"net/http"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// available is the status of an available cluster with its primary on keeper, and every keeper converged
func available(keeper string) *apiv1.InfoCluster {
	return &apiv1.InfoCluster{
		Keepers: apiv1.InfoKeepers{{UID: keeper, Healthy: true, PgWantedGeneration: 2, PgCurrentGeneration: 2}},
		Status:  apiv1.InfoGenericStatus{"available": true, "primaryKeeper": keeper},
	}
}

// unavailable is the status of a cluster without a primary
func unavailable() *apiv1.InfoCluster {
	return &apiv1.InfoCluster{Status: apiv1.InfoGenericStatus{"available": false}}
}

// handleStatusSequence lets api answer GET /v1/cluster/status with each of statuses in turn, and with the last one
// after that; a nil status is answered with an error
func handleStatusSequence(api *fakeAPI, statuses ...*apiv1.InfoCluster) {
	var mu sync.Mutex
	next := 0
	api.handle(endpoints.StatusEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		status := statuses[next]
		if next < len(statuses)-1 {
			next++
		}
		mu.Unlock()
		if status == nil {
			endpoints.WriteProblem(w, http.StatusServiceUnavailable, "the store doesn't answer")
			return
		}
		writeJSON(w, http.StatusOK, status, "")
	})
}

var _ = Describe("--wait", func() {
	const statusPath = "/v1/cluster/status"
	removeKeeperRoute := endpoints.KeeperEndPoint.Route(endpoints.MethodDelete)

	var api *fakeAPI

	BeforeEach(func() {
		api = newFakeAPI()
	})

	It("polls through failed status requests", func() {
		api.handle(endpoints.PromoteClusterEndPoint.Route(endpoints.MethodPost), accepted)
		handleStatusSequence(api, nil, available("keeper1"))
		stdout, _, err := api.run("promote", "--wait")
		Ω(err).NotTo(HaveOccurred())
		Ω(stdout).To(Equal("the cluster applied the change\n"))
	})

	DescribeTable("times out with the reason",
		func(status *apiv1.InfoCluster, reason string) {
			api.handle(endpoints.PromoteClusterEndPoint.Route(endpoints.MethodPost), accepted)
			handleStatusSequence(api, status)
			stdout, stderr, err := api.run("promote", "--wait", "--wait-timeout", "20ms")
			expectError(err, "--wait timed out after 20ms, waiting for: "+reason, "")
			expectNoUsage(stdout, stderr)
			Ω(len(api.requestsTo(http.MethodGet, statusPath))).To(BeNumerically(">=", 2),
				"expected it to poll more than once")
		},
		Entry("unavailable", unavailable(), "the cluster isn't available"),
		Entry("not converged", &apiv1.InfoCluster{
			Keepers: apiv1.InfoKeepers{{UID: "keeper2", Healthy: true, PgWantedGeneration: 3, PgCurrentGeneration: 2}},
			Status:  apiv1.InfoGenericStatus{"available": true, "primaryKeeper": "keeper1"},
		}, "keeper keeper2 hasn't applied the change yet"),
		Entry("status fails", nil, "GET "),
	)

	It("stops when the context ends", func() {
		api.handle(removeKeeperRoute, accepted)
		handleStatusSequence(api, unavailable())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, err := api.runWith(ctx, "", "removekeeper", "keeper1", "--wait")
		Ω(err).To(MatchError(context.Canceled), "expected the wait to stop with the context")
		Ω(api.requestsTo(http.MethodGet, statusPath)).To(HaveLen(1), "expected one status request")
	})

	It("is off by default", func() {
		api.handle(endpoints.PromoteClusterEndPoint.Route(endpoints.MethodPost), accepted)
		// a --wait of an earlier run must not stick
		handleStatusSequence(api, available("keeper1"))
		_, _, err := api.run("promote", "--wait")
		Ω(err).NotTo(HaveOccurred())
		_, _, err = api.run("promote")
		Ω(err).NotTo(HaveOccurred())
		Ω(api.requestsTo(http.MethodGet, statusPath)).To(HaveLen(1), "expected only the first promote to wait")
	})
})
