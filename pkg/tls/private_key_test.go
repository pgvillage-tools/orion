package tls

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"os"
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("PrivateKey", func() {
	Context("When generating private keys", Ordered, func() {
		pKey := PrivateKey{}
		It("Should generate an ECDSA P-256 key by default", func() {
			Expect(pKey.key).To(BeNil())
			err := pKey.Generate()
			Expect(err).Error().NotTo(HaveOccurred())
			Expect(pKey.key).To(BeAssignableToTypeOf(&ecdsa.PrivateKey{}))
			Expect(pKey.Algorithm).To(Equal(KeyAlgorithmECDSAP256))
		})
		It("Should not generate once generation has been done before", func() {
			Expect(pKey.key).NotTo(BeNil())
			preGenerateKey := pKey.key
			err := pKey.Generate()
			Expect(err).Error().NotTo(HaveOccurred())
			Expect(pKey.key).To(BeIdenticalTo(preGenerateKey))
		})
		It("Should encode successfully as PKCS#8", func() {
			Expect(pKey.PEM).To(BeNil())
			err := pKey.Encode()
			Expect(err).Error().NotTo(HaveOccurred())
			Expect(pKey.PEM).NotTo(BeNil())
			Expect(string(pKey.PEM)).To(HavePrefix("-----BEGIN PRIVATE KEY-----"))
			Expect(string(pKey.PEM)).To(HaveSuffix("-----END PRIVATE KEY-----\n"))
		})
		It("Should save and load successfully", func() {
			var pem []byte
			Expect(pKey.dirty).To(BeTrue())
			tmpDir, err := os.MkdirTemp("", "privateKeyTest")
			defer func() {
				Expect(os.RemoveAll(tmpDir)).Error().NotTo(HaveOccurred())
			}()
			Expect(err).Error().NotTo(HaveOccurred())
			pKeyPath := path.Join(tmpDir, "private_key")
			pKey.Path = pKeyPath
			err = pKey.Save()
			Expect(err).Error().NotTo(HaveOccurred())

			pem, err = os.ReadFile(pKeyPath)
			Expect(err).Error().NotTo(HaveOccurred())
			Expect(pem).To(Equal(pKey.PEM))
			info, err := os.Stat(pKeyPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(privateFileMode))

			loaded := PrivateKey{Path: pKeyPath}
			Expect(loaded.Load()).To(Succeed())
			Expect(loaded.key).To(Equal(pKey.key))
		})
	})
	Context("When using other algorithms", func() {
		It("Should generate RSA keys", func() {
			key := PrivateKey{Algorithm: KeyAlgorithmRSA2048}
			Expect(key.Generate()).To(Succeed())
			rsaKey, ok := key.key.(*rsa.PrivateKey)
			Expect(ok).To(BeTrue())
			Expect(rsaKey.N.BitLen()).To(Equal(rsa2048Bits))
			Expect(KeyAlgorithmRSA2048.IsRSA()).To(BeTrue())
			Expect(KeyAlgorithmECDSAP256.IsRSA()).To(BeFalse())
		})
		It("Should refuse unknown algorithms", func() {
			key := PrivateKey{Algorithm: "dsa-1024"}
			Expect(key.Generate()).To(MatchError(ContainSubstring("unsupported key algorithm")))
		})
		It("Should refuse to decode garbage", func() {
			key := PrivateKey{PEM: []byte("garbage")}
			Expect(key.Decode()).NotTo(Succeed())
		})
	})
	Context("When requesting public key from private key", Ordered, func() {
		It("Should succeed when properly initialized", func() {
			var key PrivateKey
			Expect(key.Generate()).Error().NotTo(HaveOccurred())
			pub, err := key.PublicKey()
			Expect(err).Error().NotTo(HaveOccurred())
			Expect(pub).NotTo(BeNil())
		})
		It("Should raise error when not properly initialized", func() {
			var key PrivateKey
			_, err := key.PublicKey()
			Expect(err).Error().To(HaveOccurred())
			Expect(key.Encode()).NotTo(Succeed())
		})
	})
})
