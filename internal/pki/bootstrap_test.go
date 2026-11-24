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
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/pgvillage-tools/orion/pkg/bootstrap"
	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
)

var _ = Describe("The bootstrap archive", func() {
	var (
		certs endpoints.BootstrapCertificates
		keys  bootstrap.ArchiveKeys
	)

	// parse returns the certificate in data
	parse := func(data []byte) *x509.Certificate {
		GinkgoHelper()
		block, _ := pem.Decode(data)
		Expect(block).NotTo(BeNil())
		cert, err := x509.ParseCertificate(block.Bytes)
		Expect(err).NotTo(HaveOccurred())
		return cert
	}

	BeforeEach(func() {
		chain, err := pki.Generate(pki.InitOptions{APISANs: []string{"orion-api.example.org", "10.0.0.1"}})
		Expect(err).NotTo(HaveOccurred())
		certs, keys = pki.ArchiveContents(*chain)
	})

	It("is generated in memory, with unencrypted keys and the extra names on the api certificate", func() {
		Expect(pki.VerifyArchive(certs, keys, time.Now())).To(Succeed())
		api := parse(certs.APIServer)
		Expect(api.DNSNames).To(ContainElements("localhost", "orion-api.example.org"))
		Expect(api.IPAddresses).To(ContainElement(Satisfy(func(ip net.IP) bool { return ip.Equal(net.ParseIP("10.0.0.1")) })))
		for _, key := range [][]byte{keys.Root, keys.ServerIntermediate, keys.APIClientIntermediate, keys.CLIClient} {
			Expect(string(key)).To(HavePrefix("-----BEGIN PRIVATE KEY-----"))
		}
	})

	It("refuses invalid extra names", func() {
		_, err := pki.Generate(pki.InitOptions{APISANs: []string{"*.example.org"}})
		Expect(err).To(MatchError(ContainSubstring("invalid name")))
	})

	DescribeTable("VerifyArchive refuses",
		func(change func(*endpoints.BootstrapCertificates, *bootstrap.ArchiveKeys), want string) {
			change(&certs, &keys)
			Expect(pki.VerifyArchive(certs, keys, time.Now())).To(MatchError(ContainSubstring(want)))
		},
		Entry("no root", func(c *endpoints.BootstrapCertificates, _ *bootstrap.ArchiveKeys) { c.Root = nil },
			"no single certificate for the root"),
		Entry("two certificates for the root", func(c *endpoints.BootstrapCertificates, _ *bootstrap.ArchiveKeys) {
			c.Root = append(append([]byte{}, c.Root...), c.Root...)
		}, "no single certificate for the root"),
		Entry("an intermediate as the root", func(c *endpoints.BootstrapCertificates, _ *bootstrap.ArchiveKeys) {
			c.Root = c.ServerIntermediate
		}, "isn't a self-signed CA"),
		Entry("a leaf as the intermediate", func(c *endpoints.BootstrapCertificates, _ *bootstrap.ArchiveKeys) {
			c.ServerIntermediate = c.APIServer
		}, "isn't a CA signed by the root"),
		Entry("swapped intermediates", func(c *endpoints.BootstrapCertificates, _ *bootstrap.ArchiveKeys) {
			c.ServerIntermediate, c.APIClientIntermediate = c.APIClientIntermediate, c.ServerIntermediate
		}, "isn't valid"),
		Entry("the api certificate as the cli certificate",
			func(c *endpoints.BootstrapCertificates, _ *bootstrap.ArchiveKeys) { c.CLIClient = c.APIServer },
			`is for "orion-api"`),
		Entry("a key of another certificate", func(_ *endpoints.BootstrapCertificates, k *bootstrap.ArchiveKeys) {
			k.Root = k.CLIClient
		}, "doesn't belong to its certificate"),
		Entry("no key", func(_ *endpoints.BootstrapCertificates, k *bootstrap.ArchiveKeys) { k.CLIClient = nil },
			"no private key for the cli certificate"),
		Entry("an invalid key", func(_ *endpoints.BootstrapCertificates, k *bootstrap.ArchiveKeys) {
			k.ServerIntermediate = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("x")})
		}, "invalid private key for the server intermediate"),
		Entry("an invalid certificate", func(c *endpoints.BootstrapCertificates, _ *bootstrap.ArchiveKeys) {
			c.CLIClient = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("x")})
		}, "invalid certificate for the cli certificate"),
	)

	It("VerifyArchive refuses expired certificates", func() {
		Expect(pki.VerifyArchive(certs, keys, time.Now().Add(20*365*24*time.Hour))).
			To(MatchError(ContainSubstring("isn't valid")))
	})

	Context("StoreArchive", func() {
		var dir string

		BeforeEach(func() {
			dir = filepath.Join(GinkgoT().TempDir(), "cli")
		})

		It("stores the files like Init, with the CA keys encrypted with the passphrase", func() {
			passphrase := []byte("the CA passphrase")
			Expect(pki.StoreArchive(dir, certs, keys, passphrase)).To(Succeed())

			// the cli can connect with them
			_, err := pki.CLIFiles(dir).ClientConfig()
			Expect(err).NotTo(HaveOccurred())
			Expect(os.ReadFile(filepath.Join(dir, pki.CLICAFile))).To(Equal(certs.Root))

			for _, name := range []string{"root", filepath.Join(pki.ServerIntermediate, "ca"),
				filepath.Join(pki.APIClientIntermediate, "ca")} {
				keyFile := filepath.Join(dir, pki.CLICADir, name+".key")
				data, readErr := os.ReadFile(keyFile)
				Expect(readErr).NotTo(HaveOccurred())
				Expect(string(data)).To(HavePrefix("-----BEGIN ENCRYPTED PRIVATE KEY-----"), keyFile)
				info, statErr := os.Stat(keyFile)
				Expect(statErr).NotTo(HaveOccurred())
				Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)), keyFile)

				pair := oriontls.Pair{
					Cert:       oriontls.Cert{Path: filepath.Join(dir, pki.CLICADir, name+".crt")},
					PrivateKey: oriontls.PrivateKey{Path: keyFile, Passphrase: passphrase},
				}
				Expect(pair.Load()).To(Succeed(), "the CA key decrypts with the passphrase")
			}
		})

		It("doesn't overwrite existing files", func() {
			Expect(os.MkdirAll(dir, 0o700)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(dir, pki.CLIKeyFile), []byte("mine"), 0o600)).To(Succeed())
			Expect(pki.StoreArchive(dir, certs, keys, []byte("passphrase"))).
				To(MatchError(ContainSubstring("refusing to overwrite")))
			Expect(os.ReadFile(filepath.Join(dir, pki.CLIKeyFile))).To(Equal([]byte("mine")))
			Expect(filepath.Join(dir, pki.CLICADir)).NotTo(BeADirectory())
		})

		It("needs a passphrase", func() {
			Expect(pki.StoreArchive(dir, certs, keys, nil)).To(MatchError(ContainSubstring("passphrase")))
		})

		It("removes what it wrote when it fails", func() {
			keys.APIClientIntermediate = []byte("not a key")
			Expect(pki.StoreArchive(dir, certs, keys, []byte("passphrase"))).To(HaveOccurred())
			Expect(pki.CheckNoCLIFiles(dir)).To(Succeed())
		})
	})
})
