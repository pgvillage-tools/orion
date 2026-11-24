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

package cmd

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/pgvillage-tools/orion/internal/pki"
)

// renewHint tells what to do about a certificate that expires
const renewHint = "renew it (see Certificates in the docs)"

// certWarnings holds the certificates (by fingerprint) the cli warned about in this command, so it warns about each
// one once, also when it connects more than once
var certWarnings = struct {
	sync.Mutex
	warned map[[sha256.Size]byte]bool
}{}

// resetCertWarnings forgets the warnings of an earlier command (in a test, several commands run in one process)
func resetCertWarnings() {
	certWarnings.Lock()
	defer certWarnings.Unlock()
	certWarnings.warned = nil
}

// warnExpiring warns on stderr about the certificates of owner (the cli or the api) that expire within
// pki.ExpiryWarning. It changes nothing else: not the output, and not the exit code.
func warnExpiring(owner string, certs []*x509.Certificate, now time.Time) {
	certWarnings.Lock()
	defer certWarnings.Unlock()
	for _, c := range certs {
		if !c.NotAfter.Before(now.Add(pki.ExpiryWarning)) {
			continue
		}
		fingerprint := sha256.Sum256(c.Raw)
		if certWarnings.warned[fingerprint] {
			continue
		}
		if certWarnings.warned == nil {
			certWarnings.warned = map[[sha256.Size]byte]bool{}
		}
		certWarnings.warned[fingerprint] = true
		stderr("warning: certificate %s of the %s expires at %s: %s", c.Subject.CommonName, owner,
			c.NotAfter.Format(time.RFC3339), renewHint)
	}
}

// checkValid returns an error for the first of the certificates of owner that expired or isn't valid yet
func checkValid(owner string, certs []*x509.Certificate, now time.Time) error {
	for _, c := range certs {
		if err := validityError(owner, c, now); err != nil {
			return err
		}
	}
	return nil
}

// validityError explains why certificate c of owner isn't valid at now, or returns nil when it is
func validityError(owner string, c *x509.Certificate, now time.Time) error {
	switch {
	case now.After(c.NotAfter):
		return fmt.Errorf("certificate %s of the %s expired at %s: %s", c.Subject.CommonName, owner,
			c.NotAfter.Format(time.RFC3339), renewHint)
	case now.Before(c.NotBefore):
		return fmt.Errorf("certificate %s of the %s is valid from %s only: check the clock", c.Subject.CommonName,
			owner, c.NotBefore.Format(time.RFC3339))
	}
	return nil
}

// warnExpiringAPI is the VerifyConnection of the cli: after the usual verification, it warns about the certificates
// the api sent (its certificate and its chain) that expire soon
func warnExpiringAPI(cs tls.ConnectionState) error {
	warnExpiring("api", cs.PeerCertificates, time.Now())
	return nil
}

// explainCertificateError adds an explanation to err when the api sent a certificate that expired or isn't valid
// yet: the TLS error itself only says the time is outside the validity
func explainCertificateError(err error) error {
	var invalid x509.CertificateInvalidError
	if !errors.As(err, &invalid) || invalid.Reason != x509.Expired || invalid.Cert == nil {
		return err
	}
	explanation := validityError("api", invalid.Cert, time.Now())
	if explanation == nil {
		return err
	}
	return fmt.Errorf("%w; %w", err, explanation)
}
