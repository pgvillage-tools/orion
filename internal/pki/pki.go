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

// Package pki defines the PKI of Orion: which intermediates and certificates there are, and where their files are
// stored. See docs/api/bootstrap-design.md.
package pki

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
)

const (
	// ServerIntermediate is the name of the intermediate that signs all server certificates
	ServerIntermediate = "server"
	// APIClientIntermediate is the name of the intermediate that signs the client certificates for the api
	APIClientIntermediate = "api-clients"
	// APIServerName is the CommonName of the server certificate of the api
	APIServerName = "orion-api"
	// CLIClientName is the CommonName of the client certificate of the cli
	CLIClientName = "orion-cli"

	// APIServerCertFile holds the server certificate of the api, followed by the server intermediate
	APIServerCertFile = "server.crt"
	// APIServerKeyFile holds the private key of the server certificate of the api
	APIServerKeyFile = "server.key"
	// APIClientCAFile holds the api client intermediate: the api only accepts client certificates signed by it
	APIClientCAFile = "client-ca.crt"

	// CLICertFile holds the client certificate of the cli
	CLICertFile = "tls.crt"
	// CLIKeyFile holds the private key of the client certificate of the cli
	CLIKeyFile = "tls.key"
	// CLICAFile holds the root CA, which the cli uses to verify the api
	CLICAFile = "ca.crt"
	// CLICADir is the directory (below the cli directory) with the certificates and private keys of the root and
	// the intermediates
	CLICADir = "ca"

	decimal    = 10
	uint32Bits = 32
	rootName   = "root"
	caName     = "ca"
	crtExt     = ".crt"
	keyExt     = ".key"
)

// DefaultAPISANs are always added to the server certificate of the api
var DefaultAPISANs = []string{"localhost", "127.0.0.1", "::1"}

func uniqueValues(values []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		result = append(result, v)
	}
	return result
}

// Init creates the root, the server intermediate with the server certificate of the api, and the api client
// intermediate with the client certificate of the cli. The certificates and private keys of the root and the
// intermediates are stored in the cli directory (below CLICADir), so the api never has them. With a CAPassphrase, their
// private keys are encrypted.
func Init(opts InitOptions) (*oriontls.Chain, error) {
	if err := opts.check(); err != nil {
		return nil, err
	}
	chain := newChain(opts, filepath.Join(opts.CLIDir, CLICADir))
	if err := initialize(&chain); err != nil {
		return nil, err
	}
	if err := WriteAPIFiles(chain, opts.APIDir); err != nil {
		return nil, err
	}
	if err := writeCLIFiles(chain, opts.CLIDir); err != nil {
		return nil, err
	}
	return &chain, nil
}

// Generate creates the same certificates as Init, in memory only: it writes no files, and the private keys are not
// encrypted (CAPassphrase, CLIDir and APIDir are ignored). Bootstrap mode uses it: the api encrypts the private keys
// of the CA to the cli.
func Generate(opts InitOptions) (*oriontls.Chain, error) {
	if err := opts.checkSANs(); err != nil {
		return nil, err
	}
	if err := opts.Subject.Check(); err != nil {
		return nil, err
	}
	opts.CAPassphrase = nil
	chain := newChain(opts, "")
	if err := initialize(&chain); err != nil {
		return nil, err
	}
	return &chain, nil
}

// newChain returns the chain of Init. With caDir, the root and the intermediates are stored there; without, the chain
// has no files.
func newChain(opts InitOptions, caDir string) oriontls.Chain {
	join := func(elem ...string) string {
		if caDir == "" {
			return ""
		}
		return filepath.Join(append([]string{caDir}, elem...)...)
	}
	intermediate := func(name string) oriontls.Pair {
		return oriontls.Pair{
			Cert: oriontls.Cert{Expiry: opts.IntermediateExpiry, Path: join(name, caName+crtExt)},
			PrivateKey: oriontls.PrivateKey{
				Path:       join(name, caName+keyExt),
				Passphrase: opts.CAPassphrase,
			},
		}
	}
	subject := opts.Subject.Subject()
	return oriontls.Chain{
		KeyAlgorithm: opts.KeyAlgorithm,
		Root: oriontls.Pair{
			Cert:       oriontls.Cert{Subject: &subject, Expiry: opts.RootExpiry, Path: join(rootName + crtExt)},
			PrivateKey: oriontls.PrivateKey{Path: join(rootName + keyExt), Passphrase: opts.CAPassphrase},
		},
		Intermediates: oriontls.Intermediates{
			ServerIntermediate: oriontls.Intermediate{
				Cert: intermediate(ServerIntermediate),
				Servers: oriontls.Servers{
					APIServerName: uniqueValues(append(append([]string{}, DefaultAPISANs...), opts.APISANs...)),
				},
				LeafExpiry: opts.LeafExpiry,
			},
			APIClientIntermediate: oriontls.Intermediate{
				Cert:       intermediate(APIClientIntermediate),
				Clients:    []string{CLIClientName},
				LeafExpiry: opts.LeafExpiry,
			},
		},
	}
}

// initialize creates the root and the intermediates of chain, with their certificates
func initialize(chain *oriontls.Chain) error {
	if err := chain.InitializeCA(); err != nil {
		return fmt.Errorf("failed to create the root: %w", err)
	}
	if err := chain.InitializeIntermediates(); err != nil {
		return fmt.Errorf("failed to create the intermediates: %w", err)
	}
	return nil
}

type file struct {
	path    string
	data    []byte
	private bool
}

func writeFiles(files []file) error {
	for _, f := range files {
		if err := oriontls.WriteFile(f.path, f.data, f.private); err != nil {
			return fmt.Errorf("failed to write %s: %w", f.path, err)
		}
	}
	return nil
}

// WriteAPIFiles writes the files of the api to dir: its server certificate (with the server intermediate), its private
// key, and the api client intermediate to verify clients
func WriteAPIFiles(chain oriontls.Chain, dir string) error {
	serverIM := chain.Intermediates[ServerIntermediate]
	server := serverIM.Children()[APIServerName]
	clientIM := chain.Intermediates[APIClientIntermediate]
	return writeFiles([]file{
		{
			path: filepath.Join(dir, APIServerCertFile),
			data: append(append([]byte{}, server.Cert.PEM...), serverIM.Cert.Cert.PEM...),
		},
		{path: filepath.Join(dir, APIServerKeyFile), data: server.PrivateKey.PEM, private: true},
		{path: filepath.Join(dir, APIClientCAFile), data: clientIM.Cert.Cert.PEM},
	})
}

func writeCLIFiles(chain oriontls.Chain, dir string) error {
	client := chain.Intermediates[APIClientIntermediate].Children()[CLIClientName]
	return writeFiles([]file{
		{path: filepath.Join(dir, CLICertFile), data: client.Cert.PEM},
		{path: filepath.Join(dir, CLIKeyFile), data: client.PrivateKey.PEM, private: true},
		{path: filepath.Join(dir, CLICAFile), data: chain.Root.Cert.PEM},
	})
}

// ParseValidity parses a validity, like a time.Duration, but it also accepts days (e.g. 30d) and years (e.g. 5y,
// of 365 days)
func ParseValidity(s string) (time.Duration, error) {
	for suffix, unit := range map[string]time.Duration{"y": oriontls.Year, "d": oriontls.Day} {
		if number, ok := strings.CutSuffix(s, suffix); ok {
			n, err := strconv.ParseUint(number, decimal, uint32Bits)
			if err != nil || n == 0 || n > uint64(math.MaxInt64/int64(unit)) {
				return 0, fmt.Errorf("invalid validity %q", s)
			}
			return time.Duration(n) * unit, nil
		}
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid validity %q", s)
	}
	return d, nil
}
