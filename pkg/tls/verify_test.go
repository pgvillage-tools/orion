package tls

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("A chain with separate server and client intermediates", Ordered, func() {
	const (
		serverIM  = "server"
		apiIM     = "api-clients"
		etcdIM    = "etcd-clients"
		apiServer = "orion-api"
		cli       = "orion-cli"
	)
	var (
		chain  Chain
		tmpDir string
	)
	BeforeAll(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "verifyTest")
		Expect(err).NotTo(HaveOccurred())
		chain = Chain{
			Store: tmpDir,
			Intermediates: Intermediates{
				serverIM: Intermediate{Servers: Servers{apiServer: {"localhost", "127.0.0.1"}}},
				apiIM:    Intermediate{Clients: []string{cli}},
				etcdIM:   Intermediate{Clients: []string{"etcd-user"}},
			},
		}
		Expect(chain.InitializeCA()).To(Succeed())
		Expect(chain.InitializeIntermediates()).To(Succeed())
	})
	AfterAll(func() {
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	pool := func(pairs ...Pair) *x509.CertPool {
		p := x509.NewCertPool()
		for _, pair := range pairs {
			p.AddCert(pair.Cert.X509())
		}
		return p
	}
	leaf := func(im, name string) *x509.Certificate {
		return chain.Intermediates[im].Children()[name].Cert.X509()
	}

	It("uses the expected validity and constraints", func() {
		root := chain.Root.Cert.X509()
		Expect(root.IsCA).To(BeTrue())
		Expect(root.MaxPathLen).To(Equal(rootMaxPathLen))
		Expect(root.NotAfter.Sub(root.NotBefore)).To(BeNumerically("~", DefaultRootExpiry+ClockSkew, time.Minute))
		Expect(root.NotBefore).To(BeTemporally("~", time.Now().Add(-ClockSkew), time.Minute))

		im := chain.Intermediates[serverIM].Cert.Cert.X509()
		Expect(im.IsCA).To(BeTrue())
		Expect(im.MaxPathLen).To(Equal(0))
		Expect(im.MaxPathLenZero).To(BeTrue())
		Expect(im.NotAfter.Sub(im.NotBefore)).To(BeNumerically("~", DefaultIntermediateExpiry+ClockSkew, time.Minute))

		server := leaf(serverIM, apiServer)
		Expect(server.IsCA).To(BeFalse())
		Expect(server.NotAfter.Sub(server.NotBefore)).To(BeNumerically("~", DefaultExpiry+ClockSkew, time.Minute))
		Expect(server.KeyUsage).To(Equal(x509.KeyUsageDigitalSignature))
	})

	It("stores the files in the store", func() {
		for _, f := range []string{
			"root.crt", "root.key",
			filepath.Join(serverIM, "ca.crt"), filepath.Join(serverIM, "ca.key"),
			filepath.Join(serverIM, apiServer+".crt"), filepath.Join(serverIM, apiServer+".key"),
			filepath.Join(apiIM, cli+".crt"), filepath.Join(apiIM, cli+".key"),
		} {
			Expect(filepath.Join(tmpDir, f)).To(BeAnExistingFile())
		}
	})

	It("verifies a server certificate against the root", func() {
		_, err := leaf(serverIM, apiServer).Verify(x509.VerifyOptions{
			DNSName:       "localhost",
			Roots:         pool(chain.Root),
			Intermediates: pool(chain.Intermediates[serverIM].Cert),
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("doesn't accept a client certificate as a server certificate", func() {
		_, err := leaf(apiIM, cli).Verify(x509.VerifyOptions{
			Roots:         pool(chain.Root),
			Intermediates: pool(chain.Intermediates[apiIM].Cert),
		})
		Expect(err).To(HaveOccurred())
	})

	It("accepts a client certificate of its own client intermediate", func() {
		_, err := leaf(apiIM, cli).Verify(x509.VerifyOptions{
			Roots:     pool(chain.Intermediates[apiIM].Cert),
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("doesn't accept a client certificate of another client intermediate", func() {
		_, err := leaf(etcdIM, "etcd-user").Verify(x509.VerifyOptions{
			Roots:     pool(chain.Intermediates[apiIM].Cert),
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		})
		Expect(err).To(HaveOccurred())
	})

	It("caps the expiry of a certificate at the expiry of its signer", func() {
		short := Chain{
			Root: Pair{Cert: Cert{Expiry: 2 * Day}},
			Intermediates: Intermediates{
				serverIM: Intermediate{Servers: Servers{apiServer: nil}},
			},
		}
		Expect(short.InitializeCA()).To(Succeed())
		Expect(short.InitializeIntermediates()).To(Succeed())
		root := short.Root.Cert.X509()
		Expect(short.Intermediates[serverIM].Cert.Cert.X509().NotAfter).To(Equal(root.NotAfter))
		Expect(short.Intermediates[serverIM].Children()[apiServer].Cert.X509().NotAfter).To(Equal(root.NotAfter))
	})

	It("keeps a short expiry that was set explicitly", func() {
		var c Cert
		c.Expiry = time.Hour
		c.SetDefaults(DefaultSubject, DefaultExpiry, DefaultKeyUsage, nil)
		Expect(c.Expiry).To(Equal(time.Hour))
	})

	It("can use RSA keys", func() {
		rsaChain := Chain{
			KeyAlgorithm:  KeyAlgorithmRSA2048,
			Intermediates: Intermediates{serverIM: Intermediate{Servers: Servers{apiServer: nil}}},
		}
		Expect(rsaChain.InitializeCA()).To(Succeed())
		Expect(rsaChain.InitializeIntermediates()).To(Succeed())
		server := rsaChain.Intermediates[serverIM].Children()[apiServer]
		Expect(server.PrivateKey.Algorithm).To(Equal(KeyAlgorithmRSA2048))
		Expect(server.Cert.X509().KeyUsage & x509.KeyUsageKeyEncipherment).NotTo(BeZero())
	})
})
