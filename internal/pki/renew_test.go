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
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/pgvillage-tools/orion/internal/pki"
	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
)

var _ = Describe("RenewCLI", func() {
	passphrase := []byte("renew-test-passphrase")

	// newPKI runs Init in a new directory with opts on top of the defaults, and returns the options it used
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
	readFiles := func(files pki.TLSFiles) (cert, key []byte) {
		var err error
		cert, err = os.ReadFile(files.CertFile)
		Ω(err).NotTo(HaveOccurred())
		key, err = os.ReadFile(files.KeyFile)
		Ω(err).NotTo(HaveOccurred())
		return cert, key
	}

	It("replaces the certificate and the key with new ones that the api accepts", func() {
		opts := newPKI(nil)
		files := pki.CLIFiles(opts.CLIDir)
		oldCert, oldKey := readFiles(files)

		result, err := pki.RenewCLI(pki.RenewCLIOptions{
			CLIDir: opts.CLIDir, CAPassphrase: passphrase, LeafExpiry: 90 * oriontls.Day,
		})
		Ω(err).NotTo(HaveOccurred())
		newCert, newKey := readFiles(files)
		Ω(newCert).NotTo(Equal(oldCert))
		Ω(newKey).NotTo(Equal(oldKey), "expected a new private key")
		Ω(result.Renewed.SerialNumber).NotTo(Equal(result.Previous.SerialNumber))
		Ω(result.Renewed.Subject.String()).To(Equal(result.Previous.Subject.String()))
		Ω(result.Renewed.ExtKeyUsage).To(Equal([]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}))
		Ω(result.Renewed.NotAfter).To(BeTemporally("~", time.Now().Add(90*oriontls.Day), time.Minute))
		Ω(result.Capped).To(BeFalse())

		_, err = tls.LoadX509KeyPair(files.CertFile, files.KeyFile)
		Ω(err).NotTo(HaveOccurred(), "expected the new key to belong to the new certificate")
		info, err := os.Stat(files.KeyFile)
		Ω(err).NotTo(HaveOccurred())
		Ω(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))

		// the api verifies clients with client-ca.crt
		pool, _, err := pki.LoadCertPool(pki.APIFiles(opts.APIDir).CAFile)
		Ω(err).NotTo(HaveOccurred())
		_, err = result.Renewed.Verify(x509.VerifyOptions{
			Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		})
		Ω(err).NotTo(HaveOccurred())
	})

	It("keeps the algorithm of the current key, unless another one is asked", func() {
		opts := newPKI(func(o *pki.InitOptions) { o.KeyAlgorithm = oriontls.KeyAlgorithmRSA2048 })
		renewOpts := pki.RenewCLIOptions{CLIDir: opts.CLIDir, CAPassphrase: passphrase, LeafExpiry: oriontls.Year}
		result, err := pki.RenewCLI(renewOpts)
		Ω(err).NotTo(HaveOccurred())
		Ω(result.Renewed.PublicKeyAlgorithm).To(Equal(x509.RSA))
		Ω(result.Renewed.KeyUsage & x509.KeyUsageKeyEncipherment).NotTo(BeZero())

		renewOpts.KeyAlgorithm = oriontls.KeyAlgorithmECDSAP256
		result, err = pki.RenewCLI(renewOpts)
		Ω(err).NotTo(HaveOccurred())
		Ω(result.Renewed.PublicKeyAlgorithm).To(Equal(x509.ECDSA))
	})

	It("renews an expired certificate", func() {
		opts := newPKI(func(o *pki.InitOptions) { o.LeafExpiry = time.Second })
		time.Sleep(time.Second)
		result, err := pki.RenewCLI(pki.RenewCLIOptions{
			CLIDir: opts.CLIDir, CAPassphrase: passphrase, LeafExpiry: oriontls.Year,
		})
		Ω(err).NotTo(HaveOccurred())
		Ω(time.Now().After(result.Previous.NotAfter)).To(BeTrue())
		Ω(result.Renewed.NotAfter.After(time.Now().Add(300 * oriontls.Day))).To(BeTrue())
	})

	It("caps the validity at the expiry of the intermediate, and says so", func() {
		opts := newPKI(func(o *pki.InitOptions) { o.IntermediateExpiry = 10 * oriontls.Day })
		result, err := pki.RenewCLI(pki.RenewCLIOptions{
			CLIDir: opts.CLIDir, CAPassphrase: passphrase, LeafExpiry: oriontls.Year,
		})
		Ω(err).NotTo(HaveOccurred())
		Ω(result.Capped).To(BeTrue())
		Ω(result.Renewed.NotAfter).To(Equal(result.Intermediate.NotAfter))
	})

	It("changes nothing with a wrong passphrase", func() {
		opts := newPKI(nil)
		files := pki.CLIFiles(opts.CLIDir)
		oldCert, oldKey := readFiles(files)
		_, err := pki.RenewCLI(pki.RenewCLIOptions{
			CLIDir: opts.CLIDir, CAPassphrase: []byte("wrong-passphrase"), LeafExpiry: oriontls.Year,
		})
		Ω(err).To(MatchError(oriontls.ErrWrongPassphrase))
		newCert, newKey := readFiles(files)
		Ω(newCert).To(Equal(oldCert))
		Ω(newKey).To(Equal(oldKey))
	})

	It("refuses a client certificate of another CA", func() {
		opts := newPKI(nil)
		other := newPKI(nil)
		files := pki.CLIFiles(opts.CLIDir)
		otherCert, err := os.ReadFile(pki.CLIFiles(other.CLIDir).CertFile)
		Ω(err).NotTo(HaveOccurred())
		Ω(os.WriteFile(files.CertFile, otherCert, 0o600)).To(Succeed())
		_, err = pki.RenewCLI(pki.RenewCLIOptions{
			CLIDir: opts.CLIDir, CAPassphrase: passphrase, LeafExpiry: oriontls.Year,
		})
		Ω(err).To(MatchError(ContainSubstring("is not signed by the intermediate api-clients")))
		cert, _ := readFiles(files)
		Ω(cert).To(Equal(otherCert))
	})

	It("needs the CA directory", func() {
		opts := newPKI(nil)
		Ω(os.RemoveAll(filepath.Join(opts.CLIDir, pki.CLICADir))).To(Succeed())
		_, err := pki.RenewCLI(pki.RenewCLIOptions{
			CLIDir: opts.CLIDir, CAPassphrase: passphrase, LeafExpiry: oriontls.Year,
		})
		Ω(err).To(MatchError(ContainSubstring("only the cli that created the CA")))
	})

	It("needs a client certificate", func() {
		_, err := pki.RenewCLI(pki.RenewCLIOptions{
			CLIDir: GinkgoT().TempDir(), CAPassphrase: passphrase, LeafExpiry: oriontls.Year,
		})
		Ω(err).To(MatchError(ContainSubstring("no client certificate to renew")))
	})
})

var _ = Describe("LoadIntermediate", func() {
	It("refuses a name that isn't one directory", func() {
		_, err := pki.LoadIntermediate(GinkgoT().TempDir(), "../root", nil)
		Ω(err).To(HaveOccurred())
	})
})

var _ = Describe("RenewAPI", func() {
	passphrase := []byte("renew-test-passphrase")
	var opts pki.InitOptions
	BeforeEach(func() {
		dir := GinkgoT().TempDir()
		opts = pki.InitOptions{
			CLIDir:       filepath.Join(dir, "cli"),
			APIDir:       filepath.Join(dir, "api"),
			APISANs:      []string{"orion-api.example.org", "10.0.0.1"},
			CAPassphrase: passphrase,
			Subject:      pki.SubjectOptions{Organisation: "Acme", Country: "NL"},
		}
		_, err := pki.Init(opts)
		Ω(err).NotTo(HaveOccurred())
	})
	renew := func(apiDir string, sans ...string) (*pki.RenewResult, []string, error) {
		return pki.RenewAPI(pki.RenewAPIOptions{
			CLIDir: opts.CLIDir, APIDir: apiDir, APISANs: sans, CAPassphrase: passphrase, LeafExpiry: 90 * oriontls.Day,
		})
	}
	// verify checks the api files in dir like a client of this CA, for name
	verify := func(dir, name string) *x509.Certificate {
		files := pki.APIFiles(dir)
		pair, err := tls.LoadX509KeyPair(files.CertFile, files.KeyFile)
		Ω(err).NotTo(HaveOccurred(), "expected the key to belong to the certificate")
		roots, _, err := pki.LoadCertPool(pki.CLIFiles(opts.CLIDir).CAFile)
		Ω(err).NotTo(HaveOccurred())
		intermediates := x509.NewCertPool()
		for _, der := range pair.Certificate[1:] {
			c, parseErr := x509.ParseCertificate(der)
			Ω(parseErr).NotTo(HaveOccurred())
			intermediates.AddCert(c)
		}
		_, err = pair.Leaf.Verify(x509.VerifyOptions{
			DNSName: name, Roots: roots, Intermediates: intermediates,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
		Ω(err).NotTo(HaveOccurred())
		clientCA, err := os.ReadFile(files.CAFile)
		Ω(err).NotTo(HaveOccurred())
		expected, err := os.ReadFile(filepath.Join(opts.CLIDir, pki.CLICADir, pki.APIClientIntermediate, "ca.crt"))
		Ω(err).NotTo(HaveOccurred())
		Ω(clientCA).To(Equal(expected))
		return pair.Leaf
	}

	It("renews the certificate in the api directory, with its names and the new ones", func() {
		oldKey, err := os.ReadFile(pki.APIFiles(opts.APIDir).KeyFile)
		Ω(err).NotTo(HaveOccurred())
		result, sans, err := renew(opts.APIDir, "orion.example.org")
		Ω(err).NotTo(HaveOccurred())
		Ω(sans).To(ConsistOf(pki.APIServerName, "localhost", "127.0.0.1", "::1", "orion-api.example.org", "10.0.0.1",
			"orion.example.org"))
		Ω(result.Previous).NotTo(BeNil())
		Ω(result.Renewed.SerialNumber).NotTo(Equal(result.Previous.SerialNumber))
		Ω(result.Renewed.Subject.String()).To(Equal(result.Previous.Subject.String()))
		Ω(result.Renewed.NotAfter).To(BeTemporally("~", time.Now().Add(90*oriontls.Day), time.Minute))
		leaf := verify(opts.APIDir, "orion.example.org")
		Ω(leaf.SerialNumber).To(Equal(result.Renewed.SerialNumber))
		verify(opts.APIDir, "orion-api.example.org")
		newKey, err := os.ReadFile(pki.APIFiles(opts.APIDir).KeyFile)
		Ω(err).NotTo(HaveOccurred())
		Ω(newKey).NotTo(Equal(oldKey), "expected a new private key")
	})

	It("creates the api files in an empty directory", func() {
		dir := filepath.Join(GinkgoT().TempDir(), "new-api")
		result, sans, err := renew(dir)
		Ω(err).NotTo(HaveOccurred())
		Ω(result.Previous).To(BeNil())
		Ω(sans).To(ConsistOf(pki.APIServerName, "localhost", "127.0.0.1", "::1"))
		verify(dir, "localhost")
	})

	It("refuses the api files of another CA, and changes nothing", func() {
		otherDir := GinkgoT().TempDir()
		_, err := pki.Init(pki.InitOptions{
			CLIDir: filepath.Join(otherDir, "cli"), APIDir: filepath.Join(otherDir, "api"),
		})
		Ω(err).NotTo(HaveOccurred())
		before, err := os.ReadFile(pki.APIFiles(filepath.Join(otherDir, "api")).CertFile)
		Ω(err).NotTo(HaveOccurred())
		_, _, err = renew(filepath.Join(otherDir, "api"))
		Ω(err).To(MatchError(ContainSubstring("is not signed by the intermediate server of this CA")))
		after, err := os.ReadFile(pki.APIFiles(filepath.Join(otherDir, "api")).CertFile)
		Ω(err).NotTo(HaveOccurred())
		Ω(after).To(Equal(before))
	})

	It("refuses an api directory with the CA directory in it", func() {
		_, _, err := renew(opts.CLIDir)
		Ω(err).To(MatchError(ContainSubstring("contains the CA directory")))
	})

	It("refuses an invalid name", func() {
		_, _, err := renew(opts.APIDir, "not a name")
		Ω(err).To(MatchError(ContainSubstring(`invalid name or address for the api certificate: "not a name"`)))
	})

	It("changes nothing with a wrong passphrase", func() {
		before, err := os.ReadFile(pki.APIFiles(opts.APIDir).CertFile)
		Ω(err).NotTo(HaveOccurred())
		_, err = func() (*pki.RenewResult, error) {
			r, _, renewErr := pki.RenewAPI(pki.RenewAPIOptions{
				CLIDir: opts.CLIDir, APIDir: opts.APIDir, CAPassphrase: []byte("wrong-passphrase"),
				LeafExpiry: oriontls.Year,
			})
			return r, renewErr
		}()
		Ω(err).To(MatchError(oriontls.ErrWrongPassphrase))
		after, err := os.ReadFile(pki.APIFiles(opts.APIDir).CertFile)
		Ω(err).NotTo(HaveOccurred())
		Ω(after).To(Equal(before))
	})
})
