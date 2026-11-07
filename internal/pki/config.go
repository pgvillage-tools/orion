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
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
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

// TLSFiles holds the files for a TLS server or client: a certificate (optionally followed by its chain), its
// private key, and the CA to verify the other side
type TLSFiles struct {
	CertFile string
	KeyFile  string
	CAFile   string
}

// APIFiles returns the files of the api in dir
func APIFiles(dir string) TLSFiles {
	return TLSFiles{
		CertFile: filepath.Join(dir, APIServerCertFile),
		KeyFile:  filepath.Join(dir, APIServerKeyFile),
		CAFile:   filepath.Join(dir, APIClientCAFile),
	}
}

// CLIFiles returns the files of the cli in dir
func CLIFiles(dir string) TLSFiles {
	return TLSFiles{
		CertFile: filepath.Join(dir, CLICertFile),
		KeyFile:  filepath.Join(dir, CLIKeyFile),
		CAFile:   filepath.Join(dir, CLICAFile),
	}
}

// Expand expands a leading ~ in all files
func (f TLSFiles) Expand() (TLSFiles, error) {
	var err error
	for _, p := range []*string{&f.CertFile, &f.KeyFile, &f.CAFile} {
		if *p, err = ExpandHome(*p); err != nil {
			return f, err
		}
	}
	return f, nil
}

func (f TLSFiles) load() (tls.Certificate, *x509.CertPool, error) {
	cert, err := tls.LoadX509KeyPair(f.CertFile, f.KeyFile)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("failed to load %s and %s: %w", f.CertFile, f.KeyFile, err)
	}
	pool, _, err := LoadCertPool(f.CAFile)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("failed to load CA: %w", err)
	}
	return cert, pool, nil
}

// ServerConfig returns a TLS config for a server that requires a client certificate signed by the CA
func (f TLSFiles) ServerConfig() (*tls.Config, error) {
	cert, pool, err := f.load()
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}, nil
}

// ClientConfig returns a TLS config for a client that logs in with its certificate, and verifies the server with
// the CA
func (f TLSFiles) ClientConfig() (*tls.Config, error) {
	cert, pool, err := f.load()
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
	}, nil
}

// Exist returns true when all files exist
func (f TLSFiles) Exist() bool {
	for _, p := range []string{f.CertFile, f.KeyFile, f.CAFile} {
		if _, err := os.Stat(p); err != nil {
			return false
		}
	}
	return true
}

// ExpiringCertificates returns a description of all certificates in the files (the certificate, its chain and the
// CA) that expire before now + within
func (f TLSFiles) ExpiringCertificates(now time.Time, within time.Duration) ([]string, error) {
	var expiring []string
	for _, p := range []string{f.CertFile, f.CAFile} {
		_, certs, err := LoadCertPool(p)
		if err != nil {
			return nil, err
		}
		for _, c := range certs {
			if c.NotAfter.Before(now.Add(within)) {
				expiring = append(expiring, fmt.Sprintf("%s (in %s) expires at %s", c.Subject.CommonName, p,
					c.NotAfter.Format(time.RFC3339)))
			}
		}
	}
	return expiring, nil
}
