package tls

import (
	"os"
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Pair", Ordered, func() {
	var (
		root       Pair
		tmpDir     string
		firstPair  Pair
		secondPair Pair
		pairs      Pairs
	)
	BeforeAll(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "pairTest")
		Expect(err).Error().NotTo(HaveOccurred())
		root.Cert.SetDefaults(DefaultSubject, DefaultRootExpiry, DefaultCAKeyUsage, nil)
		root.Cert.IsCa = true
		root.Cert.MaxPathLen = rootMaxPathLen
		Expect(root.SelfSign()).To(Succeed())
		firstPair = Pair{
			Cert:       Cert{Path: path.Join(tmpDir, "first", "cert.pem")},
			PrivateKey: PrivateKey{Path: path.Join(tmpDir, "first", "key.pem")},
		}
		secondPair = Pair{
			Cert:       Cert{Path: path.Join(tmpDir, "second", "cert.pem")},
			PrivateKey: PrivateKey{Path: path.Join(tmpDir, "second", "key.pem")},
		}
		firstPair.Cert.SetDefaults(
			root.Cert.Subject.SetCommonName("first"), DefaultExpiry, DefaultKeyUsage, DefaultClientExtKeyUsages)
		secondPair.Cert.SetDefaults(
			root.Cert.Subject.SetCommonName("second"), DefaultExpiry, DefaultKeyUsage, DefaultClientExtKeyUsages)
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
				Expect(string(pair.PrivateKey.PEM)).To(HavePrefix("-----BEGIN PRIVATE KEY-----"))
			}
		})
		It("should sign properly", func() {
			pairs, err := pairs.Sign(root)
			Expect(err).Error().NotTo(HaveOccurred())
			Expect(pairs).To(HaveLen(2))
			for _, pair := range pairs {
				Expect(string(pair.Cert.PEM)).To(HavePrefix("-----BEGIN CERTIFICATE-----"))
				Expect(pair.Cert.X509().CheckSignatureFrom(root.Cert.X509())).To(Succeed())
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
				keyInfo, err := os.Stat(pair.PrivateKey.Path)
				Expect(err).NotTo(HaveOccurred())
				Expect(keyInfo.Mode().Perm()).To(Equal(privateFileMode))
			}
		})
		It("should load properly", func() {
			loaded := Pair{
				Cert:       Cert{Path: pairs["first"].Cert.Path},
				PrivateKey: PrivateKey{Path: pairs["first"].PrivateKey.Path},
			}
			Expect(loaded.Load()).To(Succeed())
			Expect(loaded.Cert.X509().Equal(pairs["first"].Cert.X509())).To(BeTrue())
			Expect(loaded.PrivateKey.key).NotTo(BeNil())
		})
		It("should return errors when the private key can't be saved", func() {
			pair := Pair{
				PrivateKey: PrivateKey{Path: path.Join(tmpDir, "first", "key.pem", "impossible")},
			}
			pair.Cert.SetDefaults(DefaultSubject, DefaultExpiry, DefaultKeyUsage, DefaultClientExtKeyUsages)
			Expect(pair.Process(root)).NotTo(Succeed())
		})
		It("should refuse to sign with an unsigned signer", func() {
			var unsigned Pair
			unsigned.Cert.SetDefaults(DefaultSubject, DefaultExpiry, DefaultCAKeyUsage, nil)
			unsigned.Cert.IsCa = true
			Expect(unsigned.Generate()).To(Succeed())
			pair := Pair{}
			pair.Cert.SetDefaults(DefaultSubject, DefaultExpiry, DefaultKeyUsage, DefaultClientExtKeyUsages)
			Expect(pair.Process(unsigned)).To(MatchError(ContainSubstring("needs to be signed")))
		})
	})
})
