package tls

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Chain", func() {
	const (
		intermediate = "intermediate"
		server1      = "s1"
	)
	Context("When working with a chain", Ordered, func() {
		var (
			chain = Chain{
				Intermediates: Intermediates{
					intermediate: Intermediate{
						Servers: Servers{server1: []string{"1.2.3.4"}},
					},
				},
			}
		)
		It("Should properly initialize a CA", func() {
			//InitializeCA
			Expect(chain.InitializeCA()).Error().NotTo(HaveOccurred())
			Expect(chain.Root.Cert.cert).NotTo(BeNil())
			Expect(chain.Root.Cert.PEM).NotTo(BeEmpty())
			Expect(string(chain.Root.Cert.PEM)).To(
				HavePrefix("-----BEGIN CERTIFICATE-----"))
			Expect(chain.Root.PrivateKey.key).NotTo(BeNil())
			Expect(chain.Root.PrivateKey.PEM).NotTo(BeEmpty())
			Expect(string(chain.Root.PrivateKey.PEM)).To(
				HavePrefix("-----BEGIN RSA PRIVATE KEY-----"))
		})
		It("Should properly initialize intermediates", func() {
			//InitializeIntermediates
			Expect(chain.InitializeIntermediates()).Error().NotTo(
				HaveOccurred())
			Expect(chain.Intermediates).To(HaveKey(intermediate))

			myIntermediate := chain.Intermediates[intermediate]
			Expect(string(myIntermediate.Cert.Cert.PEM)).To(
				HavePrefix("-----BEGIN CERTIFICATE-----"))
			Expect(string(myIntermediate.Cert.PrivateKey.PEM)).To(
				HavePrefix("-----BEGIN RSA PRIVATE KEY-----"))
			Expect(myIntermediate.children).To(HaveKey(server1))

			server1 := myIntermediate.children[server1]
			Expect(server1.Cert.cert).NotTo(BeNil())
			Expect(server1.Cert.PEM).NotTo(BeEmpty())
			Expect(string(server1.Cert.PEM)).To(
				HavePrefix("-----BEGIN CERTIFICATE-----"))
			Expect(server1.PrivateKey.key).NotTo(BeNil())
			Expect(server1.PrivateKey.PEM).NotTo(BeEmpty())
			Expect(string(server1.PrivateKey.PEM)).To(
				HavePrefix("-----BEGIN RSA PRIVATE KEY-----"))
		})
		It("Should properly generate structure", func() {
			//Structure
			structure := chain.Structure()
			Expect(structure.Certs).To(HaveKey(intermediate))

			myIntermediate := structure.Certs[intermediate]
			Expect(myIntermediate).To(HaveKey(server1))
			s1 := myIntermediate[server1]
			Expect(string(s1)).To(
				HavePrefix("-----BEGIN CERTIFICATE-----"))
			i1Chain := myIntermediate["chain"]
			Expect(string(i1Chain)).To(
				HavePrefix("-----BEGIN CERTIFICATE-----"))
			Expect(
				strings.Count(string(i1Chain),
					"-----BEGIN CERTIFICATE-----",
				)).To(
				Equal(2))
			Expect(string(i1Chain)).NotTo(ContainSubstring("\n\n"))
		})
	})
})
