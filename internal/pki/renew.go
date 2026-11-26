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
	if err = ReplacePair(files.CertFile, files.KeyFile, leaf.Cert.PEM, leaf.PrivateKey.PEM); err != nil {
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

// ReplacePair writes keyPEM and certPEM to keyFile and certFile, each atomically. When the certificate can't be
// written, the previous key is put back, so the files still belong together.
func ReplacePair(certFile, keyFile string, certPEM, keyPEM []byte) error {
	previousKey, readErr := os.ReadFile(keyFile)
	if err := oriontls.WriteFile(keyFile, keyPEM, true); err != nil {
		return fmt.Errorf("failed to write %s: %w", keyFile, err)
	}
	if err := oriontls.WriteFile(certFile, certPEM, false); err != nil {
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

// RenewAPIOptions are the options of RenewAPI
type RenewAPIOptions struct {
	// CLIDir is the directory of the cli, with the CA directory
	CLIDir string
	// APIDir is where the api files are written; a certificate in it is renewed (it must be one of this CA)
	APIDir string
	// APISANs are added to the api certificate, on top of DefaultAPISANs and the names on the current certificate
	APISANs []string
	// CAPassphrase decrypts the private key of the server intermediate
	CAPassphrase []byte
	// LeafExpiry is the validity of the new certificate
	LeafExpiry time.Duration
	// KeyAlgorithm of the new key; empty means the algorithm of the current key (else that of the intermediate)
	KeyAlgorithm oriontls.KeyAlgorithm
}

// RenewAPI writes a new server certificate and private key for the api to the api directory, signed by the server
// intermediate, together with the api client intermediate (client-ca.crt): the same files as Init. The certificate
// keeps the names of the current one in the api directory, when there is one. Copy the files to the api, which loads
// them without a restart.
func RenewAPI(opts RenewAPIOptions) (*RenewResult, []string, error) {
	if opts.CLIDir == "" || opts.APIDir == "" {
		return nil, nil, errors.New("both the cli and the api directory need to be set")
	}
	if err := (InitOptions{CLIDir: opts.CLIDir, APIDir: opts.APIDir}).checkDirs(); err != nil {
		return nil, nil, err
	}
	intermediate, err := LoadIntermediate(opts.CLIDir, ServerIntermediate, opts.CAPassphrase)
	if err != nil {
		return nil, nil, err
	}
	clientCAFile := filepath.Join(opts.CLIDir, CLICADir, APIClientIntermediate, caName+crtExt)
	clientCA, err := os.ReadFile(clientCAFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read the api client intermediate: %w", err)
	}
	files := APIFiles(opts.APIDir)
	previous, err := currentAPICert(files.CertFile, intermediate.Cert.X509())
	if err != nil {
		return nil, nil, err
	}
	sans, err := apiSANs(previous, opts.APISANs)
	if err != nil {
		return nil, nil, err
	}
	algorithm := opts.KeyAlgorithm
	if algorithm == "" {
		algorithm = currentAlgorithm(files.KeyFile, intermediate.PrivateKey.Algorithm)
	}
	leaf, err := IssueLeaf(intermediate, LeafRequest{
		Subject:        intermediate.Cert.Subject.SetCommonName(APIServerName),
		ExtKeyUsage:    oriontls.DefaultServerExtKeyUsages,
		AlternateNames: sans,
		Expiry:         opts.LeafExpiry,
		KeyAlgorithm:   algorithm,
	})
	if err != nil {
		return nil, nil, err
	}
	certPEM := append(append([]byte{}, leaf.Cert.PEM...), intermediate.Cert.PEM...)
	if err = ReplacePair(files.CertFile, files.KeyFile, certPEM, leaf.PrivateKey.PEM); err != nil {
		return nil, nil, err
	}
	if err = oriontls.WriteFile(files.CAFile, clientCA, false); err != nil {
		return nil, nil, fmt.Errorf("failed to write %s: %w", files.CAFile, err)
	}
	renewed := leaf.Cert.X509()
	return &RenewResult{
		Previous:     previous,
		Renewed:      renewed,
		Capped:       renewed.NotAfter.Equal(intermediate.Cert.X509().NotAfter),
		Intermediate: intermediate.Cert.X509(),
	}, sans, nil
}

// currentAPICert returns the api certificate in certFile (nil when there is none), and refuses one that isn't signed
// by the server intermediate: the api directory then holds files of another CA
func currentAPICert(certFile string, intermediate *x509.Certificate) (*x509.Certificate, error) {
	_, certs, err := LoadCertPool(certFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = certs[0].CheckSignatureFrom(intermediate); err != nil {
		return nil, fmt.Errorf("%s is not signed by the intermediate %s of this CA: renew it in an empty directory, "+
			"or with the api directory of this CA: %w", certFile, ServerIntermediate, err)
	}
	return certs[0], nil
}

// apiSANs returns the names for a new api certificate: its CommonName, DefaultAPISANs, the names on current (when
// set) and extra
func apiSANs(current *x509.Certificate, extra []string) ([]string, error) {
	sans := append([]string{APIServerName}, DefaultAPISANs...)
	if current != nil {
		sans = append(sans, current.DNSNames...)
		for _, ip := range current.IPAddresses {
			sans = append(sans, ip.String())
		}
	}
	sans = uniqueValues(append(sans, extra...))
	for _, san := range sans {
		if !oriontls.IsValidServer(san) {
			return nil, fmt.Errorf("invalid name or address for the api certificate: %q", san)
		}
	}
	return sans, nil
}

// SignAPIRequestOptions are the options of SignAPIRequest
type SignAPIRequestOptions struct {
	// CLIDir is the directory of the cli, with the CA directory
	CLIDir string
	// CAPassphrase decrypts the private key of the server intermediate
	CAPassphrase []byte
	// Request is the PEM encoded certificate request of the api
	Request []byte
	// Current is the certificate the api serves with now: its names are kept, and it must be of this CA
	Current *x509.Certificate
	// APISANs are added to the names
	APISANs []string
	// LeafExpiry is the validity of the new certificate
	LeafExpiry time.Duration
}

// SignedAPIRequest is the result of SignAPIRequest
type SignedAPIRequest struct {
	RenewResult
	// Chain is the new certificate followed by the server intermediate, PEM encoded: what the api serves
	Chain []byte
	// SANs are the names on the new certificate
	SANs []string
}

// errUnexpectedPEM is returned for PEM data of another type than expected
var errUnexpectedPEM = errors.New("no PEM encoded certificate request found")

// SignAPIRequest signs the certificate request of the api (the online renewal) with the server intermediate. The
// request must be signed by its own key (the api has that key), and be for the CommonName of the api. The new
// certificate keeps the names of the current one.
func SignAPIRequest(opts SignAPIRequestOptions) (*SignedAPIRequest, error) {
	block, _ := pem.Decode(opts.Request)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, errUnexpectedPEM
	}
	request, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("invalid certificate request: %w", err)
	}
	if err = request.CheckSignature(); err != nil {
		return nil, fmt.Errorf("the certificate request isn't signed by its key: %w", err)
	}
	if request.Subject.CommonName != APIServerName {
		return nil, fmt.Errorf("the certificate request is for %q, not for %s", request.Subject.CommonName,
			APIServerName)
	}
	if opts.Current == nil {
		return nil, errors.New("the current certificate of the api is required")
	}
	intermediate, err := LoadIntermediate(opts.CLIDir, ServerIntermediate, opts.CAPassphrase)
	if err != nil {
		return nil, err
	}
	if err = opts.Current.CheckSignatureFrom(intermediate.Cert.X509()); err != nil {
		return nil, fmt.Errorf("the certificate of the api is not signed by the intermediate %s of this CA: %w",
			ServerIntermediate, err)
	}
	sans, err := apiSANs(opts.Current, opts.APISANs)
	if err != nil {
		return nil, err
	}
	subject := intermediate.Cert.Subject.SetCommonName(APIServerName)
	cert := oriontls.Cert{
		Subject:        &subject,
		Expiry:         opts.LeafExpiry,
		KeyUsage:       oriontls.DefaultKeyUsage,
		ExtKeyUsage:    oriontls.DefaultServerExtKeyUsages,
		AlternateNames: sans,
	}
	if request.PublicKeyAlgorithm == x509.RSA {
		cert.KeyUsage |= x509.KeyUsageKeyEncipherment
	}
	if err = cert.Generate(); err != nil {
		return nil, err
	}
	if err = cert.SignPublicKey(request.PublicKey, intermediate); err != nil {
		return nil, err
	}
	renewed := cert.X509()
	return &SignedAPIRequest{
		RenewResult: RenewResult{
			Previous:     opts.Current,
			Renewed:      renewed,
			Capped:       renewed.NotAfter.Equal(intermediate.Cert.X509().NotAfter),
			Intermediate: intermediate.Cert.X509(),
		},
		Chain: append(append([]byte{}, cert.PEM...), intermediate.Cert.PEM...),
		SANs:  sans,
	}, nil
}
