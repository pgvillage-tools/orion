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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/cmd"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

var _ = Describe("connect", func() {
	// handleWhoAmI lets api answer GET /v1/whoami with the identity of the cli
	handleWhoAmI := func(api *fakeAPI) {
		api.handle(endpoints.WhoAmIEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, endpoints.WhoAmIResponse{Name: "orion-cli", Method: "certificate"}, "")
		})
	}

	var api *fakeAPI

	BeforeEach(func() {
		api = newFakeAPI()
	})

	It("prints the identity and saves the config", func() {
		handleWhoAmI(api)
		stdout, _, err := api.run("connect")
		Ω(err).NotTo(HaveOccurred())
		Ω(stdout).To(Equal("Connected to the api as orion-cli (certificate)\n"))
		config, err := os.ReadFile(filepath.Join(testHome, ".orion", "config.yml"))
		Ω(err).NotTo(HaveOccurred())
		// viper writes the flag as a string
		expectContains(string(config), `port: "`+api.port()+`"`, "host: 127.0.0.1")
	})

	It("reports an api error", func() {
		api.handle(endpoints.WhoAmIEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
			endpoints.WriteProblem(w, http.StatusUnauthorized, "unauthorized")
		})
		stdout, stderr, err := api.run("connect")
		expectError(err, "connecting to the api ("+api.url("/v1/whoami")+") failed: GET "+api.url("/v1/whoami")+
			": 401 Unauthorized: unauthorized", httpCodeSuffix(http.StatusUnauthorized))
		expectNoUsage(stdout, stderr)
	})

	It("fails when the api is down", func() {
		api.server.Close()
		_, _, err := api.run("connect")
		expectError(err, "connecting to the api ("+api.url("/v1/whoami")+") failed: ", "")
		Ω(err.Error()).NotTo(ContainSubstring("(http "), "expected no http code without an answer of the api")
	})

	It("reports an invalid client certificate", func() {
		dir := GinkgoT().TempDir()
		files := pki.CLIFiles(dir)
		for _, f := range []string{files.CertFile, files.KeyFile, files.CAFile} {
			Ω(os.MkdirAll(filepath.Dir(f), 0o700)).To(Succeed())
			Ω(os.WriteFile(f, []byte("not a certificate"), 0o600)).To(Succeed())
		}
		usePKIDir(dir)
		_, _, err := api.run("connect")
		expectError(err, "failed to load the client certificate: ", "")
	})

	It("refuses an api of another CA", func() {
		handleWhoAmI(api)
		otherCLIDir := GinkgoT().TempDir()
		usePKIDir(otherCLIDir)
		_, _, err := run("pki", "init", "--api-dir", GinkgoT().TempDir())
		Ω(err).NotTo(HaveOccurred())
		_, _, err = api.run("connect")
		expectError(err, "connecting to the api ("+api.url("/v1/whoami")+") failed: ", "")
		expectContains(err.Error(), "certificate")
		Ω(api.requests()).To(BeEmpty(), "expected the handshake to fail before a request")
	})
})

var _ = Describe("every api command", func() {
	// apiCommandEntries returns an entry for every command that uses the api
	apiCommandEntries := func() []TableEntry {
		var entries []TableEntry
		for _, command := range [][]string{
			{"clusterdata", "read"}, {"clusterdata", "write", "{}"}, {"connect"}, {"failkeeper", "keeper1"},
			{"init", "{}"}, {"promote"}, {"removekeeper", "keeper1"}, {"spec"}, {"status"}, {"update", "{}"},
		} {
			entries = append(entries, Entry(strings.Join(command, " "), command))
		}
		return entries
	}

	DescribeTable("needs the client certificate",
		func(command []string) {
			api := newFakeAPI()
			usePKIDir(GinkgoT().TempDir())
			stdout, stderr, err := api.run(command...)
			expectError(err, "client certificate not found", "create it with `orion-cli pki init`")
			expectNoUsage(stdout, stderr)
			Ω(api.requests()).To(BeEmpty(), "expected no requests without a client certificate")
		},
		apiCommandEntries(),
	)
})

var _ = Describe("orion-cli", func() {
	It("has no store options anymore", func() {
		// the cli only talks to the api: it has no store options
		_, _, err := run("connect", "--cluster-name", "c")
		expectError(err, "unknown flag: --cluster-name", "")
	})

	It("prints the usage for an unknown flag", func() {
		stdout, stderr, err := run("status", "--no-such-flag")
		expectError(err, "unknown flag: --no-such-flag", "")
		expectContains(stdout+stderr, "Usage:", "orion-cli status [flags]")
	})

	It("prints its version", func() {
		stdout, _, err := run("version")
		Ω(err).NotTo(HaveOccurred())
		Ω(stdout).To(Equal("orion-cli version " + cmd.Version + "\n"))
	})

	It("prints the help without a command", func() {
		stdout, _, err := run()
		Ω(err).NotTo(HaveOccurred())
		expectContains(stdout, "orion command line interface", "Available Commands:", "clusterdata", "status")
	})
})

var _ = Describe("pki init", func() {
	It("creates the certificates", func() {
		cliDir, apiDir := GinkgoT().TempDir(), GinkgoT().TempDir()
		usePKIDir(cliDir)

		stdout, _, err := run("pki", "init", "--api-dir", apiDir, "--api-san", "api.example.org",
			"--key-algorithm", "ecdsa-p256", "--leaf-validity", "30d")
		Ω(err).NotTo(HaveOccurred())
		expectContains(stdout, "Created the CA and the cli client certificate in "+cliDir,
			"Created the api certificates in "+apiDir,
			"The private keys of the CA are in "+filepath.Join(cliDir, pki.CLICADir))
		Ω(pki.CLIFiles(cliDir).Check()).To(Succeed(), "expected the cli files to exist")
		Ω(pki.APIFiles(apiDir).Check()).To(Succeed(), "expected the api files to exist")
		certs, err := pki.APIFiles(apiDir).Certificates()
		Ω(err).NotTo(HaveOccurred())
		Ω(certs[0].VerifyHostname("api.example.org")).To(Succeed(), "expected --api-san in the api certificate")

		stdout, stderr, err := run("pki", "init", "--api-dir", apiDir)
		expectError(err, "failed to create the certificates: refusing to overwrite existing files", "")
		expectNoUsage(stdout, stderr)

		_, _, err = run("pki", "init", "--api-dir", apiDir, "--force")
		Ω(err).NotTo(HaveOccurred(), "expected --force to overwrite the files")
	})

	It("puts the subject flags on the certificates", func() {
		apiDir := GinkgoT().TempDir()
		usePKIDir(GinkgoT().TempDir())
		_, _, err := run("pki", "init", "--api-dir", apiDir, "--org", "Acme", "--country", "nl",
			"--state", "Utrecht", "--locality", "Utrecht")
		Ω(err).NotTo(HaveOccurred())
		certs, err := pki.APIFiles(apiDir).Certificates()
		Ω(err).NotTo(HaveOccurred())
		subject := certs[0].Subject
		Ω(subject.CommonName).To(Equal(pki.APIServerName))
		Ω(subject.Organization).To(Equal([]string{"Acme"}))
		Ω(subject.Country).To(Equal([]string{"NL"}))
		Ω(subject.Province).To(Equal([]string{"Utrecht"}))
		Ω(subject.Locality).To(Equal([]string{"Utrecht"}))
		Ω(subject.OrganizationalUnit).To(BeEmpty())
	})

	It("refuses an invalid country", func() {
		usePKIDir(GinkgoT().TempDir())
		_, _, err := run("pki", "init", "--api-dir", GinkgoT().TempDir(), "--country", "Netherlands")
		expectError(err, `failed to create the certificates: invalid country "NETHERLANDS"`, "")
	})

	It("refuses an invalid validity", func() {
		usePKIDir(GinkgoT().TempDir())
		_, _, err := run("pki", "init", "--api-dir", GinkgoT().TempDir(), "--root-validity", "forever")
		expectError(err, "invalid options: --root-validity: ", "")
	})

	It("refuses arguments", func() {
		stdout, stderr, err := run("pki", "init", "extra")
		expectError(err, `unknown command "extra"`, "")
		expectContains(stdout+stderr, "Usage:")
	})
})
