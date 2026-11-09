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
	"testing"

	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

const (
	failKeeper1Path = "/v1/cluster/keepers/keeper1/actions/fail"
	keeper1Path     = "/v1/cluster/keepers/keeper1"
	promotePath     = "/v1/cluster/actions/promote"
)

var (
	failKeeperRoute   = endpoints.FailKeeperEndPoint.Route(endpoints.MethodPost)
	removeKeeperRoute = endpoints.KeeperEndPoint.Route(endpoints.MethodDelete)
)

func TestFailkeeperFailsTheKeeper(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(failKeeperRoute, accepted)
	stdout, _, err := api.run("failkeeper", "keeper1")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "" {
		t.Errorf("expected no output without --wait, got %q", stdout)
	}
	if reqs := api.requests(); len(reqs) != 1 || reqs[0].Method != http.MethodPost || reqs[0].Path != failKeeper1Path {
		t.Errorf("expected one POST to %s, got %v", failKeeper1Path, reqs)
	}
}

func TestFailkeeperWaitsUntilAnotherKeeperRunsThePrimary(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(failKeeperRoute, accepted)
	handleStatusSequence(api, available("keeper1"), unavailable(), available("keeper2"))
	stdout, _, err := api.run("failkeeper", "keeper1", "--wait")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "the cluster applied the change\n" {
		t.Errorf("unexpected output %q", stdout)
	}
	if n := len(api.requestsTo(http.MethodGet, statusPath)); n != 3 {
		t.Errorf("expected 3 status requests, got %d", n)
	}
}

func TestFailkeeperReportsAnAPIError(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(failKeeperRoute, func(w http.ResponseWriter, _ *http.Request) {
		endpoints.WriteProblem(w, http.StatusNotFound, "keeper keeper1 doesn't exist")
	})
	stdout, stderr, err := api.run("failkeeper", "keeper1", "--wait")
	expectError(t, err, "failed to fail the keeper: POST "+api.url(failKeeper1Path)+
		": 404 Not Found: keeper keeper1 doesn't exist", httpCodeSuffix(http.StatusNotFound))
	expectNoUsage(t, stdout, stderr)
	if n := len(api.requestsTo(http.MethodGet, statusPath)); n != 0 {
		t.Errorf("expected no wait after a failure, got %d status requests", n)
	}
}

func TestRemovekeeperRemovesTheKeeper(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(removeKeeperRoute, accepted)
	if _, _, err := api.run("removekeeper", "keeper1"); err != nil {
		t.Fatal(err)
	}
	if reqs := api.requests(); len(reqs) != 1 || reqs[0].Method != http.MethodDelete || reqs[0].Path != keeper1Path {
		t.Errorf("expected one DELETE of %s, got %v", keeper1Path, reqs)
	}
}

func TestRemovekeeperWaits(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(removeKeeperRoute, accepted)
	handleStatusSequence(api, available("keeper2"))
	stdout, _, err := api.run("removekeeper", "keeper1", "--wait")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "the cluster applied the change\n" {
		t.Errorf("unexpected output %q", stdout)
	}
}

func TestRemovekeeperReportsAnAPIError(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(removeKeeperRoute, func(w http.ResponseWriter, _ *http.Request) {
		endpoints.WriteProblem(w, http.StatusConflict, "keeper keeper1 runs the primary: fail it first")
	})
	_, _, err := api.run("removekeeper", "keeper1")
	expectError(t, err, "failed to remove the keeper: DELETE "+api.url(keeper1Path)+
		": 409 Conflict: keeper keeper1 runs the primary: fail it first", httpCodeSuffix(http.StatusConflict))
}

func TestKeeperCommandsNeedOneKeeper(t *testing.T) {
	for _, command := range []string{"failkeeper", "removekeeper"} {
		for _, tc := range []struct {
			name     string
			args     []string
			expected string
		}{
			{"no keeper", nil, "keeper uid required"},
			{"two keepers", []string{"keeper1", "keeper2"}, "too many arguments"},
		} {
			t.Run(command+" "+tc.name, func(t *testing.T) {
				api := newFakeAPI(t)
				stdout, stderr, err := api.run(append([]string{command}, tc.args...)...)
				expectError(t, err, tc.expected, tc.expected)
				expectNoUsage(t, stdout, stderr)
				if len(api.requests()) != 0 {
					t.Errorf("expected no requests, got %v", api.requests())
				}
			})
		}
	}
}

func TestPromotePromotesTheCluster(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(endpoints.PromoteClusterEndPoint.Route(endpoints.MethodPost), accepted)
	if _, _, err := api.run("promote"); err != nil {
		t.Fatal(err)
	}
	if reqs := api.requests(); len(reqs) != 1 || reqs[0].Method != http.MethodPost || reqs[0].Path != promotePath {
		t.Errorf("expected one POST to %s, got %v", promotePath, reqs)
	}
}

func TestPromoteWaits(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(endpoints.PromoteClusterEndPoint.Route(endpoints.MethodPost), accepted)
	handleStatusSequence(api, available("keeper1"))
	stdout, _, err := api.run("promote", "--wait")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "the cluster applied the change\n" {
		t.Errorf("unexpected output %q", stdout)
	}
}

func TestPromoteReportsAnAPIError(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(endpoints.PromoteClusterEndPoint.Route(endpoints.MethodPost), func(w http.ResponseWriter, _ *http.Request) {
		endpoints.WriteProblem(w, http.StatusNotFound, "there is no cluster yet")
	})
	_, _, err := api.run("promote")
	expectError(t, err, "promote failed: POST "+api.url(promotePath)+": 404 Not Found: there is no cluster yet",
		httpCodeSuffix(http.StatusNotFound))
}

func TestPromoteTakesNoArguments(t *testing.T) {
	api := newFakeAPI(t)
	_, _, err := api.run("promote", "now")
	expectError(t, err, "too many arguments", "too many arguments")
	if len(api.requests()) != 0 {
		t.Errorf("expected no requests, got %v", api.requests())
	}
}
