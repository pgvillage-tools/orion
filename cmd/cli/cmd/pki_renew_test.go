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
	"crypto"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
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

var _ = Describe("pki renew api", func() {
	It("writes api files that the cli trusts, with the names of the current certificate", func() {
		cliDir := GinkgoT().TempDir()
		apiDir := filepath.Join(GinkgoT().TempDir(), "api")
		usePKIDir(cliDir)
		_, _, err := run("pki", "init", "--api-dir", apiDir, "--api-san", "orion-api.example.org")
		Ω(err).NotTo(HaveOccurred())

		out, _, err := run("pki", "renew", "api", "--api-dir", apiDir, "--api-san", "orion.example.org",
			"--leaf-validity", "90d")
		Ω(err).NotTo(HaveOccurred())
		Ω(out).To(HavePrefix("Renewed the api certificate orion-api in " + apiDir + ": valid until "))
		Ω(out).To(ContainSubstring("Names on the certificate: orion-api, localhost, 127.0.0.1, ::1, " +
			"orion-api.example.org, orion.example.org"))
		Ω(out).To(ContainSubstring("Copy server.crt, server.key and client-ca.crt to the --pki-dir of the api"))

		tlsConfig, err := pki.APIFiles(apiDir).ServerConfig()
		Ω(err).NotTo(HaveOccurred())
		api := newFakeAPIWithTLS(tlsConfig)
		out, _, err = api.run("pki", "list")
		Ω(err).NotTo(HaveOccurred())
		Ω(out).To(ContainSubstring("is trusted by the root of the cli"))
		Ω(out).To(MatchRegexp(`api\s+-\s+orion-api\s.*valid \(8\dd left\)`))
	})

	Describe("online", func() {
		var (
			api     *fakeAPI
			serving atomic.Pointer[tls.Certificate]
			pending crypto.Signer
		)
		BeforeEach(func() {
			cliDir := GinkgoT().TempDir()
			apiDir := filepath.Join(GinkgoT().TempDir(), "api")
			usePKIDir(cliDir)
			_, _, err := run("pki", "init", "--api-dir", apiDir, "--api-san", "orion-api.example.org")
			Ω(err).NotTo(HaveOccurred())
			files := pki.APIFiles(apiDir)
			pair, err := tls.LoadX509KeyPair(files.CertFile, files.KeyFile)
			Ω(err).NotTo(HaveOccurred())
			serving.Store(&pair)
			clientCAs, _, err := pki.LoadCertPool(files.CAFile)
			Ω(err).NotTo(HaveOccurred())
			// like the api: new connections get the certificate it serves with now
			api = newFakeAPIWithTLS(&tls.Config{
				MinVersion: tls.VersionTLS13,
				GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
					return &tls.Config{
						MinVersion:   tls.VersionTLS13,
						Certificates: []tls.Certificate{*serving.Load()},
						ClientCAs:    clientCAs,
						ClientAuth:   tls.RequireAndVerifyClientCert,
					}, nil
				},
			})
			api.handle(endpoints.APICertificateRequestEndPoint.Route(endpoints.MethodPost),
				func(w http.ResponseWriter, _ *http.Request) {
					key := oriontls.PrivateKey{Algorithm: oriontls.DefaultKeyAlgorithm}
					Ω(key.Generate()).To(Succeed())
					pending = key.Signer()
					der, csrErr := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
						Subject: pkix.Name{CommonName: pki.APIServerName},
					}, pending)
					Ω(csrErr).NotTo(HaveOccurred())
					writeJSON(w, http.StatusOK, endpoints.APICertificateRequestResponse{
						Request: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}),
					}, "")
				})
			api.handle(endpoints.APICertificateEndPoint.Route(endpoints.MethodPut),
				func(w http.ResponseWriter, r *http.Request) {
					var cert endpoints.APICertificate
					Ω(json.NewDecoder(r.Body).Decode(&cert)).To(Succeed())
					keyDER, keyErr := x509.MarshalPKCS8PrivateKey(pending)
					Ω(keyErr).NotTo(HaveOccurred())
					renewed, pairErr := tls.X509KeyPair(cert.Chain,
						pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
					Ω(pairErr).NotTo(HaveOccurred())
					serving.Store(&renewed)
					writeJSON(w, http.StatusOK, endpoints.APICertificateResponse{
						Serial: renewed.Leaf.SerialNumber.Text(16), NotAfter: renewed.Leaf.NotAfter,
					}, "")
				})
		})

		It("renews the certificate on the api, which keeps its key", func() {
			before := serving.Load().Leaf.SerialNumber
			out, _, err := api.run("pki", "renew", "api", "--api-san", "orion.example.org", "--leaf-validity", "90d")
			Ω(err).NotTo(HaveOccurred())
			Ω(out).To(HavePrefix("Renewed the api certificate orion-api in the api at 127.0.0.1:" + api.port() +
				": valid until "))
			Ω(out).To(ContainSubstring("Names on the certificate: orion-api, localhost, 127.0.0.1, ::1, " +
				"orion-api.example.org, orion.example.org"))
			Ω(out).To(ContainSubstring("its private key never left the api"))
			leaf := serving.Load().Leaf
			Ω(leaf.SerialNumber).NotTo(Equal(before))
			Ω(leaf.NotAfter).To(BeTemporally("~", time.Now().Add(90*oriontls.Day), time.Minute))
			Ω(api.requestsTo(http.MethodPost, "/v1/pki/api/csr")).To(HaveLen(1))
			Ω(api.requestsTo(http.MethodPut, "/v1/pki/api/certificate")).To(HaveLen(1))
		})

		It("fails when the api doesn't serve the new certificate", func() {
			// an api that answers, but keeps its certificate
			api.mux = http.NewServeMux()
			api.handle(endpoints.APICertificateRequestEndPoint.Route(endpoints.MethodPost),
				func(w http.ResponseWriter, _ *http.Request) {
					key := oriontls.PrivateKey{Algorithm: oriontls.DefaultKeyAlgorithm}
					Ω(key.Generate()).To(Succeed())
					der, csrErr := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
						Subject: pkix.Name{CommonName: pki.APIServerName},
					}, key.Signer())
					Ω(csrErr).NotTo(HaveOccurred())
					writeJSON(w, http.StatusOK, endpoints.APICertificateRequestResponse{
						Request: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}),
					}, "")
				})
			api.handle(endpoints.APICertificateEndPoint.Route(endpoints.MethodPut),
				func(w http.ResponseWriter, _ *http.Request) {
					writeJSON(w, http.StatusOK, endpoints.APICertificateResponse{}, "")
				})
			_, _, err := api.run("pki", "renew", "api")
			expectError(err, "the api took the new certificate (serial ", "")
		})
	})

	It("renews an expired api certificate offline only", func() {
		const validity = 2 * time.Second
		cliDir := GinkgoT().TempDir()
		apiDir := filepath.Join(GinkgoT().TempDir(), "api")
		usePKIDir(cliDir)
		_, _, err := run("pki", "init", "--api-dir", apiDir, "--leaf-validity", validity.String())
		Ω(err).NotTo(HaveOccurred())
		tlsConfig, err := pki.APIFiles(apiDir).ServerConfig()
		Ω(err).NotTo(HaveOccurred())
		api := newFakeAPIWithTLS(tlsConfig)
		time.Sleep(validity)
		_, _, err = api.run("pki", "renew", "api")
		expectError(err, "the certificate of the api at 127.0.0.1:"+api.port()+" expired at ",
			"renew it offline, with --api-dir, and copy the files to the api")
		Ω(api.requests()).To(BeEmpty())
	})
})
