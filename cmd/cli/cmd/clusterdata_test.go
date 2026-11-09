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
	"testing"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

const clusterPath = "/v1/cluster"

// testClusterData is cluster data with a primary (db1 on keeper1) and a standby (db2 on keeper2)
func testClusterData() *apiv1.Data {
	return &apiv1.Data{
		FormatVersion: 1,
		Cluster:       &apiv1.Cluster{UID: "cluster1", Status: apiv1.ClusterStatus{Master: "db1"}},
		Keepers:       apiv1.Keepers{"keeper1": &apiv1.Keeper{UID: "keeper1"}, "keeper2": &apiv1.Keeper{UID: "keeper2"}},
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

func TestClusterdataReadPrintsTheDataAndItsETag(t *testing.T) {
	api := newFakeAPI(t)
	handleClusterData(api, testClusterData())
	stdout, stderr, err := api.run("clusterdata", "read")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "ETag: \"7\"\n" {
		t.Errorf("expected the ETag on stderr, got %q", stderr)
	}
	if strings.Count(stdout, "\n") != 1 {
		t.Errorf("expected the data on one line, got %q", stdout)
	}
	var cd apiv1.Data
	if err = json.Unmarshal([]byte(stdout), &cd); err != nil {
		t.Fatal(err)
	}
	if cd.Cluster.UID != "cluster1" || cd.DBs["db1"].Spec.KeeperUID != "keeper1" {
		t.Errorf("unexpected cluster data %s", stdout)
	}
}

func TestClusterdataReadPrettyPrints(t *testing.T) {
	api := newFakeAPI(t)
	handleClusterData(api, testClusterData())
	stdout, _, err := api.run("clusterdata", "read", "--pretty")
	if err != nil {
		t.Fatal(err)
	}
	expectContains(t, stdout, "{\n\t\"formatVersion\": 1,\n")
}

func TestClusterdataReadReportsAnAPIError(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(endpoints.ClusterEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
		endpoints.WriteProblem(w, http.StatusNotFound, "there is no cluster data yet")
	})
	stdout, stderr, err := api.run("clusterdata", "read")
	expectError(t, err, "failed to get the cluster data: GET "+api.url(clusterPath)+
		": 404 Not Found: there is no cluster data yet", httpCodeSuffix(http.StatusNotFound))
	expectNoUsage(t, stdout, stderr)
	if stdout != "" {
		t.Errorf("expected no output, got %q", stdout)
	}
}

// handlePutClusterData lets api accept PUT /v1/cluster
func handlePutClusterData(api *fakeAPI) {
	api.handle(endpoints.ClusterEndPoint.Route(endpoints.MethodPut), func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"8"`)
		w.WriteHeader(http.StatusNoContent)
	})
}

// expectPutClusterData fails t unless api got one PUT of cluster data with cluster uid and ifMatch
func expectPutClusterData(t *testing.T, api *fakeAPI, uid, ifMatch string) {
	t.Helper()
	puts := api.requestsTo(http.MethodPut, clusterPath)
	if len(puts) != 1 {
		t.Fatalf("expected one PUT, got %v", api.requests())
	}
	if puts[0].IfMatch != ifMatch {
		t.Errorf("expected If-Match %q, got %q", ifMatch, puts[0].IfMatch)
	}
	var cd apiv1.Data
	if err := json.Unmarshal([]byte(puts[0].Body), &cd); err != nil {
		t.Fatal(err)
	}
	if cd.Cluster == nil || cd.Cluster.UID != uid {
		t.Errorf("expected cluster %s, got %s", uid, puts[0].Body)
	}
}

func TestClusterdataWriteSendsTheArgument(t *testing.T) {
	api := newFakeAPI(t)
	handlePutClusterData(api)
	if _, _, err := api.run("clusterdata", "write", `{"cluster": {"uid": "arg"}}`); err != nil {
		t.Fatal(err)
	}
	expectPutClusterData(t, api, "arg", "")
}

func TestClusterdataWriteSendsIfMatch(t *testing.T) {
	api := newFakeAPI(t)
	handlePutClusterData(api)
	if _, _, err := api.run("clusterdata", "write", "--if-match", `"7"`, `{"cluster": {"uid": "arg"}}`); err != nil {
		t.Fatal(err)
	}
	expectPutClusterData(t, api, "arg", `"7"`)
}

func TestClusterdataWriteReadsAFile(t *testing.T) {
	api := newFakeAPI(t)
	handlePutClusterData(api)
	file := filepath.Join(t.TempDir(), "cd.json")
	if err := os.WriteFile(file, []byte(`{"cluster": {"uid": "file"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := api.run("clusterdata", "write", "-f", file); err != nil {
		t.Fatal(err)
	}
	expectPutClusterData(t, api, "file", "")
}

func TestClusterdataWriteReadsStdin(t *testing.T) {
	api := newFakeAPI(t)
	handlePutClusterData(api)
	if _, _, err := api.runWith(context.Background(), `{"cluster": {"uid": "stdin"}}`, "clusterdata", "write",
		"--file", "-"); err != nil {
		t.Fatal(err)
	}
	expectPutClusterData(t, api, "stdin", "")
}

func TestClusterdataWriteRefusesInvalidInput(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")
	for _, tc := range []struct {
		name     string
		args     []string
		expected string
	}{
		{"no data", nil, "no cluster data provided: pass it as an argument or via --file"},
		{"two arguments", []string{"{}", "{}"}, "clusterdata write accepts at most one positional argument"},
		{"invalid json", []string{"{"}, "invalid cluster data: "},
		{"missing file", []string{"--file", missing}, "cannot read " + missing + ": "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := newFakeAPI(t)
			stdout, stderr, err := api.run(append([]string{"clusterdata", "write"}, tc.args...)...)
			expectError(t, err, tc.expected, "")
			expectNoUsage(t, stdout, stderr)
			if len(api.requests()) != 0 {
				t.Errorf("expected no requests, got %v", api.requests())
			}
		})
	}
}

func TestClusterdataWriteReportsAnAPIError(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(endpoints.ClusterEndPoint.Route(endpoints.MethodPut), func(w http.ResponseWriter, _ *http.Request) {
		endpoints.WriteProblem(w, http.StatusPreconditionFailed, "it changed since you read it")
	})
	_, _, err := api.run("clusterdata", "write", "--if-match", `"6"`, `{}`)
	expectError(t, err, "failed to write the cluster data: PUT "+api.url(clusterPath)+
		": 412 Precondition Failed: it changed since you read it", httpCodeSuffix(http.StatusPreconditionFailed))
}
