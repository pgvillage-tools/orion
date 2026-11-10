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
	"errors"
	"net/http"
	"sync"
	"testing"

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

func TestWaitPollsThroughFailedStatusRequests(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(endpoints.PromoteClusterEndPoint.Route(endpoints.MethodPost), accepted)
	handleStatusSequence(api, nil, available("keeper1"))
	stdout, _, err := api.run("promote", "--wait")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "the cluster applied the change\n" {
		t.Errorf("unexpected output %q", stdout)
	}
}

func TestWaitTimesOutWithTheReason(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status *apiv1.InfoCluster
		reason string
	}{
		{"unavailable", unavailable(), "the cluster isn't available"},
		{"not converged", &apiv1.InfoCluster{
			Keepers: apiv1.InfoKeepers{{UID: "keeper2", Healthy: true, PgWantedGeneration: 3, PgCurrentGeneration: 2}},
			Status:  apiv1.InfoGenericStatus{"available": true, "primaryKeeper": "keeper1"},
		}, "keeper keeper2 hasn't applied the change yet"},
		{"status fails", nil, "GET "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := newFakeAPI(t)
			api.handle(endpoints.PromoteClusterEndPoint.Route(endpoints.MethodPost), accepted)
			handleStatusSequence(api, tc.status)
			stdout, stderr, err := api.run("promote", "--wait", "--wait-timeout", "20ms")
			expectError(t, err, "--wait timed out after 20ms, waiting for: "+tc.reason, "")
			expectNoUsage(t, stdout, stderr)
			if n := len(api.requestsTo(http.MethodGet, statusPath)); n < 2 {
				t.Errorf("expected it to poll more than once, got %d status requests", n)
			}
		})
	}
}

func TestWaitStopsWhenTheContextEnds(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(removeKeeperRoute, accepted)
	handleStatusSequence(api, unavailable())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := api.runWith(ctx, "", "removekeeper", "keeper1", "--wait")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the wait to stop with the context, got %v", err)
	}
	if n := len(api.requestsTo(http.MethodGet, statusPath)); n != 1 {
		t.Errorf("expected one status request, got %d", n)
	}
}

func TestWaitIsOffByDefault(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(endpoints.PromoteClusterEndPoint.Route(endpoints.MethodPost), accepted)
	// a --wait of an earlier run must not stick
	handleStatusSequence(api, available("keeper1"))
	if _, _, err := api.run("promote", "--wait"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := api.run("promote"); err != nil {
		t.Fatal(err)
	}
	if n := len(api.requestsTo(http.MethodGet, statusPath)); n != 1 {
		t.Errorf("expected only the first promote to wait, got %d status requests", n)
	}
}
