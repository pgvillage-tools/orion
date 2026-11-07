// Copyright 2026 PgVillage
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied
// See the License for the specific language governing permissions and
// limitations under the License.

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
func (kus KeyUsages) AsKeyUsage() (x509.KeyUsage, error) {
	var result x509.KeyUsage
	for _, key := range kus {
		ku, exists := stringToKU[key]
		if !exists {
			return result, fmt.Errorf("invalid Key Usage: %s", key)
		}
		result |= ku
	}
	return result, nil
}
