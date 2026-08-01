package tls

import (
	"crypto/x509"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/exp/constraints"
)

func bitCompare[T constraints.Integer](a T, b T) bool {
	return (a&b == b)
}

var _ = Describe("KeyUsages", func() {
	var ku = KeyUsages{
		"dataEncipherment", //x509.KeyUsageDataEncipherment
		"certSign",         //x509.KeyUsageCertSign
	}
	var invalidKu = KeyUsages{
		"ClientAuth",
		"ServerAuth",
	}

	Context("with proper KeyUsages", func() {
		It("Should work as expected", func() {
			ku509, err := ku.AsKeyUsage()
			Expect(err).Error().NotTo(HaveOccurred())
			for _, tku := range []x509.KeyUsage{
				x509.KeyUsageDataEncipherment,
				x509.KeyUsageCertSign,
			} {
				Expect(bitCompare(ku509, tku)).To(BeTrue())
			}
		})
	})
	Context("with improper KeyUsages", func() {
		It("Should raise error", func() {
			ku509, err := invalidKu.AsKeyUsage()
			Expect(err).Error().To(HaveOccurred())
			Expect(ku509).To(BeZero())
		})
	})
})
