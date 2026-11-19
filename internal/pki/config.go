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

package pki

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ExpiryWarning is the period before the expiry of a certificate in which a warning is given
const ExpiryWarning = 30 * 24 * time.Hour

// ExpandHome replaces a leading ~ in path with the home directory of the current user
func ExpandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to expand %s: %w", path, err)
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
}

// LoadCertPool reads all PEM encoded certificates in path into a CertPool
func LoadCertPool(path string) (*x509.CertPool, []*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	pool := x509.NewCertPool()
	var certs []*x509.Certificate
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, parseErr := x509.ParseCertificate(block.Bytes)
		if parseErr != nil {
			return nil, nil, fmt.Errorf("failed to parse a certificate in %s: %w", path, parseErr)
		}
		pool.AddCert(cert)
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		return nil, nil, fmt.Errorf("no certificates found in %s", path)
	}
	return pool, certs, nil
}

// ErrFilesNotFound is returned by Check when one of the files doesn't exist
var ErrFilesNotFound = errors.New("certificates not found")

// ExpiringCertificates returns a description of all certificates that expire before now + within
func ExpiringCertificates(certs []*x509.Certificate, now time.Time, within time.Duration) []string {
	var expiring []string
	for _, c := range certs {
		if c.NotAfter.Before(now.Add(within)) {
			expiring = append(expiring, fmt.Sprintf("%s expires at %s", c.Subject.CommonName,
				c.NotAfter.Format(time.RFC3339)))
		}
	}
	return expiring
}
