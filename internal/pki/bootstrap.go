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
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/pgvillage-tools/orion/pkg/bootstrap"
	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
)

// The archive of the bootstrap: what the api hands over to the cli. See docs/api/bootstrap-design.md, "The archive".

// APIBootstrapArchiveFile holds the archive in the api directory between the hand-over and the confirmation of the
// cli. Its private keys are encrypted to the cli: the api can't read them.
const APIBootstrapArchiveFile = "bootstrap-archive.json"

// ArchiveContents returns the certificates and the (unencrypted) private keys of chain (from Generate) for the
// archive
func ArchiveContents(chain oriontls.Chain) (endpoints.BootstrapCertificates, bootstrap.ArchiveKeys) {
	serverIM := chain.Intermediates[ServerIntermediate]
	clientIM := chain.Intermediates[APIClientIntermediate]
	server := serverIM.Children()[APIServerName]
	client := clientIM.Children()[CLIClientName]
	certs := endpoints.BootstrapCertificates{
		Root:                  chain.Root.Cert.PEM,
		ServerIntermediate:    serverIM.Cert.Cert.PEM,
		APIClientIntermediate: clientIM.Cert.Cert.PEM,
		APIServer:             server.Cert.PEM,
		CLIClient:             client.Cert.PEM,
	}
	keys := bootstrap.ArchiveKeys{
		Root:                  chain.Root.PrivateKey.PEM,
		ServerIntermediate:    serverIM.Cert.PrivateKey.PEM,
		APIClientIntermediate: clientIM.Cert.PrivateKey.PEM,
		CLIClient:             client.PrivateKey.PEM,
	}
	return certs, keys
}

// parseCert parses the PEM certificate of what
func parseCert(what string, data []byte) (*x509.Certificate, error) {
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" || len(strings.TrimSpace(string(rest))) > 0 {
		return nil, fmt.Errorf("the archive has no single certificate for the %s", what)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("invalid certificate for the %s in the archive: %w", what, err)
	}
	return cert, nil
}

// checkKey checks that the unencrypted PKCS#8 PEM key of what belongs to cert
func checkKey(what string, data []byte, cert *x509.Certificate) error {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" {
		return fmt.Errorf("the archive has no private key for the %s", what)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("invalid private key for the %s in the archive: %w", what, err)
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return fmt.Errorf("the private key for the %s in the archive can't sign", what)
	}
	pub, ok := signer.Public().(interface{ Equal(crypto.PublicKey) bool })
	if !ok || !pub.Equal(cert.PublicKey) {
		return fmt.Errorf("the private key for the %s in the archive doesn't belong to its certificate", what)
	}
	return nil
}

// VerifyArchive checks the archive the cli got: the root is a self-signed CA, the intermediates are CAs signed by
// it, the api certificate is a server certificate for orion-api signed by the server intermediate, the cli
// certificate is a client certificate for orion-cli signed by the api client intermediate, and every private key
// belongs to its certificate
func VerifyArchive(certs endpoints.BootstrapCertificates, keys bootstrap.ArchiveKeys, now time.Time) error {
	root, err := parseCert("root", certs.Root)
	if err != nil {
		return err
	}
	if !root.IsCA || root.CheckSignatureFrom(root) != nil {
		return errors.New("the root in the archive isn't a self-signed CA")
	}
	roots := x509.NewCertPool()
	roots.AddCert(root)
	leaves := []struct {
		name, imName, cn string
		imPEM, leafPEM   []byte
		usage            x509.ExtKeyUsage
	}{
		{"api certificate", "server intermediate", APIServerName, certs.ServerIntermediate, certs.APIServer,
			x509.ExtKeyUsageServerAuth},
		{"cli certificate", "api client intermediate", CLIClientName, certs.APIClientIntermediate, certs.CLIClient,
			x509.ExtKeyUsageClientAuth},
	}
	parsed := map[string]*x509.Certificate{"root": root}
	for _, l := range leaves {
		im, imErr := parseCert(l.imName, l.imPEM)
		if imErr != nil {
			return imErr
		}
		if !im.IsCA || im.CheckSignatureFrom(root) != nil {
			return fmt.Errorf("the %s in the archive isn't a CA signed by the root", l.imName)
		}
		leaf, leafErr := parseCert(l.name, l.leafPEM)
		if leafErr != nil {
			return leafErr
		}
		if leaf.Subject.CommonName != l.cn {
			return fmt.Errorf("the %s in the archive is for %q, not %q", l.name, leaf.Subject.CommonName, l.cn)
		}
		intermediates := x509.NewCertPool()
		intermediates.AddCert(im)
		if _, err = leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, CurrentTime: now,
			KeyUsages: []x509.ExtKeyUsage{l.usage}}); err != nil {
			return fmt.Errorf("the %s in the archive isn't valid: %w", l.name, err)
		}
		parsed[l.imName], parsed[l.name] = im, leaf
	}
	for _, k := range []struct {
		name string
		data []byte
	}{
		{"root", keys.Root},
		{"server intermediate", keys.ServerIntermediate},
		{"api client intermediate", keys.APIClientIntermediate},
		{"cli certificate", keys.CLIClient},
	} {
		if err = checkKey(k.name, k.data, parsed[k.name]); err != nil {
			return err
		}
	}
	return nil
}

// CheckNoCLIFiles returns an error when one of the files the bootstrap stores already exists in the cli directory
// dir: the bootstrap doesn't overwrite them
func CheckNoCLIFiles(dir string) error {
	var existing []string
	for _, f := range cliFiles(dir) {
		if _, err := os.Lstat(f); err == nil {
			existing = append(existing, f)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if len(existing) > 0 {
		return fmt.Errorf("refusing to overwrite existing files: %s (move them away to bootstrap again)",
			strings.Join(existing, ", "))
	}
	return nil
}

// encryptKey returns the unencrypted PKCS#8 PEM key data encrypted with passphrase (as Init stores the CA keys)
func encryptKey(data, passphrase []byte) ([]byte, error) {
	pk := oriontls.PrivateKey{PEM: data}
	if err := pk.Decode(); err != nil {
		return nil, err
	}
	pk.PEM, pk.Passphrase = nil, passphrase
	if err := pk.Encode(); err != nil {
		return nil, err
	}
	return pk.PEM, nil
}

// StoreArchive stores the archive (checked with VerifyArchive) in the cli directory dir, like Init does: the root and
// the intermediates below CLICADir, with their private keys encrypted with passphrase, and the client certificate of
// the cli with the root to verify the api. It doesn't overwrite files (see CheckNoCLIFiles); when it fails, it removes
// the files it wrote.
func StoreArchive(dir string, certs endpoints.BootstrapCertificates, keys bootstrap.ArchiveKeys,
	passphrase []byte) (err error) {
	if len(passphrase) == 0 {
		return errors.New("a passphrase is required for the CA keys")
	}
	if err = CheckNoCLIFiles(dir); err != nil {
		return err
	}
	caDir := filepath.Join(dir, CLICADir)
	files := []file{
		{path: filepath.Join(caDir, rootName+crtExt), data: certs.Root},
		{path: filepath.Join(caDir, rootName+keyExt), data: keys.Root, private: true},
		{path: filepath.Join(caDir, ServerIntermediate, caName+crtExt), data: certs.ServerIntermediate},
		{path: filepath.Join(caDir, ServerIntermediate, caName+keyExt), data: keys.ServerIntermediate, private: true},
		{path: filepath.Join(caDir, APIClientIntermediate, caName+crtExt), data: certs.APIClientIntermediate},
		{path: filepath.Join(caDir, APIClientIntermediate, caName+keyExt), data: keys.APIClientIntermediate,
			private: true},
	}
	for i := range files {
		if files[i].private {
			if files[i].data, err = encryptKey(files[i].data, passphrase); err != nil {
				return fmt.Errorf("failed to encrypt %s: %w", files[i].path, err)
			}
		}
	}
	files = append(files,
		file{path: filepath.Join(dir, CLICertFile), data: certs.CLIClient},
		file{path: filepath.Join(dir, CLIKeyFile), data: keys.CLIClient, private: true},
		file{path: filepath.Join(dir, CLICAFile), data: certs.Root},
	)
	defer func() {
		if err != nil {
			for _, f := range files {
				_ = os.Remove(f.path)
			}
		}
	}()
	return writeFiles(files)
}
