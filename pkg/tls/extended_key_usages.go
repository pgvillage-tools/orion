package tls

import (
	"crypto/x509"
	"fmt"
)

// ExtKeyUsages can be used to store KeyUsage references as strings
type ExtKeyUsages []string

var stringToEKU = map[string]x509.ExtKeyUsage{
	"clientAuth": x509.ExtKeyUsageClientAuth,
	"serverAuth": x509.ExtKeyUsageServerAuth,

	"any":                            x509.ExtKeyUsageAny,
	"codeSigning":                    x509.ExtKeyUsageCodeSigning,
	"emailProtection":                x509.ExtKeyUsageEmailProtection,
	"ipSecEndSystem":                 x509.ExtKeyUsageIPSECEndSystem,
	"ipSecTunnel":                    x509.ExtKeyUsageIPSECTunnel,
	"ipSecUser":                      x509.ExtKeyUsageIPSECUser,
	"timestamping":                   x509.ExtKeyUsageTimeStamping,
	"ocpsSigning":                    x509.ExtKeyUsageOCSPSigning,
	"microsoftServerGatedCrypto":     x509.ExtKeyUsageMicrosoftServerGatedCrypto,
	"metscapeServerGatedCrypto":      x509.ExtKeyUsageNetscapeServerGatedCrypto,
	"microsoftCommercialCodeSigning": x509.ExtKeyUsageMicrosoftCommercialCodeSigning,
	"microsoftKernelCodeSigning":     x509.ExtKeyUsageMicrosoftKernelCodeSigning,
}

// AsEKeyUsages converts a ExtKeyUsages into a list of x509.ExtKeyUsage's
func (eks ExtKeyUsages) AsEKeyUsages() ([]x509.ExtKeyUsage, error) {
	var result []x509.ExtKeyUsage
	for _, key := range eks {
		eku, exists := stringToEKU[key]
		if !exists {
			return nil, fmt.Errorf("invalid Extended Key Usage: %s", key)
		}
		result = append(result, eku)
	}
	return result, nil
}
