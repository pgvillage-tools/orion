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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
)

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
