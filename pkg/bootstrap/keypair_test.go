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
	"crypto/hpke"
	"encoding/pem"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/pkg/bootstrap"
)

var passphrase = []byte("a passphrase")

// reencode decodes the PEM in data, applies change to the block, and encodes it again
func reencode(data []byte, change func(*pem.Block)) []byte {
	block, _ := pem.Decode(data)
	Expect(block).NotTo(BeNil())
	change(block)
	return pem.EncodeToMemory(block)
}

var _ = Describe("The key pair", func() {
	var priv hpke.PrivateKey

	BeforeEach(func() {
		var err error
		priv, err = bootstrap.GenerateKey()
		Expect(err).NotTo(HaveOccurred())
	})

	It("is ML-KEM-768 with X25519 (X-Wing)", func() {
		Expect(priv.KEM().ID()).To(Equal(hpke.MLKEM768X25519().ID()))
	})

	It("encrypts the private key with the passphrase", func() {
		data, err := bootstrap.EncodePrivateKey(priv, passphrase)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("BEGIN ORION BOOTSTRAP ENCRYPTED PRIVATE KEY"))
		Expect(string(data)).To(ContainSubstring("KDF: argon2id"))
		raw, err := priv.Bytes()
		Expect(err).NotTo(HaveOccurred())
		block, _ := pem.Decode(data)
		Expect(string(block.Bytes)).NotTo(ContainSubstring(string(raw)), "the private key is in the file as it is")

		decoded, err := bootstrap.DecodePrivateKey(data, passphrase)
		Expect(err).NotTo(HaveOccurred())
		Expect(decoded.Bytes()).To(Equal(raw))

		_, err = bootstrap.DecodePrivateKey(data, []byte("another passphrase"))
		Expect(err).To(MatchError(bootstrap.ErrWrongPassphrase))
	})

	It("can decrypt what is encrypted to the public key", func() {
		pub, err := bootstrap.DecodePublicKey(bootstrap.EncodePublicKey(priv.PublicKey()))
		Expect(err).NotTo(HaveOccurred())
		sealed, err := hpke.Seal(pub, hpke.HKDFSHA256(), hpke.AES256GCM(), []byte("info"), []byte("the archive"))
		Expect(err).NotTo(HaveOccurred())
		data, err := bootstrap.EncodePrivateKey(priv, passphrase)
		Expect(err).NotTo(HaveOccurred())
		decoded, err := bootstrap.DecodePrivateKey(data, passphrase)
		Expect(err).NotTo(HaveOccurred())
		plain, err := hpke.Open(decoded, hpke.HKDFSHA256(), hpke.AES256GCM(), []byte("info"), sealed)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(plain)).To(Equal("the archive"))
	})

	It("requires a passphrase", func() {
		_, err := bootstrap.EncodePrivateKey(priv, nil)
		Expect(err).To(HaveOccurred())
	})

	DescribeTable("refuses an invalid private key file",
		func(change func(*pem.Block), want string) {
			data, err := bootstrap.EncodePrivateKey(priv, passphrase)
			Expect(err).NotTo(HaveOccurred())
			_, err = bootstrap.DecodePrivateKey(reencode(data, change), passphrase)
			Expect(err).To(MatchError(ContainSubstring(want)))
		},
		Entry("another type", func(b *pem.Block) { b.Type = "PRIVATE KEY" }, "no ORION BOOTSTRAP"),
		Entry("another KEM", func(b *pem.Block) { b.Headers["KEM"] = "0x0020" }, "unsupported KEM"),
		Entry("another KDF", func(b *pem.Block) { b.Headers["KDF"] = "scrypt" }, "unsupported key derivation"),
		Entry("too much memory", func(b *pem.Block) { b.Headers["Argon2-Memory"] = "4194304" }, "Argon2-Memory"),
		Entry("no passes", func(b *pem.Block) { b.Headers["Argon2-Time"] = "0" }, "Argon2-Time"),
		Entry("no threads", func(b *pem.Block) { delete(b.Headers, "Argon2-Threads") }, "Argon2-Threads"),
		Entry("a short salt", func(b *pem.Block) { b.Headers["Salt"] = "c2FsdA==" }, "invalid salt"),
		Entry("an invalid nonce", func(b *pem.Block) { b.Headers["Nonce"] = "!" }, "invalid nonce"),
		Entry("a short nonce", func(b *pem.Block) { b.Headers["Nonce"] = "bm9uY2U=" }, "invalid nonce"),
		Entry("a changed key", func(b *pem.Block) { b.Bytes[0] ^= 1 }, "wrong passphrase"),
	)

	It("refuses an invalid public key file", func() {
		_, err := bootstrap.DecodePublicKey([]byte("not a key"))
		Expect(err).To(HaveOccurred())
		data := reencode(bootstrap.EncodePublicKey(priv.PublicKey()), func(b *pem.Block) { b.Bytes = b.Bytes[1:] })
		_, err = bootstrap.DecodePublicKey(data)
		Expect(err).To(HaveOccurred())
	})

	It("is stored in a directory, with the private key only readable by the owner", func() {
		dir := filepath.Join(GinkgoT().TempDir(), "cli")
		Expect(bootstrap.KeyPairExists(dir)).To(BeFalse())
		_, err := bootstrap.ReadPublicKey(dir)
		Expect(err).To(MatchError(fs.ErrNotExist))

		Expect(bootstrap.WriteKeyPair(dir, priv, passphrase)).To(Succeed())
		Expect(bootstrap.KeyPairExists(dir)).To(BeTrue())
		info, err := os.Stat(filepath.Join(dir, bootstrap.PrivateKeyFile))
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))

		pub, err := bootstrap.ReadPublicKey(dir)
		Expect(err).NotTo(HaveOccurred())
		Expect(pub.Bytes()).To(Equal(priv.PublicKey().Bytes()))
		read, err := bootstrap.ReadPrivateKey(dir, passphrase)
		Expect(err).NotTo(HaveOccurred())
		Expect(read.PublicKey().Bytes()).To(Equal(pub.Bytes()))
		_, err = bootstrap.ReadPrivateKey(filepath.Join(dir, "missing"), passphrase)
		Expect(err).To(MatchError(fs.ErrNotExist))

		Expect(os.Remove(filepath.Join(dir, bootstrap.PublicKeyFile))).To(Succeed())
		_, err = bootstrap.KeyPairExists(dir)
		Expect(err).To(MatchError(ContainSubstring("only one of")))
	})

	It("fails to store without a passphrase", func() {
		Expect(bootstrap.WriteKeyPair(GinkgoT().TempDir(), priv, nil)).NotTo(Succeed())
	})

	It("has a PEM header with its KEM", func() {
		Expect(strings.Contains(string(bootstrap.EncodePublicKey(priv.PublicKey())), "KEM: 0x647A")).To(BeTrue())
	})
})
