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
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
)

var _ = Describe("pki renew cli", func() {
	// newPKI runs pki init with a leaf and intermediate validity in new directories, makes the cli use them, and
	// returns a fake api with the api certificate of it (which answers whoami), the cli directory and the api directory
	newPKI := func(leafValidity, intermediateValidity string) (*fakeAPI, string, string) {
		cliDir := GinkgoT().TempDir()
		apiDir := filepath.Join(GinkgoT().TempDir(), "api")
		usePKIDir(cliDir)
		_, _, err := run("pki", "init", "--api-dir", apiDir, "--leaf-validity", leafValidity,
			"--intermediate-validity", intermediateValidity)
		Ω(err).NotTo(HaveOccurred())
		tlsConfig, err := pki.APIFiles(apiDir).ServerConfig()
		Ω(err).NotTo(HaveOccurred())
		api := newFakeAPIWithTLS(tlsConfig)
		api.handle(endpoints.WhoAmIEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, endpoints.WhoAmIResponse{Name: pki.CLIClientName, Method: "certificate"}, "")
		})
		return api, cliDir, apiDir
	}

	It("renews an expired client certificate, with one the api accepts", func() {
		const validity = 2 * time.Second
		api, cliDir, apiDir := newPKI(validity.String(), "3y")
		time.Sleep(validity)
		_, _, err := api.run("connect")
		expectError(err, "certificate orion-cli of the cli expired at ", renewHint)

		oldCert, err := os.ReadFile(filepath.Join(cliDir, pki.CLICertFile))
		Ω(err).NotTo(HaveOccurred())
		out, stderrOut, err := run("pki", "renew", "cli", "--leaf-validity", "90d")
		Ω(err).NotTo(HaveOccurred())
		Ω(out).To(HavePrefix("Renewed the client certificate orion-cli in " + cliDir + ": valid until "))
		Ω(stderrOut).To(BeEmpty())
		newCert, err := os.ReadFile(filepath.Join(cliDir, pki.CLICertFile))
		Ω(err).NotTo(HaveOccurred())
		Ω(newCert).NotTo(Equal(oldCert))

		// the api of this pki expired too, so the cli can't connect to it: check the new certificate with the CA
		// the api verifies clients with
		_, certs, err := pki.LoadCertPool(filepath.Join(cliDir, pki.CLICertFile))
		Ω(err).NotTo(HaveOccurred())
		Ω(certs[0].NotAfter).To(BeTemporally("~", time.Now().Add(90*oriontls.Day), time.Minute))
		clientCAs, _, err := pki.LoadCertPool(pki.APIFiles(apiDir).CAFile)
		Ω(err).NotTo(HaveOccurred())
		_, err = certs[0].Verify(x509.VerifyOptions{
			Roots: clientCAs, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		})
		Ω(err).NotTo(HaveOccurred())
		_, err = tls.LoadX509KeyPair(filepath.Join(cliDir, pki.CLICertFile), filepath.Join(cliDir, pki.CLIKeyFile))
		Ω(err).NotTo(HaveOccurred(), "expected the new key to belong to the new certificate")
	})

	It("warns when the intermediate expires before the requested validity", func() {
		_, _, _ = newPKI("1d", "10d")
		_, stderrOut, err := run("pki", "renew", "cli")
		Ω(err).NotTo(HaveOccurred())
		Ω(stderrOut).To(ContainSubstring("warning: the certificate expires with the intermediate api-clients at "))
	})

	It("refuses an invalid validity or key algorithm", func() {
		_, _, err := run("pki", "renew", "cli", "--leaf-validity", "forever")
		expectError(err, "--leaf-validity: invalid validity", "")
		_, _, err = run("pki", "renew", "cli", "--key-algorithm", "dsa")
		expectError(err, `--key-algorithm: unknown algorithm "dsa"`, "")
	})
})
