package tls

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Intermediate", func() {
	const (
		im     = "intermediate"
		s1     = "server1"
		s1ip   = "1.2.3.4"
		s1fqdn = "server1.example.org"
		s2     = "server2"
		s2ip   = "1.2.3.5"
		s2fqdn = "server2.example.org"
		c1     = "client1"
		c2     = "client2"
	)
	Context("Intermediate", Ordered, func() {
		When("checking for valid server", func() {
			It("Should return true for valid ip and hostnames", func() {
				for _, s := range []string{s1, s1ip, s1fqdn, s2, s2ip, s2fqdn} {
					fmt.Fprintf(GinkgoWriter, "DEBUG - Valid server: %s\n", s)
					Expect(isValidServer(s)).To(BeTrue())
				}
			})
			It("Should return false for anything else", func() {
				for _, s := range []string{
					"",
					"1.2.3.4.5",
					":::",
					".s.example.com",
					"{",
				} {
					fmt.Fprintf(GinkgoWriter, "DEBUG - Invalid server: %s\n", s)
					Expect(isValidServer(s)).To(BeFalse())
				}
			})
		})
		When("When working with a valid intermediate", Ordered, func() {
			var (
				chain = Chain{
					Intermediates: Intermediates{
						im: Intermediate{
							Servers: Servers{
								s1: []string{s1ip, s1fqdn},
								s2: []string{s2ip, s2fqdn},
							},
							Clients: []string{c1, c2},
						},
					},
				}
			)
			It("Should properly initialize", func() {
				//InitializeCa
				Expect(chain.InitializeCA()).Error().NotTo(HaveOccurred())
				//InitializeIntermediates
				Expect(chain.Intermediates.Initialize(chain.Root)).Error().NotTo(
					HaveOccurred())
				Expect(chain.Intermediates).To(HaveKey(im))
				myIntermediate := chain.Intermediates[im]
				Expect(string(myIntermediate.Cert.Cert.PEM)).To(
					HavePrefix("-----BEGIN CERTIFICATE-----"))
				Expect(string(myIntermediate.Cert.PrivateKey.PEM)).To(
					HavePrefix("-----BEGIN RSA PRIVATE KEY-----"))
				Expect(myIntermediate.children).To(HaveKey(s1))

				server1 := myIntermediate.children[s1]
				Expect(server1.Cert.cert).NotTo(BeNil())
				Expect(server1.Cert.PEM).NotTo(BeEmpty())
				Expect(string(server1.Cert.PEM)).To(
					HavePrefix("-----BEGIN CERTIFICATE-----"))
				Expect(server1.PrivateKey.key).NotTo(BeNil())
				Expect(server1.PrivateKey.PEM).NotTo(BeEmpty())
				Expect(string(server1.PrivateKey.PEM)).To(
					HavePrefix("-----BEGIN RSA PRIVATE KEY-----"))
			})
			It("requires an initialized root to sign the intermediate", func() {
				//InitializeIntermediates
				im, err := chain.Intermediates.Initialize(Pair{})
				Expect(err).Error().To(MatchError(SatisfyAll(ContainSubstring(
					"signer not properly initialized"))))
				Expect(im).To(BeNil())
			})
		})
		When("When working with a valid intermediate", Ordered, func() {
			var (
				invalidClientChain = Chain{
					Intermediates: Intermediates{
						im: Intermediate{
							Servers: Servers{
								s1: []string{s1ip, s1fqdn},
							},
							Clients: []string{c1, ""},
						},
					},
				}
				invalidServerChain = Chain{
					Intermediates: Intermediates{
						im: Intermediate{
							Servers: Servers{
								s1: []string{s1ip, ""},
							},
							Clients: []string{c1, c2},
						},
					},
				}
			)
			It("Should fail on initialization", func() {
				for _, test := range []struct {
					chain     Chain
					errPrefix string
				}{
					{chain: invalidClientChain, errPrefix: "client is not valid"},
					{chain: invalidServerChain, errPrefix: "invalid server or address"},
				} {
					chain := test.chain
					//InitializeCa
					Expect(chain.InitializeCA()).Error().NotTo(HaveOccurred())
					//InitializeIntermediates
					_, err := chain.Intermediates.Initialize(chain.Root)
					Expect(err).To(HaveOccurred())
					Expect(err).Error().To(MatchError(HavePrefix(
						test.errPrefix)))
				}
			})
		})
	})
})
