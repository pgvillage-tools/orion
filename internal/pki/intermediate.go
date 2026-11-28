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
	"slices"
	"time"

	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
)

// PreviousIntermediateFile holds the certificate of an intermediate before its last renewal, next to ca.crt. The
// certificates it signed can still be renewed.
const PreviousIntermediateFile = "previous.crt"

// Intermediates returns the names of the intermediates that the cli can renew
func Intermediates() []string {
	return []string{ServerIntermediate, APIClientIntermediate}
}

// intermediateDir returns the directory with the files of the intermediate name
func intermediateDir(cliDir, name string) string {
	return filepath.Join(cliDir, CLICADir, name)
}

// signedByIntermediate returns nil when cert is signed by intermediate, or by the intermediate it replaced (in
// PreviousIntermediateFile): after a renewal of an intermediate, its certificates are renewed too
func signedByIntermediate(cert *x509.Certificate, cliDir, name string, intermediate *x509.Certificate) error {
	err := cert.CheckSignatureFrom(intermediate)
	if err == nil {
		return nil
	}
	_, previous, loadErr := LoadCertPool(filepath.Join(intermediateDir(cliDir, name), PreviousIntermediateFile))
	if loadErr != nil {
		return err
	}
	if cert.CheckSignatureFrom(previous[0]) == nil {
		return nil
	}
	return err
}

// loadRoot loads the certificate and the private key of the root from the CA directory in cliDir
func loadRoot(cliDir string, passphrase []byte) (oriontls.Pair, error) {
	dir := filepath.Join(cliDir, CLICADir)
	root := oriontls.Pair{
		Cert:       oriontls.Cert{Path: filepath.Join(dir, rootName+crtExt)},
		PrivateKey: oriontls.PrivateKey{Path: filepath.Join(dir, rootName+keyExt), Passphrase: passphrase},
	}
	if err := root.Load(); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return oriontls.Pair{}, fmt.Errorf("the root is not in %s: only the cli that created the CA (pki init or "+
				"bootstrap) has it: %w", dir, err)
		}
		return oriontls.Pair{}, fmt.Errorf("failed to load the root: %w", err)
	}
	if !root.Cert.IsCa {
		return oriontls.Pair{}, fmt.Errorf("%s is not a CA certificate", root.Cert.Path)
	}
	return root, nil
}

// RenewIntermediateOptions are the options of RenewIntermediate
type RenewIntermediateOptions struct {
	// CLIDir is the directory of the cli, with the CA directory
	CLIDir string
	// Name is the name of the intermediate (see Intermediates)
	Name string
	// CAPassphrase decrypts the private keys of the root and the intermediate, and encrypts the new one
	CAPassphrase []byte
	// Expiry is the validity of the new intermediate
	Expiry time.Duration
	// KeyAlgorithm of the new key; empty means the algorithm of the current key
	KeyAlgorithm oriontls.KeyAlgorithm
}

// RenewIntermediate replaces the intermediate name (its certificate and private key) in the CA directory with a new
// one, with the same subject, signed by the root. The current certificate is kept as PreviousIntermediateFile. The
// certificates that the intermediate signed still need to be renewed: they don't chain to the new key.
func RenewIntermediate(opts RenewIntermediateOptions) (*RenewResult, error) {
	if !slices.Contains(Intermediates(), opts.Name) {
		return nil, fmt.Errorf("unknown intermediate %q (expected one of %v)", opts.Name, Intermediates())
	}
	root, err := loadRoot(opts.CLIDir, opts.CAPassphrase)
	if err != nil {
		return nil, err
	}
	if now := time.Now(); now.After(root.Cert.X509().NotAfter) {
		return nil, fmt.Errorf("the root expired at %s: it can't sign a new intermediate",
			root.Cert.X509().NotAfter.Format(time.RFC3339))
	}
	current, err := LoadIntermediate(opts.CLIDir, opts.Name, opts.CAPassphrase)
	if err != nil {
		return nil, err
	}
	if err = current.Cert.X509().CheckSignatureFrom(root.Cert.X509()); err != nil {
		return nil, fmt.Errorf("the intermediate %s is not signed by the root in %s: %w", opts.Name,
			filepath.Join(opts.CLIDir, CLICADir), err)
	}
	algorithm := opts.KeyAlgorithm
	if algorithm == "" {
		algorithm = current.PrivateKey.Algorithm
	}
	subject := oriontls.SubjectFromPkixName(current.Cert.X509().Subject)
	renewed := oriontls.Pair{
		Cert: oriontls.Cert{
			Subject:  &subject,
			Expiry:   opts.Expiry,
			KeyUsage: oriontls.DefaultCAKeyUsage,
			IsCa:     true,
		},
		PrivateKey: oriontls.PrivateKey{Algorithm: algorithm, Passphrase: opts.CAPassphrase},
	}
	if err = renewed.Process(root); err != nil {
		return nil, fmt.Errorf("failed to create the intermediate %s: %w", opts.Name, err)
	}
	dir := intermediateDir(opts.CLIDir, opts.Name)
	if err = oriontls.WriteFile(filepath.Join(dir, PreviousIntermediateFile), current.Cert.PEM, false); err != nil {
		return nil, fmt.Errorf("failed to keep the current intermediate: %w", err)
	}
	if err = ReplacePair(current.Cert.Path, current.PrivateKey.Path, renewed.Cert.PEM,
		renewed.PrivateKey.PEM); err != nil {
		return nil, err
	}
	cert := renewed.Cert.X509()
	return &RenewResult{
		Previous:     current.Cert.X509(),
		Renewed:      cert,
		Capped:       cert.NotAfter.Equal(root.Cert.X509().NotAfter),
		Intermediate: root.Cert.X509(),
	}, nil
}

// WriteAPIClientCA writes the api client intermediate of the cli in cliDir to the api directory apiDir, as
// APIClientCAFile: the api only accepts client certificates signed by it
func WriteAPIClientCA(cliDir, apiDir string) (string, error) {
	if err := (InitOptions{CLIDir: cliDir, APIDir: apiDir}).CheckDirs(); err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(intermediateDir(cliDir, APIClientIntermediate), caName+crtExt))
	if err != nil {
		return "", fmt.Errorf("failed to read the api client intermediate: %w", err)
	}
	path := APIFiles(apiDir).CAFile
	if err = oriontls.WriteFile(path, data, false); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", path, err)
	}
	return path, nil
}
