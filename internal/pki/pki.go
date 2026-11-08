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
// stored. See docs/design/bootstrap-design.md.
package pki

import (
	"errors"
	"fmt"
	"math"
	"os"
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

// InitOptions holds the options to initialize the PKI
type InitOptions struct {
	// CLIDir is where the cli files are stored (e.g. ~/.orion/cli)
	CLIDir string
	// APIDir is where the api files are stored (e.g. ~/.orion/api)
	APIDir string
	// APISANs are added to the server certificate of the api, on top of DefaultAPISANs
	APISANs []string
	// KeyAlgorithm is used for all private keys (oriontls.DefaultKeyAlgorithm when not set)
	KeyAlgorithm oriontls.KeyAlgorithm
	// RootExpiry is the validity of the root (oriontls.DefaultRootExpiry when not set)
	RootExpiry time.Duration
	// IntermediateExpiry is the validity of the intermediates (oriontls.DefaultIntermediateExpiry when not set)
	IntermediateExpiry time.Duration
	// LeafExpiry is the validity of the server and client certificates (oriontls.DefaultExpiry when not set)
	LeafExpiry time.Duration
	// Force overwrites existing files
	Force bool
}

// Files returns the files that Init creates
func (o InitOptions) Files() []string {
	caDir := filepath.Join(o.CLIDir, CLICADir)
	files := []string{
		filepath.Join(o.APIDir, APIServerCertFile),
		filepath.Join(o.APIDir, APIServerKeyFile),
		filepath.Join(o.APIDir, APIClientCAFile),
		filepath.Join(o.CLIDir, CLICertFile),
		filepath.Join(o.CLIDir, CLIKeyFile),
		filepath.Join(o.CLIDir, CLICAFile),
		filepath.Join(caDir, rootName+crtExt),
		filepath.Join(caDir, rootName+keyExt),
	}
	for _, im := range []string{ServerIntermediate, APIClientIntermediate} {
		files = append(files,
			filepath.Join(caDir, im, caName+crtExt),
			filepath.Join(caDir, im, caName+keyExt),
		)
	}
	return files
}

// realPath returns the absolute path of path, with symlinks resolved as far as the path exists
func realPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	existing, rest := abs, ""
	for {
		if resolved, evalErr := filepath.EvalSymlinks(existing); evalErr == nil {
			return filepath.Join(resolved, rest), nil
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return abs, nil
		}
		rest = filepath.Join(filepath.Base(existing), rest)
		existing = parent
	}
}

// isWithin returns true when path is dir, or is below dir
func isWithin(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// checkDirs refuses an api directory that contains the CA directory of the cli: copying the api directory to the
// api host would copy the CA keys too
func (o InitOptions) checkDirs() error {
	apiDir, err := realPath(o.APIDir)
	if err != nil {
		return err
	}
	caDir, err := realPath(filepath.Join(o.CLIDir, CLICADir))
	if err != nil {
		return err
	}
	if isWithin(caDir, apiDir) {
		return fmt.Errorf("the api directory %s contains the CA directory %s: use separate directories", o.APIDir,
			filepath.Join(o.CLIDir, CLICADir))
	}
	return nil
}

func (o InitOptions) checkSANs() error {
	for _, san := range o.APISANs {
		if !oriontls.IsValidServer(san) {
			return fmt.Errorf("invalid name or address for the api certificate: %q", san)
		}
	}
	return nil
}

func (o InitOptions) check() error {
	if o.CLIDir == "" || o.APIDir == "" {
		return errors.New("both the cli and the api directory need to be set")
	}
	if err := o.checkDirs(); err != nil {
		return err
	}
	if err := o.checkSANs(); err != nil {
		return err
	}
	if o.Force {
		return nil
	}
	var dirs []string
	for _, f := range o.Files() {
		if _, err := os.Stat(f); err == nil {
			dirs = append(dirs, filepath.Dir(f))
		}
	}
	if len(dirs) > 0 {
		return fmt.Errorf("refusing to overwrite existing files in %s (use force to overwrite)",
			strings.Join(uniqueValues(dirs), ", "))
	}
	return nil
}

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
// intermediates are stored in the cli directory (below CLICADir), so the api never has them.
func Init(opts InitOptions) (*oriontls.Chain, error) {
	if err := opts.check(); err != nil {
		return nil, err
	}
	caDir := filepath.Join(opts.CLIDir, CLICADir)
	intermediate := func(name string) oriontls.Pair {
		return oriontls.Pair{
			Cert:       oriontls.Cert{Expiry: opts.IntermediateExpiry, Path: filepath.Join(caDir, name, caName+crtExt)},
			PrivateKey: oriontls.PrivateKey{Path: filepath.Join(caDir, name, caName+keyExt)},
		}
	}
	chain := oriontls.Chain{
		KeyAlgorithm: opts.KeyAlgorithm,
		Root: oriontls.Pair{
			Cert:       oriontls.Cert{Expiry: opts.RootExpiry, Path: filepath.Join(caDir, rootName+crtExt)},
			PrivateKey: oriontls.PrivateKey{Path: filepath.Join(caDir, rootName+keyExt)},
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
	if err := chain.InitializeCA(); err != nil {
		return nil, fmt.Errorf("failed to create the root: %w", err)
	}
	if err := chain.InitializeIntermediates(); err != nil {
		return nil, fmt.Errorf("failed to create the intermediates: %w", err)
	}
	if err := writeAPIFiles(chain, opts.APIDir); err != nil {
		return nil, err
	}
	if err := writeCLIFiles(chain, opts.CLIDir); err != nil {
		return nil, err
	}
	return &chain, nil
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

func writeAPIFiles(chain oriontls.Chain, dir string) error {
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
