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
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/pgvillage-tools/orion/pkg/bootstrap"
)

const testBootstrapSecret = "the bootstrap secret of the api"

// portOf returns the port of the listener of server
func portOf(server *httptest.Server) string {
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	Ω(err).NotTo(HaveOccurred())
	return port
}

// bootstrapAPI starts a fake api in mode: a probe port with /status, and an api port with the bootstrap endpoints
// for testBootstrapSecret. It returns the flags for the cli to reach it.
func bootstrapAPI(mode string) []string {
	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Ω(r.URL.Path).To(Equal("/" + string(endpoints.ModeEndPoint)))
		_ = json.NewEncoder(w).Encode(endpoints.ModeResponse{Mode: mode})
	}))
	DeferCleanup(probe.Close)

	var apiNonce []byte
	mux := http.NewServeMux()
	mux.HandleFunc(endpoints.BootstrapChallengeEndPoint.Route(endpoints.MethodPost),
		func(w http.ResponseWriter, _ *http.Request) {
			apiNonce = make([]byte, bootstrap.NonceSize)
			_ = json.NewEncoder(w).Encode(endpoints.BootstrapChallengeResponse{Nonce: apiNonce})
		})
	mux.HandleFunc(endpoints.BootstrapProofEndPoint.Route(endpoints.MethodPost),
		func(w http.ResponseWriter, r *http.Request) {
			var req endpoints.BootstrapProofRequest
			Ω(json.NewDecoder(r.Body).Decode(&req)).To(Succeed())
			ekm, err := bootstrap.ExportKeyingMaterial(r.TLS)
			Ω(err).NotTo(HaveOccurred())
			want, err := bootstrap.CLIProof([]byte(testBootstrapSecret), apiNonce, req.Nonce, ekm)
			Ω(err).NotTo(HaveOccurred())
			if !bootstrap.Equal(want, req.Proof) {
				endpoints.WriteProblem(w, http.StatusForbidden, "wrong bootstrap proof")
				return
			}
			proof, err := bootstrap.APIProof([]byte(testBootstrapSecret), apiNonce, req.Nonce, ekm)
			Ω(err).NotTo(HaveOccurred())
			_ = json.NewEncoder(w).Encode(endpoints.BootstrapProofResponse{Proof: proof})
		})
	api := httptest.NewUnstartedServer(mux)
	api.StartTLS()
	DeferCleanup(api.Close)
	return []string{"--host", "127.0.0.1", "--port", portOf(api), "--probe-port", portOf(probe), "--timeout", "5s"}
}

// bootstrapSecretEnv sets ORIONCLI_BOOTSTRAP_SECRET to value for the spec ("" unsets it)
func bootstrapSecretEnv(value string) {
	if value == "" {
		Ω(os.Unsetenv(envBootstrapSecret)).To(Succeed())
	} else {
		Ω(os.Setenv(envBootstrapSecret, value)).To(Succeed())
	}
	DeferCleanup(os.Unsetenv, envBootstrapSecret)
}

var _ = Describe("bootstrapSecret", func() {
	It("reads the secret from a file, without the newline", func() {
		file := filepath.Join(GinkgoT().TempDir(), "secret")
		Ω(os.WriteFile(file, []byte(testBootstrapSecret+"\n"), 0o600)).To(Succeed())
		Ω(bootstrapSecret(file)).To(Equal([]byte(testBootstrapSecret)))
	})

	It("reads the secret from the environment", func() {
		bootstrapSecretEnv(testBootstrapSecret)
		Ω(bootstrapSecret("")).To(Equal([]byte(testBootstrapSecret)))
	})

	It("prompts on a terminal", func() {
		bootstrapSecretEnv("")
		fakeTerminal(testBootstrapSecret)
		Ω(bootstrapSecret("")).To(Equal([]byte(testBootstrapSecret)))
	})

	It("fails without a secret", func() {
		bootstrapSecretEnv("")
		_, err := bootstrapSecret("")
		Ω(err).To(MatchError(ContainSubstring("the bootstrap secret is required")))
	})

	It("fails on a missing file", func() {
		_, err := bootstrapSecret(filepath.Join(GinkgoT().TempDir(), "missing"))
		Ω(err).To(HaveOccurred())
	})

	It("refuses a secret that is too short to be the one of the api", func() {
		bootstrapSecretEnv("short")
		_, err := bootstrapSecret("")
		Ω(err).To(MatchError(ContainSubstring("at least 20 characters")))
	})

	It("fails when the terminal can't be read", func() {
		bootstrapSecretEnv("")
		fakeTerminal()
		_, err := bootstrapSecret("")
		Ω(err).To(HaveOccurred())
	})
})

var _ = Describe("bootstrap", func() {
	var dir string

	BeforeEach(func() {
		dir = filepath.Join(GinkgoT().TempDir(), "cli")
		usePKIDir(dir)
	})

	It("creates the key pair, and proves the secret with the api", func() {
		bootstrapSecretEnv(testBootstrapSecret)
		out, _, err := run(append([]string{"bootstrap"}, bootstrapAPI(endpoints.ModeBootstrap)...)...)
		Ω(err).NotTo(HaveOccurred())
		Ω(out).To(ContainSubstring("Created the bootstrap key pair in " + dir))
		Ω(out).To(ContainSubstring("The api proved that it has the bootstrap secret"))
		_, err = bootstrap.ReadPrivateKey(dir, []byte(testCAPassphrase))
		Ω(err).NotTo(HaveOccurred(), "the private key is encrypted with the passphrase of the CA keys")

		pub, err := os.ReadFile(filepath.Join(dir, bootstrap.PublicKeyFile))
		Ω(err).NotTo(HaveOccurred())
		out, _, err = run(append([]string{"bootstrap"}, bootstrapAPI(endpoints.ModeBootstrap)...)...)
		Ω(err).NotTo(HaveOccurred())
		Ω(out).NotTo(ContainSubstring("Created the bootstrap key pair"), "the key pair exists already")
		Ω(os.ReadFile(filepath.Join(dir, bootstrap.PublicKeyFile))).To(Equal(pub))
	})

	It("fails with another secret than the one of the api", func() {
		bootstrapSecretEnv("not the secret of the api")
		_, _, err := run(append([]string{"bootstrap"}, bootstrapAPI(endpoints.ModeBootstrap)...)...)
		Ω(err).To(MatchError(ContainSubstring("wrong bootstrap proof")))
	})

	It("refuses an api in normal mode", func() {
		bootstrapSecretEnv(testBootstrapSecret)
		_, _, err := run(append([]string{"bootstrap"}, bootstrapAPI(endpoints.ModeNormal)...)...)
		Ω(err).To(MatchError(ContainSubstring("not in bootstrap mode")))
		Ω(filepath.Join(dir, bootstrap.PublicKeyFile)).NotTo(BeAnExistingFile(), "no key pair without a bootstrap")
	})

	It("fails without a probe port", func() {
		bootstrapSecretEnv(testBootstrapSecret)
		flags := bootstrapAPI(endpoints.ModeBootstrap)
		for i, f := range flags {
			if f == "--probe-port" {
				flags[i+1] = "1"
			}
		}
		_, _, err := run(append([]string{"bootstrap"}, flags...)...)
		Ω(err).To(MatchError(ContainSubstring("probe port")))
	})

	It("fails without a passphrase for a new key pair", func() {
		bootstrapSecretEnv(testBootstrapSecret)
		noPassphraseEnv()
		_, _, err := run(append([]string{"bootstrap"}, bootstrapAPI(endpoints.ModeBootstrap)...)...)
		Ω(err).To(MatchError(ContainSubstring("passphrase")))
	})

	It("is hidden until the api supports bootstrap mode", func() {
		out, _, err := run("--help")
		Ω(err).NotTo(HaveOccurred())
		Ω(strings.Contains(out, "bootstrap")).To(BeFalse())
	})
})
