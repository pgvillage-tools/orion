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
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/auth"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/prometheus/client_golang/prometheus"
)

var _ = Describe("certReloader", func() {
	var (
		first, second pki.InitOptions
		apiFiles      pki.TLSFiles
		metrics       *apiMetrics
	)
	// newPKI runs pki init in a new directory
	newPKI := func() pki.InitOptions {
		dir := GinkgoT().TempDir()
		opts := pki.InitOptions{CLIDir: filepath.Join(dir, "cli"), APIDir: filepath.Join(dir, "api")}
		_, err := pki.Init(opts)
		Expect(err).NotTo(HaveOccurred())
		return opts
	}
	// copyFile copies the file name from the api directory of opts to the api directory of the reloader
	copyFile := func(opts pki.InitOptions, name string) {
		data, err := os.ReadFile(filepath.Join(opts.APIDir, name))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(filepath.Join(filepath.Dir(apiFiles.CertFile), name), data, 0o600)).To(Succeed())
	}
	copyAll := func(opts pki.InitOptions) {
		for _, name := range []string{pki.APIServerCertFile, pki.APIServerKeyFile, pki.APIClientCAFile} {
			copyFile(opts, name)
		}
	}
	// serverSerial returns the serial of the certificate the reloader serves new connections with
	serverSerial := func(r *certReloader) *big.Int {
		config, err := r.tlsConfig().GetConfigForClient(&tls.ClientHelloInfo{})
		Expect(err).NotTo(HaveOccurred())
		return config.Certificates[0].Leaf.SerialNumber
	}
	apiSerial := func(opts pki.InitOptions) *big.Int {
		_, certs, err := pki.LoadCertPool(filepath.Join(opts.APIDir, pki.APIServerCertFile))
		Expect(err).NotTo(HaveOccurred())
		return certs[0].SerialNumber
	}

	BeforeEach(func() {
		first, second = newPKI(), newPKI()
		apiFiles = pki.APIFiles(GinkgoT().TempDir())
		metrics = newAPIMetrics(prometheus.NewRegistry())
		copyAll(first)
	})

	It("fails to start with files that can't be used", func() {
		copyFile(second, pki.APIServerKeyFile)
		_, err := newCertReloader(apiFiles, metrics)
		Expect(err).To(MatchError(ContainSubstring("doesn't belong to the certificate")))
	})

	It("serves new connections with new files, and keeps the current ones when they didn't change", func(
		ctx SpecContext) {
		r, err := newCertReloader(apiFiles, metrics)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.reload()).To(MatchError(errUnchanged))

		var isReady atomic.Bool
		isReady.Store(true)
		server := httptest.NewUnstartedServer(auth.Middleware(newMux(ctx, NewHandlers(nil, nil, &isReady).Routes()),
			auth.ClientCertAuthenticator{}))
		server.TLS = r.tlsConfig()
		server.StartTLS()
		DeferCleanup(server.Close)
		whoAmI := func(opts pki.InitOptions) (*x509.Certificate, error) {
			config, configErr := pki.CLIFiles(opts.CLIDir).ClientConfig()
			Expect(configErr).NotTo(HaveOccurred())
			client := http.Client{Transport: &http.Transport{TLSClientConfig: config}, Timeout: 5 * time.Second}
			resp, getErr := client.Get(server.URL + "/" + endpoints.APIVersion + "/" + string(endpoints.WhoAmIEndPoint))
			if getErr != nil {
				return nil, getErr
			}
			_ = resp.Body.Close()
			return resp.TLS.PeerCertificates[0], nil
		}

		peer, err := whoAmI(first)
		Expect(err).NotTo(HaveOccurred())
		Expect(peer.SerialNumber).To(Equal(apiSerial(first)))

		copyAll(second)
		Expect(r.reload()).To(Succeed())
		peer, err = whoAmI(second)
		Expect(err).NotTo(HaveOccurred(), "expected the cli of the new CA to be accepted")
		Expect(peer.SerialNumber).To(Equal(apiSerial(second)))
		_, err = whoAmI(first)
		Expect(err).To(HaveOccurred(), "expected the cli of the old CA to be refused")
	})

	It("keeps the current certificates while the files don't belong together", func() {
		r, err := newCertReloader(apiFiles, metrics)
		Expect(err).NotTo(HaveOccurred())
		// e.g. while the files are copied one by one
		copyFile(second, pki.APIServerCertFile)
		Expect(r.reload()).To(MatchError(ContainSubstring("doesn't belong to the certificate")))
		Expect(serverSerial(r)).To(Equal(apiSerial(first)))

		copyFile(second, pki.APIServerKeyFile)
		copyFile(second, pki.APIClientCAFile)
		Expect(r.reload()).To(Succeed())
		Expect(serverSerial(r)).To(Equal(apiSerial(second)))
	})

	It("refuses a certificate that isn't valid now", func() {
		r, err := newCertReloader(apiFiles, metrics)
		Expect(err).NotTo(HaveOccurred())
		r.now = func() time.Time { return time.Now().Add(20 * 365 * 24 * time.Hour) }
		copyAll(second)
		Expect(r.reload()).To(MatchError(ContainSubstring("is valid from")))
		Expect(serverSerial(r)).To(Equal(apiSerial(first)))
	})

	It("refuses a certificate that isn't signed by the intermediate after it", func() {
		r, err := newCertReloader(apiFiles, metrics)
		Expect(err).NotTo(HaveOccurred())
		copyAll(second)
		// the certificate and key of second, with the intermediate of first
		_, firstChain, err := pki.LoadCertPool(filepath.Join(first.APIDir, pki.APIServerCertFile))
		Expect(err).NotTo(HaveOccurred())
		certPEM, err := os.ReadFile(filepath.Join(second.APIDir, pki.APIServerCertFile))
		Expect(err).NotTo(HaveOccurred())
		leafPEM, _ := splitFirstPEM(certPEM)
		Expect(os.WriteFile(apiFiles.CertFile, append(leafPEM, pemOf(firstChain[1])...), 0o600)).To(Succeed())
		Expect(r.reload()).To(MatchError(ContainSubstring("isn't signed by the next one in the file")))
		Expect(serverSerial(r)).To(Equal(apiSerial(first)))
	})

	It("updates the expiry metric", func() {
		r, err := newCertReloader(apiFiles, metrics)
		Expect(err).NotTo(HaveOccurred())
		// serials returns the serials in the expiry metric
		serials := func() []string {
			families, gatherErr := metrics.registry.Gather()
			Expect(gatherErr).NotTo(HaveOccurred())
			var result []string
			for _, f := range families {
				if f.GetName() != "orion_api_certificate_expiry_timestamp_seconds" {
					continue
				}
				for _, m := range f.GetMetric() {
					for _, l := range m.GetLabel() {
						if l.GetName() == "serial" {
							result = append(result, l.GetValue())
						}
					}
				}
			}
			return result
		}
		serialOf := func(opts pki.InitOptions) string { return hex.EncodeToString(apiSerial(opts).Bytes()) }
		Expect(serials()).To(ContainElement(serialOf(first)))
		copyAll(second)
		Expect(r.reload()).To(Succeed())
		Expect(serials()).To(And(ContainElement(serialOf(second)), Not(ContainElement(serialOf(first)))))
		Expect(serials()).To(HaveLen(len(r.certificates())), "expected only the certificates in use")
	})

	It("reloads on SIGHUP and periodically until the context is done", func() {
		r, err := newCertReloader(apiFiles, metrics)
		Expect(err).NotTo(HaveOccurred())
		hup := make(chan os.Signal, 1)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			r.watch(ctx, time.Hour, hup)
		}()
		copyAll(second)
		hup <- syscall.SIGHUP
		Eventually(func() *big.Int { return serverSerial(r) }).Should(Equal(apiSerial(second)))
		cancel()
		Eventually(done).Should(BeClosed())

		ctx, cancel = context.WithCancel(context.Background())
		DeferCleanup(cancel)
		go r.watch(ctx, 10*time.Millisecond, nil)
		copyAll(first)
		Eventually(func() *big.Int { return serverSerial(r) }).Should(Equal(apiSerial(first)))
	})
})

// splitFirstPEM returns the first PEM block in data, and the rest
func splitFirstPEM(data []byte) (first, rest []byte) {
	block, rest := pem.Decode(data)
	Expect(block).NotTo(BeNil())
	return pem.EncodeToMemory(block), rest
}

// pemOf returns c PEM encoded
func pemOf(c *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})
}
