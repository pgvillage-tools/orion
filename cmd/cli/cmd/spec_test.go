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

const specPath = "/v1/cluster/spec"

// handleSpec lets api answer GET /v1/cluster/spec with spec and ETag "abc"
func handleSpec(api *fakeAPI, spec string) {
	api.handle(endpoints.ClusterSpecEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"abc"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(spec))
	})
}

func TestSpecPrintsTheSetValuesAndTheETag(t *testing.T) {
	api := newFakeAPI(t)
	handleSpec(api, `{"maxStandbys": 4, "pgParameters": {"max_connections": "200"}}`)
	stdout, stderr, err := api.run("spec")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "ETag: \"abc\"\n" {
		t.Errorf("expected the ETag on stderr, got %q", stderr)
	}
	expected := "{\n\t\"maxStandbys\": 4,\n\t\"pgParameters\": {\n\t\t\"max_connections\": \"200\"\n\t}\n}\n"
	if stdout != expected {
		t.Errorf("expected\n%s\ngot\n%s", expected, stdout)
	}
}

func TestSpecPrintsTheDefaults(t *testing.T) {
	api := newFakeAPI(t)
	handleSpec(api, `{"maxStandbys": 4}`)
	stdout, _, err := api.run("spec", "--defaults")
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err = json.Unmarshal([]byte(stdout), &spec); err != nil {
		t.Fatal(err)
	}
	if spec["maxStandbys"] != float64(4) {
		t.Errorf("expected the set value, got %v", spec["maxStandbys"])
	}
	defaults := (&apiv1.Spec{}).WithDefaults()
	if spec["sleepInterval"] != defaults.SleepInterval.String() || spec["role"] != string(*defaults.Role) {
		t.Errorf("expected the defaults, got %s", stdout)
	}
	if _, ok := spec["standbyConfig"]; !ok {
		t.Errorf("expected every field, also the unset ones, got %s", stdout)
	}
}

func TestSpecReportsAnAPIError(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(endpoints.ClusterSpecEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
		endpoints.WriteProblem(w, http.StatusNotFound, "there is no cluster spec yet")
	})
	stdout, stderr, err := api.run("spec")
	expectError(t, err, "failed to get the cluster spec: GET "+api.url(specPath)+
		": 404 Not Found: there is no cluster spec yet", httpCodeSuffix(http.StatusNotFound))
	expectNoUsage(t, stdout, stderr)
}

// handleSpecUpdates lets api accept PUT and PATCH of /v1/cluster/spec
func handleSpecUpdates(api *fakeAPI) {
	for _, method := range []endpoints.Method{endpoints.MethodPut, endpoints.MethodPatch} {
		api.handle(endpoints.ClusterSpecEndPoint.Route(method), accepted)
	}
}

// expectSpecUpdate fails t unless api got one method request of the spec, and returns it
func expectSpecUpdate(t *testing.T, api *fakeAPI, method string) recordedRequest {
	t.Helper()
	reqs := api.requests()
	if len(reqs) != 1 || reqs[0].Method != method || reqs[0].Path != specPath {
		t.Fatalf("expected one %s of %s, got %v", method, specPath, reqs)
	}
	return reqs[0]
}

// expectMaxStandbys fails t unless req has a spec with maxStandbys expected
func expectMaxStandbys(t *testing.T, req recordedRequest, expected uint16) {
	t.Helper()
	var spec apiv1.Spec
	if err := json.Unmarshal([]byte(req.Body), &spec); err != nil {
		t.Fatal(err)
	}
	if spec.MaxStandbys == nil || *spec.MaxStandbys != expected {
		t.Errorf("expected maxStandbys %d, got %s", expected, req.Body)
	}
}

func TestUpdateReplacesTheSpec(t *testing.T) {
	api := newFakeAPI(t)
	handleSpecUpdates(api)
	stdout, _, err := api.run("update", `{"maxStandbys": 2}`)
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "" {
		t.Errorf("expected no output without --wait, got %q", stdout)
	}
	req := expectSpecUpdate(t, api, http.MethodPut)
	if req.ContentType != "application/json" || req.IfMatch != "" {
		t.Errorf("unexpected request %+v", req)
	}
	expectMaxStandbys(t, req, 2)
}

func TestUpdatePatchesTheSpecWithIfMatch(t *testing.T) {
	api := newFakeAPI(t)
	handleSpecUpdates(api)
	// a patch is sent as it is: null removes a value
	patch := `{"maxStandbys": null}`
	if _, _, err := api.run("update", "--patch", "--if-match", `"abc"`, patch); err != nil {
		t.Fatal(err)
	}
	req := expectSpecUpdate(t, api, http.MethodPatch)
	if req.Body != patch || req.ContentType != endpoints.ContentTypeMergePatch || req.IfMatch != `"abc"` {
		t.Errorf("unexpected request %+v", req)
	}
}

func TestUpdateReadsAFile(t *testing.T) {
	api := newFakeAPI(t)
	handleSpecUpdates(api)
	file := filepath.Join(t.TempDir(), "patch.json")
	if err := os.WriteFile(file, []byte(`{"usePgrewind": true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := api.run("update", "-P", "-f", file); err != nil {
		t.Fatal(err)
	}
	if req := expectSpecUpdate(t, api, http.MethodPatch); req.Body != `{"usePgrewind": true}` {
		t.Errorf("unexpected body %s", req.Body)
	}
}

func TestUpdateReadsStdin(t *testing.T) {
	api := newFakeAPI(t)
	handleSpecUpdates(api)
	if _, _, err := api.runWith(context.Background(), `{"maxStandbys": 6}`, "update", "--file", "-",
		"--if-match", `"abc"`); err != nil {
		t.Fatal(err)
	}
	req := expectSpecUpdate(t, api, http.MethodPut)
	if req.IfMatch != `"abc"` {
		t.Errorf("unexpected request %+v", req)
	}
	expectMaxStandbys(t, req, 6)
}

func TestUpdateWaits(t *testing.T) {
	api := newFakeAPI(t)
	handleSpecUpdates(api)
	handleStatusSequence(api, &apiv1.InfoCluster{
		Keepers: apiv1.InfoKeepers{{UID: "keeper1", Healthy: true, PgWantedGeneration: 3, PgCurrentGeneration: 2}},
		Status:  apiv1.InfoGenericStatus{"available": true, "primaryKeeper": "keeper1"},
	}, available("keeper1"))
	stdout, _, err := api.run("update", "--patch", "--wait", `{"maxStandbys": 2}`)
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

func TestUpdateReportsAnAPIError(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(endpoints.ClusterSpecEndPoint.Route(endpoints.MethodPatch), func(w http.ResponseWriter, _ *http.Request) {
		endpoints.WriteProblem(w, http.StatusPreconditionFailed, "it changed since you read it")
	})
	stdout, stderr, err := api.run("update", "--patch", "--if-match", `"old"`, `{}`)
	expectError(t, err, "failed to update the cluster spec: PATCH "+api.url(specPath)+
		": 412 Precondition Failed: it changed since you read it", httpCodeSuffix(http.StatusPreconditionFailed))
	expectNoUsage(t, stdout, stderr)
}

func TestUpdateRefusesInvalidInput(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")
	for _, tc := range []struct {
		name     string
		args     []string
		expected string
	}{
		{"no spec", nil, "no cluster spec provided as argument and no file provided (--file/-f option)"},
		{"argument and file", []string{"--file", missing, "{}"}, "only one of cluster spec provided as argument or file"},
		{"two arguments", []string{"{}", "{}"}, "too many arguments"},
		{"invalid json", []string{"{"}, "invalid cluster spec: "},
		{"missing file", []string{"--file", missing}, "cannot read " + missing + ": "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := newFakeAPI(t)
			stdout, stderr, err := api.run(append([]string{"update"}, tc.args...)...)
			expectError(t, err, tc.expected, "")
			expectNoUsage(t, stdout, stderr)
			if len(api.requests()) != 0 {
				t.Errorf("expected no requests, got %v", api.requests())
			}
		})
	}
}
