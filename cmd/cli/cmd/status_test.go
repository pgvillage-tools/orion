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
	"reflect"
	"strings"
	"testing"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

const statusPath = "/v1/cluster/status"

// testStatus is the status of a cluster with a primary on keeper1 and a standby on keeper2, as the api returns it
func testStatus() *apiv1.InfoCluster {
	return &apiv1.InfoCluster{
		Sentinels: apiv1.InfoSentinels{{UID: "sentinel1", Leader: true}, {UID: "sentinel2"}},
		Proxies:   apiv1.InfoProxies{{UID: "proxy1", Generation: 4}},
		Keepers: apiv1.InfoKeepers{
			{UID: "keeper1", ListenAddress: "10.0.0.1:5432", Healthy: true, PgHealthy: true, PgWantedGeneration: 3,
				PgCurrentGeneration: 3},
			{UID: "keeper2", ListenAddress: "10.0.0.2:5432", Healthy: false, PgHealthy: false, PgWantedGeneration: 2,
				PgCurrentGeneration: 1},
		},
		Status: apiv1.InfoGenericStatus{"available": true, "primaryDB": "db1", "primaryKeeper": "keeper1",
			"role": "primary"},
	}
}

// handleStatus lets api answer GET /v1/cluster/status with ic
func handleStatus(api *fakeAPI, ic *apiv1.InfoCluster) {
	api.handle(endpoints.StatusEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, ic, "")
	})
}

// lines returns the lines of s, with the trailing spaces and tabs of every line removed
func lines(s string) []string {
	result := strings.Split(s, "\n")
	for i, l := range result {
		result[i] = strings.TrimRight(l, " \t")
	}
	return result
}

// expectLines fails t unless output has expected as consecutive lines
func expectLines(t *testing.T, output string, expected ...string) {
	t.Helper()
	if !strings.Contains(strings.Join(lines(output), "\n"), strings.Join(expected, "\n")) {
		t.Errorf("expected the lines\n%s\nin\n%s", strings.Join(expected, "\n"), output)
	}
}

// columns returns the line of output that starts with first, split on whitespace
func columns(t *testing.T, output, first string) []string {
	t.Helper()
	for _, l := range lines(output) {
		if f := strings.Fields(l); len(f) > 0 && f[0] == first {
			return f
		}
	}
	t.Fatalf("no line starting with %s in\n%s", first, output)
	return nil
}

func TestStatusPrintsTheClusterAsText(t *testing.T) {
	api := newFakeAPI(t)
	handleStatus(api, testStatus())
	handleClusterData(api, testClusterData())
	stdout, _, err := api.run("status")
	if err != nil {
		t.Fatal(err)
	}
	expectLines(t, stdout, "=== Active sentinels ===", "")
	if c := columns(t, stdout, "ID"); !reflect.DeepEqual(c, []string{"ID", "LEADER"}) {
		t.Errorf("unexpected sentinel header %v", c)
	}
	if c := columns(t, stdout, "sentinel1"); !reflect.DeepEqual(c, []string{"sentinel1", "true"}) {
		t.Errorf("unexpected sentinel line %v", c)
	}
	if c := columns(t, stdout, "sentinel2"); !reflect.DeepEqual(c, []string{"sentinel2", "false"}) {
		t.Errorf("unexpected sentinel line %v", c)
	}
	expectLines(t, stdout, "=== Active proxies ===", "", "ID", "proxy1")
	expectLines(t, stdout, "=== Keepers ===", "")
	if c := columns(t, stdout, "UID"); strings.Join(c, " ") !=
		"UID HEALTHY PG LISTENADDRESS PG HEALTHY PG WANTEDGENERATION PG CURRENTGENERATION" {
		t.Errorf("unexpected keeper header %v", c)
	}
	if c := columns(t, stdout, "keeper1"); !reflect.DeepEqual(c,
		[]string{"keeper1", "true", "10.0.0.1:5432", "true", "3", "3"}) {
		t.Errorf("unexpected keeper line %v", c)
	}
	if c := columns(t, stdout, "keeper2"); !reflect.DeepEqual(c,
		[]string{"keeper2", "false", "10.0.0.2:5432", "false", "2", "1"}) {
		t.Errorf("unexpected keeper line %v", c)
	}
	expectLines(t, stdout, "=== Cluster Info ===", "", "Cluster role: primary", "Master Keeper: keeper1")
	expectLines(t, stdout, "===== Keepers/DB tree =====", "", "keeper1 (master)", "└─keeper2", "")
}

func TestStatusPrintsAnEmptyCluster(t *testing.T) {
	api := newFakeAPI(t)
	handleStatus(api, &apiv1.InfoCluster{Status: apiv1.InfoGenericStatus{"available": false}})
	handleClusterData(api, &apiv1.Data{})
	stdout, _, err := api.run("status", "--format", "text")
	if err != nil {
		t.Fatal(err)
	}
	expectLines(t, stdout,
		"=== Active sentinels ===", "", "No active sentinels", "",
		"=== Active proxies ===", "", "No active proxies", "",
		"=== Keepers ===", "", "No keepers available", "",
		"No cluster available", "")
	if strings.Contains(stdout, "Keepers/DB tree") {
		t.Errorf("expected no tree without a primary, got\n%s", stdout)
	}
}

func TestStatusPrintsTheTreeOfFollowers(t *testing.T) {
	cd := testClusterData()
	// db1 has followers db2 and db3, db2 has db7 and db8, db3 has db4, which has db6; db5 isn't in the cluster data
	// (a removed keeper)
	cd.DBs["db1"].Spec.Followers = []string{"db2", "db5", "db3"}
	cd.DBs["db3"] = &apiv1.DB{UID: "db3", Spec: &apiv1.DBSpec{KeeperUID: "keeper3", Followers: []string{"db4"}}}
	cd.DBs["db4"] = &apiv1.DB{UID: "db4", Spec: &apiv1.DBSpec{KeeperUID: "keeper4", Followers: []string{"db6"}}}
	cd.DBs["db6"] = &apiv1.DB{UID: "db6", Spec: &apiv1.DBSpec{KeeperUID: "keeper6"}}
	cd.DBs["db2"].Spec.Followers = []string{"db7", "db8"}
	cd.DBs["db7"] = &apiv1.DB{UID: "db7", Spec: &apiv1.DBSpec{KeeperUID: "keeper7"}}
	cd.DBs["db8"] = &apiv1.DB{UID: "db8", Spec: &apiv1.DBSpec{KeeperUID: "keeper8"}}
	api := newFakeAPI(t)
	handleStatus(api, testStatus())
	handleClusterData(api, cd)
	stdout, _, err := api.run("status")
	if err != nil {
		t.Fatal(err)
	}
	expectLines(t, stdout, "===== Keepers/DB tree =====", "",
		"keeper1 (master)",
		"├─keeper2",
		"│ ├─keeper7",
		"│ └─keeper8",
		"└─keeper3",
		"  └─keeper4",
		"    └─keeper6",
		"")
}

func TestStatusPrintsTheClusterAsJSON(t *testing.T) {
	api := newFakeAPI(t)
	handleStatus(api, testStatus())
	stdout, _, err := api.run("status", "-f", "json")
	if err != nil {
		t.Fatal(err)
	}
	// the status as the api returned it; it used to marshal the status function instead, and failed
	var got apiv1.InfoCluster
	if err = json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("expected the status as json, got %q: %v", stdout, err)
	}
	expected := testStatus()
	expectedJSON, _ := json.Marshal(expected)
	gotJSON, _ := json.Marshal(got)
	if string(gotJSON) != string(expectedJSON) {
		t.Errorf("expected %s, got %s", expectedJSON, gotJSON)
	}
	expectContains(t, stdout, "\n\t\"proxies\": [")
	if len(api.requestsTo(http.MethodGet, clusterPath)) != 0 {
		t.Error("expected json not to need the cluster data")
	}
}

func TestStatusRefusesAnUnknownFormat(t *testing.T) {
	api := newFakeAPI(t)
	stdout, stderr, err := api.run("status", "--format", "yaml")
	expectError(t, err, "unrecognised output format yaml", "unrecognised output format yaml")
	expectNoUsage(t, stdout, stderr)
	if len(api.requests()) != 0 {
		t.Errorf("expected no requests, got %v", api.requests())
	}
}

func TestStatusReportsAFailedStatus(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(endpoints.StatusEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
		endpoints.WriteProblem(w, http.StatusServiceUnavailable, "the store doesn't answer")
	})
	_, _, err := api.run("status")
	expectError(t, err, "failed to get the cluster status: GET "+api.url(statusPath)+
		": 503 Service Unavailable: the store doesn't answer", httpCodeSuffix(http.StatusServiceUnavailable))
}

func TestStatusReportsFailedClusterData(t *testing.T) {
	api := newFakeAPI(t)
	handleStatus(api, testStatus())
	api.handle(endpoints.ClusterEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
		endpoints.WriteProblem(w, http.StatusInternalServerError, "")
	})
	_, _, err := api.run("status")
	expectError(t, err, "failed to get the cluster data: GET "+api.url(clusterPath)+": 500 Internal Server Error",
		httpCodeSuffix(http.StatusInternalServerError))
}
