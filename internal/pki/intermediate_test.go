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

package pki_test

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/pgvillage-tools/orion/internal/pki"
	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
)

var _ = Describe("RenewIntermediate", func() {
	passphrase := []byte("intermediate-test-passphrase")

	newPKI := func(change func(*pki.InitOptions)) pki.InitOptions {
		dir := GinkgoT().TempDir()
		opts := pki.InitOptions{
			CLIDir:       filepath.Join(dir, "cli"),
			APIDir:       filepath.Join(dir, "api"),
			CAPassphrase: passphrase,
			Subject:      pki.SubjectOptions{Organisation: "Acme", Country: "NL"},
		}
		if change != nil {
			change(&opts)
		}
		_, err := pki.Init(opts)
		Ω(err).NotTo(HaveOccurred())
		return opts
	}
	caFile := func(opts pki.InitOptions, elem ...string) string {
		return filepath.Join(append([]string{opts.CLIDir, pki.CLICADir}, elem...)...)
	}
	certOf := func(path string) *x509.Certificate {
		_, certs, err := pki.LoadCertPool(path)
		Ω(err).NotTo(HaveOccurred())
		return certs[0]
	}
	renew := func(opts pki.InitOptions, name string) *pki.RenewResult {
		result, err := pki.RenewIntermediate(pki.RenewIntermediateOptions{
			CLIDir: opts.CLIDir, Name: name, CAPassphrase: passphrase, Expiry: 2 * oriontls.Year,
		})
		Ω(err).NotTo(HaveOccurred())
		return result
	}

	It("replaces the server intermediate, after which the api certificate can be renewed", func() {
		opts := newPKI(nil)
		oldCA, err := os.ReadFile(caFile(opts, pki.ServerIntermediate, "ca.crt"))
		Ω(err).NotTo(HaveOccurred())
		oldAPICert := certOf(pki.APIFiles(opts.APIDir).CertFile)

		result := renew(opts, pki.ServerIntermediate)
		Ω(result.Renewed.Subject.String()).To(Equal(result.Previous.Subject.String()))
		Ω(result.Renewed.IsCA).To(BeTrue())
		Ω(result.Renewed.MaxPathLenZero).To(BeTrue())
		Ω(result.Renewed.KeyUsage & x509.KeyUsageCertSign).NotTo(BeZero())
		Ω(result.Renewed.NotAfter).To(BeTemporally("~", time.Now().Add(2*oriontls.Year), time.Minute))
		Ω(result.Renewed.CheckSignatureFrom(certOf(caFile(opts, "root.crt")))).To(Succeed())
		Ω(result.Capped).To(BeFalse())
		previous, err := os.ReadFile(caFile(opts, pki.ServerIntermediate, pki.PreviousIntermediateFile))
		Ω(err).NotTo(HaveOccurred())
		Ω(previous).To(Equal(oldCA))

		// a new key, encrypted with the passphrase
		intermediate, err := pki.LoadIntermediate(opts.CLIDir, pki.ServerIntermediate, passphrase)
		Ω(err).NotTo(HaveOccurred())
		Ω(intermediate.Cert.X509().Equal(result.Renewed)).To(BeTrue())
		Ω(oldAPICert.CheckSignatureFrom(result.Renewed)).NotTo(Succeed())
		_, err = pki.LoadIntermediate(opts.CLIDir, pki.ServerIntermediate, nil)
		Ω(err).To(HaveOccurred())

		// the api certificate of the previous intermediate is renewed with the new one
		renewed, _, err := pki.RenewAPI(pki.RenewAPIOptions{
			CLIDir: opts.CLIDir, APIDir: opts.APIDir, CAPassphrase: passphrase, LeafExpiry: 90 * oriontls.Day,
		})
		Ω(err).NotTo(HaveOccurred())
		roots, _, err := pki.LoadCertPool(pki.CLIFiles(opts.CLIDir).CAFile)
		Ω(err).NotTo(HaveOccurred())
		chain, chainCerts, err := pki.LoadCertPool(pki.APIFiles(opts.APIDir).CertFile)
		Ω(err).NotTo(HaveOccurred())
		Ω(chainCerts).To(HaveLen(2))
		Ω(chainCerts[1].Equal(result.Renewed)).To(BeTrue())
		_, err = renewed.Renewed.Verify(x509.VerifyOptions{Roots: roots, Intermediates: chain, DNSName: "localhost"})
		Ω(err).NotTo(HaveOccurred())
	})

	It("replaces the api client intermediate, after which the cli certificate can be renewed", func() {
		opts := newPKI(nil)
		result := renew(opts, pki.APIClientIntermediate)

		_, err := pki.RenewCLI(pki.RenewCLIOptions{
			CLIDir: opts.CLIDir, CAPassphrase: passphrase, LeafExpiry: 90 * oriontls.Day,
		})
		Ω(err).NotTo(HaveOccurred())
		path, err := pki.WriteAPIClientCA(opts.CLIDir, opts.APIDir)
		Ω(err).NotTo(HaveOccurred())
		Ω(path).To(Equal(pki.APIFiles(opts.APIDir).CAFile))
		clientCAs, caCerts, err := pki.LoadCertPool(path)
		Ω(err).NotTo(HaveOccurred())
		Ω(caCerts[0].Equal(result.Renewed)).To(BeTrue())
		_, err = certOf(pki.CLIFiles(opts.CLIDir).CertFile).Verify(x509.VerifyOptions{
			Roots: clientCAs, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		})
		Ω(err).NotTo(HaveOccurred())
	})

	It("keeps the key algorithm, unless another one is asked", func() {
		opts := newPKI(nil)
		result := renew(opts, pki.ServerIntermediate)
		Ω(result.Renewed.PublicKeyAlgorithm).To(Equal(result.Previous.PublicKeyAlgorithm))
		rsa, err := pki.RenewIntermediate(pki.RenewIntermediateOptions{
			CLIDir: opts.CLIDir, Name: pki.ServerIntermediate, CAPassphrase: passphrase, Expiry: oriontls.Year,
			KeyAlgorithm: oriontls.KeyAlgorithmRSA2048,
		})
		Ω(err).NotTo(HaveOccurred())
		Ω(rsa.Renewed.PublicKeyAlgorithm).To(Equal(x509.RSA))
	})

	It("never outlives the root", func() {
		opts := newPKI(func(o *pki.InitOptions) { o.RootExpiry = 30 * oriontls.Day })
		result := renew(opts, pki.APIClientIntermediate)
		Ω(result.Capped).To(BeTrue())
		Ω(result.Renewed.NotAfter).To(Equal(result.Intermediate.NotAfter))
	})

	It("refuses an unknown intermediate or a wrong passphrase, and changes nothing", func() {
		opts := newPKI(nil)
		before, err := os.ReadFile(caFile(opts, pki.ServerIntermediate, "ca.crt"))
		Ω(err).NotTo(HaveOccurred())
		_, err = pki.RenewIntermediate(pki.RenewIntermediateOptions{
			CLIDir: opts.CLIDir, Name: "root", CAPassphrase: passphrase, Expiry: oriontls.Year,
		})
		Ω(err).To(MatchError(ContainSubstring(`unknown intermediate "root"`)))
		_, err = pki.RenewIntermediate(pki.RenewIntermediateOptions{
			CLIDir: opts.CLIDir, Name: pki.ServerIntermediate, CAPassphrase: []byte("wrong passphrase"),
			Expiry: oriontls.Year,
		})
		Ω(err).To(MatchError(ContainSubstring("failed to load the root")))
		after, err := os.ReadFile(caFile(opts, pki.ServerIntermediate, "ca.crt"))
		Ω(err).NotTo(HaveOccurred())
		Ω(after).To(Equal(before))
		Ω(caFile(opts, pki.ServerIntermediate, pki.PreviousIntermediateFile)).NotTo(BeAnExistingFile())
	})

	It("refuses to write the api client intermediate into the CA directory", func() {
		opts := newPKI(nil)
		_, err := pki.WriteAPIClientCA(opts.CLIDir, opts.CLIDir)
		Ω(err).To(MatchError(ContainSubstring("use separate directories")))
	})
})
