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
	"crypto/ecdh"
	"crypto/hpke"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/pgvillage-tools/orion/pkg/bootstrap"
)

var _ = Describe("The archive", func() {
	var (
		priv  hpke.PrivateKey
		ekm   []byte
		certs endpoints.BootstrapCertificates
		keys  bootstrap.ArchiveKeys
	)

	BeforeEach(func() {
		var err error
		priv, err = bootstrap.GenerateKey()
		Expect(err).NotTo(HaveOccurred())
		ekm = bytes.Repeat([]byte{7}, bootstrap.ExporterLength)
		certs = endpoints.BootstrapCertificates{Root: []byte("root"), ServerIntermediate: []byte("server"),
			APIClientIntermediate: []byte("clients"), APIServer: []byte("api"), CLIClient: []byte("cli")}
		keys = bootstrap.ArchiveKeys{Root: []byte("root key"), ServerIntermediate: []byte("server key"),
			APIClientIntermediate: []byte("clients key"), CLIClient: []byte("cli key")}
	})

	seal := func() endpoints.BootstrapArchive {
		GinkgoHelper()
		archive, err := bootstrap.SealArchive(priv.PublicKey(), ekm, certs, keys)
		Expect(err).NotTo(HaveOccurred())
		return archive
	}

	It("encrypts the keys to the public key of the cli, and leaves the certificates as they are", func() {
		archive := seal()
		Expect(archive.Certificates).To(Equal(certs))
		Expect(archive.SealedKeys).NotTo(ContainSubstring("root key"))
		Expect(bootstrap.OpenArchive(priv, ekm, archive)).To(Equal(keys))
	})

	It("can't be opened with another key pair", func() {
		other, err := bootstrap.GenerateKey()
		Expect(err).NotTo(HaveOccurred())
		_, err = bootstrap.OpenArchive(other, ekm, seal())
		Expect(err).To(MatchError(bootstrap.ErrArchive))
	})

	It("can't be opened for another TLS connection", func() {
		archive := seal()
		_, err := bootstrap.OpenArchive(priv, bytes.Repeat([]byte{8}, bootstrap.ExporterLength), archive)
		Expect(err).To(MatchError(bootstrap.ErrArchive))
	})

	DescribeTable("can't be opened with changed certificates",
		func(change func(*endpoints.BootstrapCertificates)) {
			archive := seal()
			change(&archive.Certificates)
			_, err := bootstrap.OpenArchive(priv, ekm, archive)
			Expect(err).To(MatchError(bootstrap.ErrArchive))
		},
		Entry("another root", func(c *endpoints.BootstrapCertificates) { c.Root = []byte("evil root") }),
		Entry("another server intermediate",
			func(c *endpoints.BootstrapCertificates) { c.ServerIntermediate = []byte("evil") }),
		Entry("another client intermediate",
			func(c *endpoints.BootstrapCertificates) { c.APIClientIntermediate = []byte("evil") }),
		Entry("another api certificate", func(c *endpoints.BootstrapCertificates) { c.APIServer = []byte("evil") }),
		Entry("another cli certificate", func(c *endpoints.BootstrapCertificates) { c.CLIClient = []byte("evil") }),
		Entry("bytes moved between certificates", func(c *endpoints.BootstrapCertificates) {
			c.Root, c.ServerIntermediate = []byte("roots"), []byte("erver")
		}),
	)

	It("can't be opened when the keys were changed", func() {
		archive := seal()
		archive.SealedKeys[0] ^= 1
		_, err := bootstrap.OpenArchive(priv, ekm, archive)
		Expect(err).To(MatchError(bootstrap.ErrArchive))
	})

	It("can't be opened with an invalid encapsulated key", func() {
		archive := seal()
		archive.Enc = archive.Enc[:10]
		_, err := bootstrap.OpenArchive(priv, ekm, archive)
		Expect(err).To(MatchError(bootstrap.ErrArchive))
	})

	It("needs keying material of the right size", func() {
		_, err := bootstrap.SealArchive(priv.PublicKey(), ekm[:10], certs, keys)
		Expect(err).To(MatchError(bootstrap.ErrSize))
		_, err = bootstrap.OpenArchive(priv, ekm[:10], seal())
		Expect(err).To(MatchError(bootstrap.ErrSize))
	})

	It("needs the KEM of the key pair of the cli", func() {
		x25519, err := ecdh.X25519().GenerateKey(nil)
		Expect(err).NotTo(HaveOccurred())
		pub, err := hpke.NewDHKEMPublicKey(x25519.PublicKey())
		Expect(err).NotTo(HaveOccurred())
		_, err = bootstrap.SealArchive(pub, ekm, certs, keys)
		Expect(err).To(MatchError(ContainSubstring("unsupported KEM")))
	})
})
