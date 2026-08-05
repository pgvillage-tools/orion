package tls

import (
	"crypto/x509"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ExtendedKeyUsages", func() {
	var eku = ExtKeyUsages{
		"clientAuth",
		"serverAuth",
	}
	var invalidEku = ExtKeyUsages{
		"ClientAuth",
		"ServerAuth",
	}
	Context("with proper ExtKeyUsages", func() {
		It("Should work as expected", func() {
			eku509, err := eku.AsEKeyUsages()
			Expect(err).Error().NotTo(HaveOccurred())
			Expect(eku509).To(ContainElements(x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth))
		})
	})
	Context("with improper ExtKeyUsages", func() {
		It("Should raise error", func() {
			eku509, err := invalidEku.AsEKeyUsages()
			Expect(err).Error().To(HaveOccurred())
			Expect(eku509).To(BeNil())
		})
	})
})
