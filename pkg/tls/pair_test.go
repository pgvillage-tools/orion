package tls

import (
	"os"
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Pair", Ordered, func() {
	var root Pair
	root.Cert.SetDefaults(DefaultSubject, DefaultExpiry, DefaultKeyUsage, DefaultExtendedKeyUsages)
	root.Cert.IsCa = true
	var (
		tmpDir     string
		firstPair  Pair
		secondPair Pair
		pairs      Pairs
	)
	BeforeAll(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "pairTest")
		Expect(err).Error().NotTo(HaveOccurred())
		Expect(root.Generate()).Error().NotTo(HaveOccurred())
		Expect(root.Encode()).Error().NotTo(HaveOccurred())
		Expect(root.Sign(root)).Error().NotTo(HaveOccurred())
		firstPair = Pair{
			Cert: Cert{
				Path: path.Join(tmpDir, "first", "cert.pem"),
			},
			PrivateKey: PrivateKey{Path: path.Join(tmpDir, "first", "key.pem")},
		}
		secondPair = Pair{
			Cert:       Cert{Path: path.Join(tmpDir, "second", "cert.pem")},
			PrivateKey: PrivateKey{Path: path.Join(tmpDir, "second", "key.pem")},
		}
		firstPair.Cert.SetDefaults(
			root.Cert.Subject.SetCommonName("first"),
			root.Cert.Expiry,
			root.Cert.KeyUsage,
			root.Cert.ExtKeyUsage,
		)
		secondPair.Cert.SetDefaults(
			root.Cert.Subject.SetCommonName("second"),
			root.Cert.Expiry,
			root.Cert.KeyUsage,
			root.Cert.ExtKeyUsage,
		)
		pairs = Pairs{
			"first":  firstPair,
			"second": secondPair,
		}
	})
	AfterAll(func() {
		Expect(os.RemoveAll(tmpDir)).Error().NotTo(HaveOccurred())
	})
	Context("When managing a pair", Ordered, func() {
		It("should generate properly", func() {
			pairs, err := pairs.Generate()
			Expect(err).Error().NotTo(HaveOccurred())
			Expect(pairs).To(HaveLen(2))
			for _, pair := range pairs {
				Expect(pair.Cert.cert).NotTo(BeNil())
				Expect(pair.PrivateKey.key).NotTo(BeNil())
			}
		})
		It("should encode properly", func() {
			pairs, err := pairs.Encode()
			Expect(err).Error().NotTo(HaveOccurred())
			Expect(pairs).To(HaveLen(2))
			for _, pair := range pairs {
				Expect(string(pair.PrivateKey.PEM)).To(HavePrefix("-----BEGIN RSA PRIVATE KEY-----"))
			}
		})
		It("should sign properly", func() {
			pairs, err := pairs.Sign(root)
			Expect(err).Error().NotTo(HaveOccurred())
			Expect(pairs).To(HaveLen(2))
			for _, pair := range pairs {
				Expect(string(pair.Cert.PEM)).To(HavePrefix("-----BEGIN CERTIFICATE-----"))
			}
		})
		It("should save properly", func() {
			for _, pair := range pairs {
				Expect(pair.PrivateKey.dirty).To(BeTrue())
				Expect(pair.Cert.dirty).To(BeTrue())
			}
			pairs, err := pairs.Save()
			Expect(err).Error().NotTo(HaveOccurred())
			Expect(pairs).To(HaveLen(2))
			for _, pair := range pairs {
				Expect(pair.Cert.dirty).To(BeFalse())
				Expect(pair.PrivateKey.dirty).To(BeFalse())
				Expect(pair.Cert.Path).To(BeAnExistingFile())
				Expect(pair.PrivateKey.Path).To(BeAnExistingFile())
			}
		})
	})
})
