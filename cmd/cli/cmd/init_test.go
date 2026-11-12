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
	"testing"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// newInitAPI returns a fake api without a cluster, that accepts POST /v1/cluster
func newInitAPI(t *testing.T) *fakeAPI {
	api := newFakeAPI(t)
	api.handle(endpoints.ClusterEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
		endpoints.WriteProblem(w, http.StatusNotFound, "there is no cluster data yet")
	})
	api.handle(endpoints.ClusterEndPoint.Route(endpoints.MethodPost), accepted)
	return api
}

// expectPostedSpec fails t unless api got one POST of a spec, and returns that spec
func expectPostedSpec(t *testing.T, api *fakeAPI) apiv1.Spec {
	t.Helper()
	posts := api.requestsTo(http.MethodPost, clusterPath)
	if len(posts) != 1 {
		t.Fatalf("expected one POST, got %v", api.requests())
	}
	var spec apiv1.Spec
	if err := json.Unmarshal([]byte(posts[0].Body), &spec); err != nil {
		t.Fatal(err)
	}
	return spec
}

func TestInitPostsTheSpecOfTheArgument(t *testing.T) {
	api := newInitAPI(t)
	stdout, _, err := api.run("init", `{"initMode": "new", "maxStandbys": 3}`)
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "" {
		t.Errorf("expected no output without --wait, got %q", stdout)
	}
	spec := expectPostedSpec(t, api)
	if spec.InitMode == nil || *spec.InitMode != apiv1.New || spec.MaxStandbys == nil || *spec.MaxStandbys != 3 {
		t.Errorf("unexpected spec %+v", spec)
	}
}

func TestInitReadsTheSpecFromStdinWithADash(t *testing.T) {
	api := newInitAPI(t)
	if _, _, err := api.runWith(context.Background(), `{"maxStandbys": 5}`, "init", "--file", "-"); err != nil {
		t.Fatal(err)
	}
	if spec := expectPostedSpec(t, api); spec.MaxStandbys == nil || *spec.MaxStandbys != 5 {
		t.Errorf("unexpected spec %+v", spec)
	}
}

func TestInitWithoutASpecCreatesANewClusterWithDefaults(t *testing.T) {
	for name, tc := range map[string]struct {
		stdin string
		args  []string
	}{
		// it doesn't read stdin without --file -: at a terminal it would wait for input
		"no arguments, stdin ignored": {stdin: `{"maxStandbys": 5}`},
		"empty stdin":                 {stdin: "", args: []string{"--file", "-"}},
		"stdin with only whitespace":  {stdin: " \n", args: []string{"--file", "-"}},
	} {
		t.Run(name, func(t *testing.T) {
			api := newInitAPI(t)
			if _, _, err := api.runWith(context.Background(), tc.stdin, append([]string{"init"}, tc.args...)...); err != nil {
				t.Fatal(err)
			}
			spec := expectPostedSpec(t, api)
			if spec.InitMode == nil || *spec.InitMode != apiv1.New || spec.MaxStandbys != nil {
				t.Errorf("expected a new cluster with the default settings, got %+v", spec)
			}
		})
	}
}

func TestInitDefaultsTheInitModeToNew(t *testing.T) {
	api := newInitAPI(t)
	if _, _, err := api.run("init", `{"maxStandbys": 3}`); err != nil {
		t.Fatal(err)
	}
	if spec := expectPostedSpec(t, api); spec.InitMode == nil || *spec.InitMode != apiv1.New {
		t.Errorf("expected initMode new, got %+v", spec)
	}
}

func TestInitKeepsAnotherInitMode(t *testing.T) {
	api := newInitAPI(t)
	if _, _, err := api.run("init", `{"initMode": "existing", "existingConfig": {"keeperUID": "keeper1"}}`); err != nil {
		t.Fatal(err)
	}
	if spec := expectPostedSpec(t, api); spec.InitMode == nil || *spec.InitMode != apiv1.ExistingCluster {
		t.Errorf("expected initMode existing, got %+v", spec)
	}
}

func TestInitReadsTheSpecFromAFile(t *testing.T) {
	api := newInitAPI(t)
	file := filepath.Join(t.TempDir(), "spec.json")
	if err := os.WriteFile(file, []byte(`{"maxStandbys": 7}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := api.run("init", "--file", file); err != nil {
		t.Fatal(err)
	}
	if spec := expectPostedSpec(t, api); spec.MaxStandbys == nil || *spec.MaxStandbys != 7 {
		t.Errorf("unexpected spec %+v", spec)
	}
}

func TestInitRefusesAnExistingCluster(t *testing.T) {
	api := newFakeAPI(t)
	handleClusterData(api, testClusterData())
	stdout, stderr, err := api.run("init", `{}`)
	expectError(t, err, "cannot initialize an already existing cluster", "cannot initialize an already existing cluster")
	expectNoUsage(t, stdout, stderr)
	if len(api.requestsTo(http.MethodPost, clusterPath)) != 0 {
		t.Error("expected no POST for an existing cluster")
	}
}

func TestInitReportsAFailedRead(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(endpoints.ClusterEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
		endpoints.WriteProblem(w, http.StatusInternalServerError, "the store doesn't answer")
	})
	_, _, err := api.run("init", `{}`)
	expectError(t, err, "failed to get the cluster data: GET "+api.url(clusterPath)+
		": 500 Internal Server Error: the store doesn't answer", httpCodeSuffix(http.StatusInternalServerError))
}

func TestInitReportsAFailedPost(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(endpoints.ClusterEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
		endpoints.WriteProblem(w, http.StatusNotFound, "there is no cluster data yet")
	})
	api.handle(endpoints.ClusterEndPoint.Route(endpoints.MethodPost), func(w http.ResponseWriter, _ *http.Request) {
		endpoints.WriteProblem(w, http.StatusBadRequest, "invalid spec: maxStandbys can't be 0")
	})
	_, _, err := api.run("init", `{"maxStandbys": 0}`)
	expectError(t, err, "init failed: POST "+api.url(clusterPath)+
		": 400 Bad Request: invalid spec: maxStandbys can't be 0", httpCodeSuffix(http.StatusBadRequest))
}

func TestInitRefusesInvalidInput(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")
	for _, tc := range []struct {
		name     string
		args     []string
		expected string
	}{
		{"two arguments", []string{"{}", "{}"}, "too many arguments"},
		{"invalid json", []string{"{"}, "invalid cluster spec: "},
		{"missing file", []string{"--file", missing}, "cannot read " + missing + ": "},
		{"an argument and a file", []string{"{}", "--file", missing}, "pass the cluster spec as an argument or with --file, not both"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := newInitAPI(t)
			_, _, err := api.run(append([]string{"init"}, tc.args...)...)
			expectError(t, err, tc.expected, "")
			if len(api.requests()) != 0 {
				t.Errorf("expected no requests, got %v", api.requests())
			}
		})
	}
}

func TestInitWaitsUntilTheClusterIsAvailable(t *testing.T) {
	api := newInitAPI(t)
	handleStatusSequence(api, unavailable(), available("keeper1"))
	stdout, _, err := api.run("init", "--wait", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "the cluster applied the change\n" {
		t.Errorf("unexpected output %q", stdout)
	}
	if n := len(api.requestsTo(http.MethodGet, statusPath)); n != 2 {
		t.Errorf("expected 2 status requests, got %d", n)
	}
}
