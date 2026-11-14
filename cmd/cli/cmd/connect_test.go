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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pgvillage-tools/orion/cmd"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

func TestConnectPrintsTheIdentityAndSavesTheConfig(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(endpoints.WhoAmIEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, endpoints.WhoAmIResponse{Name: "orion-cli", Method: "certificate"}, "")
	})
	stdout, _, err := api.run("connect")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "Connected to the api as orion-cli (certificate)\n" {
		t.Errorf("unexpected output %q", stdout)
	}
	config, err := os.ReadFile(filepath.Join(testHome, ".orion", "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	// viper writes the flag as a string
	expectContains(t, string(config), `port: "`+api.port()+`"`, "host: 127.0.0.1")
}

func TestConnectReportsAnAPIError(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(endpoints.WhoAmIEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
		endpoints.WriteProblem(w, http.StatusUnauthorized, "unauthorized")
	})
	stdout, stderr, err := api.run("connect")
	expectError(t, err, "connecting to the api ("+api.url("/v1/whoami")+") failed: GET "+api.url("/v1/whoami")+
		": 401 Unauthorized: unauthorized", httpCodeSuffix(http.StatusUnauthorized))
	expectNoUsage(t, stdout, stderr)
}

func TestConnectFailsWhenTheAPIIsDown(t *testing.T) {
	api := newFakeAPI(t)
	api.server.Close()
	_, _, err := api.run("connect")
	expectError(t, err, "connecting to the api ("+api.url("/v1/whoami")+") failed: ", "")
	if strings.Contains(err.Error(), "(http ") {
		t.Errorf("expected no http code without an answer of the api, got %q", err)
	}
}

func TestEveryAPICommandNeedsTheClientCertificate(t *testing.T) {
	for _, command := range [][]string{
		{"clusterdata", "read"}, {"clusterdata", "write", "{}"}, {"connect"}, {"failkeeper", "keeper1"},
		{"init", "{}"}, {"promote"}, {"removekeeper", "keeper1"}, {"spec"}, {"status"}, {"update", "{}"},
	} {
		t.Run(strings.Join(command, " "), func(t *testing.T) {
			api := newFakeAPI(t)
			usePKIDir(t, t.TempDir())
			stdout, stderr, err := api.run(command...)
			expectError(t, err, "client certificate not found", "create it with `orion-cli pki init`")
			expectNoUsage(t, stdout, stderr)
			if len(api.requests()) != 0 {
				t.Errorf("expected no requests without a client certificate, got %v", api.requests())
			}
		})
	}
}

func TestConnectReportsAnInvalidClientCertificate(t *testing.T) {
	api := newFakeAPI(t)
	dir := t.TempDir()
	files := pki.CLIFiles(dir)
	for _, f := range []string{files.CertFile, files.KeyFile, files.CAFile} {
		if err := os.MkdirAll(filepath.Dir(f), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("not a certificate"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	usePKIDir(t, dir)
	_, _, err := api.run("connect")
	expectError(t, err, "failed to load the client certificate: ", "")
}

func TestConnectRefusesAnAPIOfAnotherCA(t *testing.T) {
	api := newFakeAPI(t)
	api.handle(endpoints.WhoAmIEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, endpoints.WhoAmIResponse{Name: "orion-cli", Method: "certificate"}, "")
	})
	otherCLIDir := t.TempDir()
	usePKIDir(t, otherCLIDir)
	if _, _, err := run(t, "pki", "init", "--api-dir", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	_, _, err := api.run("connect")
	expectError(t, err, "connecting to the api ("+api.url("/v1/whoami")+") failed: ", "")
	expectContains(t, err.Error(), "certificate")
	if len(api.requests()) != 0 {
		t.Errorf("expected the handshake to fail before a request, got %v", api.requests())
	}
}

func TestTheStoreOptionsAreGone(t *testing.T) {
	// the cli only talks to the api: it has no store options
	_, _, err := run(t, "connect", "--cluster-name", "c")
	expectError(t, err, "unknown flag: --cluster-name", "")
}

func TestAnUnknownFlagPrintsTheUsage(t *testing.T) {
	stdout, stderr, err := run(t, "status", "--no-such-flag")
	expectError(t, err, "unknown flag: --no-such-flag", "")
	expectContains(t, stdout+stderr, "Usage:", "orion-cli status [flags]")
}

func TestVersion(t *testing.T) {
	stdout, _, err := run(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "orion-cli version "+cmd.Version+"\n" {
		t.Errorf("unexpected output %q", stdout)
	}
}

func TestTheRootCommandPrintsTheHelp(t *testing.T) {
	stdout, _, err := run(t)
	if err != nil {
		t.Fatal(err)
	}
	expectContains(t, stdout, "orion command line interface", "Available Commands:", "clusterdata", "status")
}

func TestPKIInitCreatesTheCertificates(t *testing.T) {
	cliDir, apiDir := t.TempDir(), t.TempDir()
	usePKIDir(t, cliDir)

	stdout, _, err := run(t, "pki", "init", "--api-dir", apiDir, "--api-san", "api.example.org",
		"--key-algorithm", "ecdsa-p256", "--leaf-validity", "30d")
	if err != nil {
		t.Fatal(err)
	}
	expectContains(t, stdout, "Created the CA and the cli client certificate in "+cliDir,
		"Created the api certificates in "+apiDir, "The private keys of the CA are in "+filepath.Join(cliDir, pki.CLICADir))
	if !pki.CLIFiles(cliDir).Exist() || !pki.APIFiles(apiDir).Exist() {
		t.Fatal("expected the cli and the api files to exist")
	}
	certs, err := pki.APIFiles(apiDir).Certificates()
	if err != nil {
		t.Fatal(err)
	}
	if err = certs[0].VerifyHostname("api.example.org"); err != nil {
		t.Errorf("expected --api-san in the api certificate: %v", err)
	}

	stdout, stderr, err := run(t, "pki", "init", "--api-dir", apiDir)
	expectError(t, err, "failed to create the certificates: refusing to overwrite existing files", "")
	expectNoUsage(t, stdout, stderr)

	if _, _, err = run(t, "pki", "init", "--api-dir", apiDir, "--force"); err != nil {
		t.Errorf("expected --force to overwrite the files: %v", err)
	}
}

func TestPKIInitRefusesAnInvalidValidity(t *testing.T) {
	usePKIDir(t, t.TempDir())
	_, _, err := run(t, "pki", "init", "--api-dir", t.TempDir(), "--root-validity", "forever")
	expectError(t, err, "invalid options: --root-validity: ", "")
}

func TestPKIInitRefusesArguments(t *testing.T) {
	stdout, stderr, err := run(t, "pki", "init", "extra")
	expectError(t, err, `unknown command "extra"`, "")
	expectContains(t, stdout+stderr, "Usage:")
}
