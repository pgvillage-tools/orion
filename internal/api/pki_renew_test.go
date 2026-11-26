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

package api

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/auth"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
	"github.com/prometheus/client_golang/prometheus"
)

var _ = Describe("online renewal of the api certificate", func() {
	var (
		opts     pki.InitOptions
		h        *Handlers
		server   *httptest.Server
		apiFiles pki.TLSFiles
	)
	BeforeEach(func(ctx SpecContext) {
		dir := GinkgoT().TempDir()
		opts = pki.InitOptions{CLIDir: filepath.Join(dir, "cli"), APIDir: filepath.Join(dir, "api")}
		_, err := pki.Init(opts)
		Expect(err).NotTo(HaveOccurred())
		apiFiles = pki.APIFiles(opts.APIDir)
		reloader, err := newCertReloader(apiFiles, newAPIMetrics(prometheus.NewRegistry()))
		Expect(err).NotTo(HaveOccurred())
		var isReady atomic.Bool
		isReady.Store(true)
		h = NewHandlers(nil, nil, &isReady)
		h.certs = reloader
		server = httptest.NewUnstartedServer(auth.Middleware(newMux(ctx, h.Routes()), auth.ClientCertAuthenticator{}))
		server.TLS = reloader.tlsConfig()
		server.StartTLS()
		DeferCleanup(server.Close)
	})

	// clientOf returns an http client that logs in with the certificate and key in files
	clientOf := func(files pki.TLSFiles) *http.Client {
		config, err := files.ClientConfig()
		Expect(err).NotTo(HaveOccurred())
		config.ServerName = "localhost"
		return &http.Client{Transport: &http.Transport{TLSClientConfig: config}, Timeout: 5 * time.Second}
	}
	cli := func() *http.Client { return clientOf(pki.CLIFiles(opts.CLIDir)) }
	// call sends body as JSON with method to ep, and decodes a 200 response into out; it returns the status
	call := func(client *http.Client, method string, ep endpoints.EndPoint, body, out any) (int, string) {
		data, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		req, err := http.NewRequest(method, server.URL+"/"+endpoints.APIVersion+"/"+string(ep), bytes.NewReader(data))
		Expect(err).NotTo(HaveOccurred())
		resp, err := client.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			var problem map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&problem)
			detail, _ := problem["detail"].(string)
			return resp.StatusCode, detail
		}
		if out != nil {
			Expect(json.NewDecoder(resp.Body).Decode(out)).To(Succeed())
		}
		return resp.StatusCode, ""
	}
	requestCSR := func(algorithm string) []byte {
		var resp endpoints.APICertificateRequestResponse
		code, detail := call(cli(), http.MethodPost, endpoints.APICertificateRequestEndPoint,
			endpoints.APICertificateRequestRequest{KeyAlgorithm: algorithm}, &resp)
		Expect(code).To(Equal(http.StatusOK), detail)
		return resp.Request
	}
	currentCert := func() *x509.Certificate {
		_, certs, err := pki.LoadCertPool(apiFiles.CertFile)
		Expect(err).NotTo(HaveOccurred())
		return certs[0]
	}
	sign := func(request []byte, sans ...string) *pki.SignedAPIRequest {
		signed, err := pki.SignAPIRequest(pki.SignAPIRequestOptions{
			CLIDir: opts.CLIDir, Request: request, Current: currentCert(), APISANs: sans, LeafExpiry: 90 * oriontls.Day,
		})
		Expect(err).NotTo(HaveOccurred())
		return signed
	}
	// servedSerial returns the serial of the certificate the api serves a new connection with
	servedSerial := func() string {
		conn, err := tls.Dial("tcp", server.Listener.Addr().String(), &tls.Config{
			MinVersion: tls.VersionTLS13, InsecureSkipVerify: true, //nolint:gosec // only reads the certificate
		})
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = conn.Close() }()
		return conn.ConnectionState().PeerCertificates[0].SerialNumber.String()
	}

	It("renews the certificate with a key that the api made, without a restart", func() {
		oldKey, err := os.ReadFile(apiFiles.KeyFile)
		Expect(err).NotTo(HaveOccurred())
		signed := sign(requestCSR(""), "orion.example.org")

		var resp endpoints.APICertificateResponse
		code, detail := call(cli(), http.MethodPut, endpoints.APICertificateEndPoint,
			endpoints.APICertificate{Chain: signed.Chain}, &resp)
		Expect(code).To(Equal(http.StatusOK), detail)
		Expect(resp.NotAfter).To(BeTemporally("~", signed.Renewed.NotAfter, time.Second))
		Expect(servedSerial()).To(Equal(signed.Renewed.SerialNumber.String()))
		Expect(currentCert().SerialNumber).To(Equal(signed.Renewed.SerialNumber))
		Expect(currentCert().DNSNames).To(ContainElement("orion.example.org"))
		newKey, err := os.ReadFile(apiFiles.KeyFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(newKey).NotTo(Equal(oldKey))
		_, err = tls.LoadX509KeyPair(apiFiles.CertFile, apiFiles.KeyFile)
		Expect(err).NotTo(HaveOccurred())

		// the key is used up
		code, _ = call(cli(), http.MethodPut, endpoints.APICertificateEndPoint,
			endpoints.APICertificate{Chain: signed.Chain}, &resp)
		Expect(code).To(Equal(http.StatusConflict))
	})

	It("makes a key of the algorithm that is asked", func() {
		signed := sign(requestCSR(string(oriontls.KeyAlgorithmRSA2048)))
		Expect(signed.Renewed.PublicKeyAlgorithm).To(Equal(x509.RSA))
		code, _ := call(cli(), http.MethodPost, endpoints.APICertificateRequestEndPoint,
			endpoints.APICertificateRequestRequest{KeyAlgorithm: "dsa"}, nil)
		Expect(code).To(Equal(http.StatusBadRequest))
	})

	It("only lets the cli renew", func() {
		intermediate, err := pki.LoadIntermediate(opts.CLIDir, pki.APIClientIntermediate, nil)
		Expect(err).NotTo(HaveOccurred())
		subject := intermediate.Cert.Subject.SetCommonName("someone")
		other, err := pki.IssueLeaf(intermediate, pki.LeafRequest{
			Subject: subject, ExtKeyUsage: oriontls.DefaultClientExtKeyUsages, Expiry: oriontls.Day,
			KeyAlgorithm: oriontls.DefaultKeyAlgorithm,
		})
		Expect(err).NotTo(HaveOccurred())
		dir := GinkgoT().TempDir()
		files := pki.TLSFiles{
			CertFile: filepath.Join(dir, "tls.crt"), KeyFile: filepath.Join(dir, "tls.key"),
			CAFile: pki.CLIFiles(opts.CLIDir).CAFile,
		}
		Expect(os.WriteFile(files.CertFile, other.Cert.PEM, 0o600)).To(Succeed())
		Expect(os.WriteFile(files.KeyFile, other.PrivateKey.PEM, 0o600)).To(Succeed())
		code, _ := call(clientOf(files), http.MethodPost, endpoints.APICertificateRequestEndPoint, nil, nil)
		Expect(code).To(Equal(http.StatusForbidden))
		code, _ = call(clientOf(files), http.MethodPut, endpoints.APICertificateEndPoint, nil, nil)
		Expect(code).To(Equal(http.StatusForbidden))
	})

	It("needs a certificate request first, and a recent one", func() {
		signed := sign(requestCSR(""))
		h.renewal.mu.Lock()
		h.renewal.created = time.Now().Add(-renewalTimeout - time.Second)
		h.renewal.mu.Unlock()
		code, detail := call(cli(), http.MethodPut, endpoints.APICertificateEndPoint,
			endpoints.APICertificate{Chain: signed.Chain}, nil)
		Expect(code).To(Equal(http.StatusConflict))
		Expect(detail).To(ContainSubstring("request one first"))
	})

	DescribeTable("refuses a certificate that doesn't fit, and keeps the current one",
		func(chain func(signed *pki.SignedAPIRequest) []byte, expected string) {
			before := servedSerial()
			signed := sign(requestCSR(""))
			code, detail := call(cli(), http.MethodPut, endpoints.APICertificateEndPoint,
				endpoints.APICertificate{Chain: chain(signed)}, nil)
			Expect(code).To(Equal(http.StatusBadRequest))
			Expect(detail).To(ContainSubstring(expected))
			Expect(servedSerial()).To(Equal(before))
		},
		Entry("for the key of an earlier request", func(signed *pki.SignedAPIRequest) []byte {
			// a new request replaces the key of the one signed
			_ = requestCSR("")
			return signed.Chain
		}, "isn't for the key of the last certificate request"),
		Entry("without its intermediate", func(signed *pki.SignedAPIRequest) []byte {
			block, _ := pemDecode(signed.Chain)
			return block
		}, "expected the certificate and its intermediate, got 1"),
		Entry("of another CA", func(signed *pki.SignedAPIRequest) []byte {
			dir := GinkgoT().TempDir()
			other := pki.InitOptions{CLIDir: filepath.Join(dir, "cli"), APIDir: filepath.Join(dir, "api")}
			_, err := pki.Init(other)
			Expect(err).NotTo(HaveOccurred())
			intermediate, err := os.ReadFile(filepath.Join(other.CLIDir, pki.CLICADir, pki.ServerIntermediate,
				"ca.crt"))
			Expect(err).NotTo(HaveOccurred())
			leaf, _ := pemDecode(signed.Chain)
			return append(leaf, intermediate...)
		}, "isn't signed by the intermediate the api has"),
	)

	It("can't renew without certificates", func() {
		h.certs = nil
		code, _ := call(cli(), http.MethodPost, endpoints.APICertificateRequestEndPoint, nil, nil)
		Expect(code).To(Equal(http.StatusConflict))
	})
})

// pemDecode returns the first PEM block of data, PEM encoded, and the rest
func pemDecode(data []byte) (first, rest []byte) {
	return splitFirstPEM(data)
}
