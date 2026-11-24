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
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// testCert returns a certificate (not signed: only its fields matter) named cn, valid from notBefore to notAfter
func testCert(cn string, notBefore, notAfter time.Time) *x509.Certificate {
	return &x509.Certificate{
		Raw:       []byte(cn + notAfter.String()),
		Subject:   pkix.Name{CommonName: cn},
		NotBefore: notBefore,
		NotAfter:  notAfter,
	}
}

// captureStderr sends the error output of the cli to a buffer for the rest of the spec, and forgets earlier warnings
func captureStderr() *bytes.Buffer {
	var buf bytes.Buffer
	CmdCLI.SetErr(&buf)
	resetCertWarnings()
	DeferCleanup(func() {
		CmdCLI.SetErr(nil)
		resetCertWarnings()
	})
	return &buf
}

var _ = Describe("certificate expiry", func() {
	const day = 24 * time.Hour
	now := time.Now()

	Describe("warnExpiring", func() {
		It("warns once about each certificate that expires within 30 days", func() {
			buf := captureStderr()
			soon := testCert("soon", now.Add(-day), now.Add(10*day))
			later := testCert("later", now.Add(-day), now.Add(60*day))
			warnExpiring("cli", []*x509.Certificate{soon, later}, now)
			warnExpiring("api", []*x509.Certificate{soon}, now)
			Ω(buf.String()).To(Equal(fmt.Sprintf("warning: certificate soon of the cli expires at %s: %s\n",
				soon.NotAfter.Format(time.RFC3339), renewHint)))
		})
	})

	Describe("checkValid", func() {
		It("accepts valid certificates", func() {
			Ω(checkValid("cli", []*x509.Certificate{testCert("ok", now.Add(-day), now.Add(day))}, now)).
				To(Succeed())
		})
		It("refuses an expired certificate", func() {
			err := checkValid("cli", []*x509.Certificate{
				testCert("ok", now.Add(-day), now.Add(day)),
				testCert("old", now.Add(-2*day), now.Add(-day)),
			}, now)
			expectError(err, "certificate old of the cli expired at ", renewHint)
		})
		It("refuses a certificate that isn't valid yet", func() {
			err := checkValid("cli", []*x509.Certificate{testCert("new", now.Add(day), now.Add(2*day))}, now)
			expectError(err, "certificate new of the cli is valid from ", "only: check the clock")
		})
	})

	Describe("explainCertificateError", func() {
		It("explains an expired certificate of the api", func() {
			tlsErr := &tls.CertificateVerificationError{Err: x509.CertificateInvalidError{
				Cert: testCert(pki.APIServerName, now.Add(-2*day), now.Add(-day)), Reason: x509.Expired}}
			err := explainCertificateError(fmt.Errorf("GET /v1/whoami: %w", tlsErr))
			expectError(err, "GET /v1/whoami: tls: failed to verify certificate: x509: ", renewHint)
			expectContains(err.Error(), "certificate orion-api of the api expired at ")
			Ω(errors.Is(err, tlsErr)).To(BeTrue(), "expected the TLS error to be kept")
		})
		It("keeps other errors as they are", func() {
			other := errors.New("other")
			Ω(explainCertificateError(other)).To(BeIdenticalTo(other))
			Ω(explainCertificateError(nil)).To(Succeed())
			unknown := x509.UnknownAuthorityError{}
			Ω(explainCertificateError(unknown)).To(Equal(unknown))
		})
		It("keeps the error when the certificate is valid after all", func() {
			err := x509.CertificateInvalidError{Cert: testCert("ok", now.Add(-day), now.Add(day)),
				Reason: x509.Expired}
			Ω(explainCertificateError(err)).To(Equal(err))
		})
	})

	Describe("the api client", func() {
		// The specs use newAPIClient instead of a command: `connect` would save the --pki-dir of the spec in the
		// config of the later specs.

		// shortLived creates certificates (cli and api) with a leaf validity, and makes the cli use them; it returns
		// a fake api with the api certificate
		shortLived := func(validity string) *fakeAPI {
			cliDir := GinkgoT().TempDir()
			apiDir := filepath.Join(GinkgoT().TempDir(), "api")
			usePKIDir(cliDir)
			_, _, err := run("pki", "init", "--api-dir", apiDir, "--leaf-validity", validity)
			Ω(err).NotTo(HaveOccurred())
			tlsConfig, err := pki.APIFiles(apiDir).ServerConfig()
			Ω(err).NotTo(HaveOccurred())
			api := newFakeAPIWithTLS(tlsConfig)
			api.handle(endpoints.WhoAmIEndPoint.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, http.StatusOK, endpoints.WhoAmIResponse{Name: "orion-cli", Method: "certificate"}, "")
			})
			return api
		}
		// whoAmI connects to api twice, like a command that sends more than one request
		whoAmI := func(api *fakeAPI) error {
			port, err := strconv.ParseUint(api.port(), 10, 16)
			Ω(err).NotTo(HaveOccurred())
			conn, err := newAPIClient("127.0.0.1", uint16(port))
			if err != nil {
				return err
			}
			for range 2 {
				var me endpoints.WhoAmIResponse
				if _, err = conn.Get(endpoints.WhoAmIEndPoint, &me); err != nil {
					return err
				}
				conn.CloseIdleConnections()
			}
			return nil
		}

		It("warns once about the certificates of the cli and the api that expire soon", func() {
			api := shortLived("10d")
			buf := captureStderr()
			Ω(whoAmI(api)).To(Succeed())
			Ω(strings.Count(buf.String(), "warning: certificate orion-cli of the cli expires at ")).To(Equal(1))
			Ω(strings.Count(buf.String(), "warning: certificate orion-api of the api expires at ")).To(Equal(1))
		})

		It("doesn't warn about certificates that are valid for long", func() {
			api := shortLived("1y")
			buf := captureStderr()
			Ω(whoAmI(api)).To(Succeed())
			Ω(buf.String()).To(BeEmpty())
		})

		It("refuses an expired client certificate before it connects", func() {
			const validity = 2 * time.Second
			api := shortLived(validity.String())
			time.Sleep(validity)
			expectError(whoAmI(api), "certificate orion-cli of the cli expired at ", renewHint)
			Ω(api.requests()).To(BeEmpty())
		})
	})
})
