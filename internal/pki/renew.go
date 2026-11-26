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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
)

// LoadIntermediate loads the certificate and the private key of the intermediate name from the CA directory in the
// cli directory cliDir. An encrypted key is decrypted with passphrase.
func LoadIntermediate(cliDir, name string, passphrase []byte) (oriontls.Pair, error) {
	if err := oriontls.ValidateStoreName(name); err != nil {
		return oriontls.Pair{}, err
	}
	dir := filepath.Join(cliDir, CLICADir, name)
	pair := oriontls.Pair{
		Cert:       oriontls.Cert{Path: filepath.Join(dir, caName+crtExt)},
		PrivateKey: oriontls.PrivateKey{Path: filepath.Join(dir, caName+keyExt), Passphrase: passphrase},
	}
	if err := pair.Load(); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return oriontls.Pair{}, fmt.Errorf("the intermediate %s is not in %s: only the cli that created the CA "+
				"(pki init or bootstrap) has it: %w", name, dir, err)
		}
		return oriontls.Pair{}, fmt.Errorf("failed to load the intermediate %s: %w", name, err)
	}
	if !pair.Cert.IsCa {
		return oriontls.Pair{}, fmt.Errorf("%s is not a CA certificate", pair.Cert.Path)
	}
	return pair, nil
}

// LeafRequest describes a server or client certificate that an intermediate signs
type LeafRequest struct {
	Subject oriontls.Subject
	// ExtKeyUsage is oriontls.DefaultServerExtKeyUsages or oriontls.DefaultClientExtKeyUsages
	ExtKeyUsage []x509.ExtKeyUsage
	// AlternateNames are the SANs (for a server certificate)
	AlternateNames []string
	Expiry         time.Duration
	KeyAlgorithm   oriontls.KeyAlgorithm
}

// IssueLeaf creates a private key and a certificate for req, signed by intermediate. The certificate never outlives
// the intermediate. Nothing is written: the PEMs are in the returned pair.
func IssueLeaf(intermediate oriontls.Pair, req LeafRequest) (oriontls.Pair, error) {
	if intermediate.Cert.X509() == nil || intermediate.PrivateKey.Signer() == nil {
		return oriontls.Pair{}, errors.New("the intermediate is not loaded")
	}
	subject := req.Subject
	pair := oriontls.Pair{
		Cert: oriontls.Cert{
			Subject:        &subject,
			Expiry:         req.Expiry,
			KeyUsage:       oriontls.DefaultKeyUsage,
			ExtKeyUsage:    req.ExtKeyUsage,
			AlternateNames: req.AlternateNames,
		},
		PrivateKey: oriontls.PrivateKey{Algorithm: req.KeyAlgorithm},
	}
	if err := pair.Process(intermediate); err != nil {
		return oriontls.Pair{}, fmt.Errorf("failed to create the certificate %s: %w", subject.CommonName, err)
	}
	return pair, nil
}

// RenewCLIOptions are the options of RenewCLI
type RenewCLIOptions struct {
	// CLIDir is the directory of the cli: its client certificate, and the CA directory with the intermediates
	CLIDir string
	// CAPassphrase decrypts the private key of the api client intermediate
	CAPassphrase []byte
	// LeafExpiry is the validity of the new certificate
	LeafExpiry time.Duration
	// KeyAlgorithm of the new key; empty means the algorithm of the current key
	KeyAlgorithm oriontls.KeyAlgorithm
}

// RenewResult describes a renewed certificate
type RenewResult struct {
	Previous *x509.Certificate
	Renewed  *x509.Certificate
	// Capped is true when the certificate expires with its intermediate, before the requested validity
	Capped bool
	// Intermediate is the certificate that signed the new one
	Intermediate *x509.Certificate
}

// RenewCLI replaces the client certificate of the cli (and its private key) with a new one, with the same subject,
// signed by the api client intermediate. The current certificate may have expired, but it must be one of that
// intermediate: the api only accepts client certificates signed by it.
func RenewCLI(opts RenewCLIOptions) (*RenewResult, error) {
	files := CLIFiles(opts.CLIDir)
	current := oriontls.Cert{Path: files.CertFile}
	if err := current.Load(); err != nil {
		return nil, fmt.Errorf("no client certificate to renew in %s (create one with `orion-cli pki init`): %w",
			opts.CLIDir, err)
	}
	intermediate, err := LoadIntermediate(opts.CLIDir, APIClientIntermediate, opts.CAPassphrase)
	if err != nil {
		return nil, err
	}
	if err = current.X509().CheckSignatureFrom(intermediate.Cert.X509()); err != nil {
		return nil, fmt.Errorf("%s is not signed by the intermediate %s in %s: %w", files.CertFile,
			APIClientIntermediate, filepath.Join(opts.CLIDir, CLICADir), err)
	}
	algorithm := opts.KeyAlgorithm
	if algorithm == "" {
		algorithm = currentAlgorithm(files.KeyFile, intermediate.PrivateKey.Algorithm)
	}
	leaf, err := IssueLeaf(intermediate, LeafRequest{
		Subject:      oriontls.SubjectFromPkixName(current.X509().Subject),
		ExtKeyUsage:  oriontls.DefaultClientExtKeyUsages,
		Expiry:       opts.LeafExpiry,
		KeyAlgorithm: algorithm,
	})
	if err != nil {
		return nil, err
	}
	if err = replacePair(files.CertFile, files.KeyFile, leaf); err != nil {
		return nil, err
	}
	renewed := leaf.Cert.X509()
	return &RenewResult{
		Previous:     current.X509(),
		Renewed:      renewed,
		Capped:       renewed.NotAfter.Equal(intermediate.Cert.X509().NotAfter),
		Intermediate: intermediate.Cert.X509(),
	}, nil
}

// currentAlgorithm returns the algorithm of the (unencrypted) private key in keyFile, or fallback when it can't be
// read
func currentAlgorithm(keyFile string, fallback oriontls.KeyAlgorithm) oriontls.KeyAlgorithm {
	key := oriontls.PrivateKey{Path: keyFile}
	if err := key.Load(); err != nil {
		return fallback
	}
	return key.Algorithm
}

// replacePair writes the private key and the certificate of pair to keyFile and certFile, each atomically. When the
// certificate can't be written, the previous key is put back, so the files still belong together.
func replacePair(certFile, keyFile string, pair oriontls.Pair) error {
	previousKey, readErr := os.ReadFile(keyFile)
	if err := oriontls.WriteFile(keyFile, pair.PrivateKey.PEM, true); err != nil {
		return fmt.Errorf("failed to write %s: %w", keyFile, err)
	}
	if err := oriontls.WriteFile(certFile, pair.Cert.PEM, false); err != nil {
		if readErr == nil {
			if restoreErr := oriontls.WriteFile(keyFile, previousKey, true); restoreErr != nil {
				return fmt.Errorf("failed to write %s (%w), and to put back the previous %s: %w", certFile, err,
					keyFile, restoreErr)
			}
		}
		return fmt.Errorf("failed to write %s: %w", certFile, err)
	}
	return nil
}
