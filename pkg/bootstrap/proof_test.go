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

package bootstrap_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/pkg/bootstrap"
)

var (
	secret   = []byte("a secret of at least twenty characters")
	apiNonce = bytes.Repeat([]byte{1}, bootstrap.NonceSize)
	cliNonce = bytes.Repeat([]byte{2}, bootstrap.NonceSize)
	ekm      = bytes.Repeat([]byte{3}, bootstrap.ExporterLength)
)

// selfSigned returns a self-signed server certificate for localhost
func selfSigned() tls.Certificate {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	template := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{"localhost"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	Expect(err).NotTo(HaveOccurred())
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// handshake connects a client and a server over a pipe with maxVersion, and returns their connection states
func handshake(maxVersion uint16) (client, server tls.ConnectionState) {
	c, s := net.Pipe()
	DeferCleanup(c.Close)
	DeferCleanup(s.Close)
	srv := tls.Server(s, &tls.Config{Certificates: []tls.Certificate{selfSigned()}, MaxVersion: maxVersion})
	cl := tls.Client(c, &tls.Config{InsecureSkipVerify: true, MaxVersion: maxVersion}) //nolint:gosec // a test
	done := make(chan error, 1)
	go func() { done <- srv.Handshake() }()
	Expect(cl.Handshake()).To(Succeed())
	Expect(<-done).To(Succeed())
	return cl.ConnectionState(), srv.ConnectionState()
}

var _ = Describe("Proofs", func() {
	It("are an HMAC-SHA256 that depends on every input", func() {
		p, err := bootstrap.CLIProof(secret, apiNonce, cliNonce, ekm)
		Expect(err).NotTo(HaveOccurred())
		Expect(p).To(HaveLen(bootstrap.ProofSize))
		again, err := bootstrap.CLIProof(secret, apiNonce, cliNonce, ekm)
		Expect(err).NotTo(HaveOccurred())
		Expect(bootstrap.Equal(p, again)).To(BeTrue(), "the proof is deterministic")

		other := bytes.Repeat([]byte{9}, bootstrap.NonceSize)
		for name, args := range map[string][4][]byte{
			"secret":          {[]byte("another secret of twenty characters"), apiNonce, cliNonce, ekm},
			"api nonce":       {secret, other, cliNonce, ekm},
			"cli nonce":       {secret, apiNonce, other, ekm},
			"keying material": {secret, apiNonce, cliNonce, other},
			"nonces swapped":  {secret, cliNonce, apiNonce, ekm},
		} {
			q, err := bootstrap.CLIProof(args[0], args[1], args[2], args[3])
			Expect(err).NotTo(HaveOccurred())
			Expect(bootstrap.Equal(p, q)).To(BeFalse(), "the proof doesn't depend on the %s", name)
		}
	})

	It("differ between the cli and the api, so one can't be replayed as the other", func() {
		cli, err := bootstrap.CLIProof(secret, apiNonce, cliNonce, ekm)
		Expect(err).NotTo(HaveOccurred())
		api, err := bootstrap.APIProof(secret, apiNonce, cliNonce, ekm)
		Expect(err).NotTo(HaveOccurred())
		Expect(bootstrap.Equal(cli, api)).To(BeFalse())
	})

	DescribeTable("refuse inputs of the wrong size",
		func(a, c, k []byte) {
			_, err := bootstrap.CLIProof(secret, a, c, k)
			Expect(err).To(MatchError(bootstrap.ErrSize))
			_, err = bootstrap.APIProof(secret, a, c, k)
			Expect(err).To(MatchError(bootstrap.ErrSize))
		},
		Entry("a short api nonce", apiNonce[1:], cliNonce, ekm),
		Entry("a long cli nonce", apiNonce, append(bytes.Clone(cliNonce), 0), ekm),
		Entry("no keying material", apiNonce, cliNonce, nil),
	)
})

var _ = Describe("ExportKeyingMaterial", func() {
	It("returns the same keying material on both sides of a TLS 1.3 session", func() {
		client, server := handshake(tls.VersionTLS13)
		c, err := bootstrap.ExportKeyingMaterial(&client)
		Expect(err).NotTo(HaveOccurred())
		s, err := bootstrap.ExportKeyingMaterial(&server)
		Expect(err).NotTo(HaveOccurred())
		Expect(c).To(HaveLen(bootstrap.ExporterLength))
		Expect(c).To(Equal(s))

		// another session has other keying material: a man in the middle can't forward a proof
		other, _ := handshake(tls.VersionTLS13)
		o, err := bootstrap.ExportKeyingMaterial(&other)
		Expect(err).NotTo(HaveOccurred())
		Expect(o).NotTo(Equal(c))
	})

	It("requires TLS 1.3", func() {
		client, _ := handshake(tls.VersionTLS12)
		_, err := bootstrap.ExportKeyingMaterial(&client)
		Expect(err).To(MatchError(ContainSubstring("TLS 1.3")))
		_, err = bootstrap.ExportKeyingMaterial(nil)
		Expect(err).To(HaveOccurred(), "no TLS")
	})
})
