package tls

import (
	"crypto/x509"
	"fmt"
)

// KeyUsages is a list of KeyUsage objects
type KeyUsages []string

var stringToKU = map[string]x509.KeyUsage{
	"dataEncipherment": x509.KeyUsageDataEncipherment,
	"digitalSignature": x509.KeyUsageDigitalSignature,
	"certSign":         x509.KeyUsageCertSign,
	"keyEncipherment":  x509.KeyUsageKeyEncipherment,
	"crlSign":          x509.KeyUsageCRLSign,
}

// AsKeyUsage converts a internal KeyUsages object to a x509.KeyUsage value
func (eks KeyUsages) AsKeyUsage() (x509.KeyUsage, error) {
	var result x509.KeyUsage
	for _, key := range eks {
		ku, exists := stringToKU[key]
		if !exists {
			return result, fmt.Errorf("invalid Extended Key Usage: %s", key)
		}
		result |= ku
	}
	return result, nil
}
