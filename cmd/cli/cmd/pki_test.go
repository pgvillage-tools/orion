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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pgvillage-tools/orion/internal/pki"
	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
)

func TestPKIInitOptions(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	o := pkiInitOptions{
		apiDir:               "~/api",
		apiSANs:              []string{"orion-api.example.org"},
		keyAlgorithm:         string(oriontls.KeyAlgorithmRSA2048),
		rootValidity:         "10y",
		intermediateValidity: "1000d",
		leafValidity:         "720h",
	}
	opts, err := o.initOptions("/cli", "api.example.org")
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIDir != filepath.Join(home, "api") || opts.CLIDir != "/cli" {
		t.Errorf("unexpected directories %s and %s", opts.APIDir, opts.CLIDir)
	}
	if len(opts.APISANs) != 2 || opts.APISANs[0] != "api.example.org" || opts.APISANs[1] != "orion-api.example.org" {
		t.Errorf("unexpected SANs %v", opts.APISANs)
	}
	if opts.RootExpiry != 10*oriontls.Year || opts.IntermediateExpiry != 1000*oriontls.Day ||
		opts.LeafExpiry != 720*time.Hour {
		t.Errorf("unexpected validities %s, %s, %s", opts.RootExpiry, opts.IntermediateExpiry, opts.LeafExpiry)
	}
	if opts.KeyAlgorithm != oriontls.KeyAlgorithmRSA2048 {
		t.Errorf("unexpected key algorithm %s", opts.KeyAlgorithm)
	}

	o.leafValidity = "forever"
	if _, err = o.initOptions("/cli", "localhost"); err == nil {
		t.Error("expected an invalid validity to be refused")
	}
}

func TestCLITLSFiles(t *testing.T) {
	dir := t.TempDir()
	usePKIDir(t, dir)
	files, err := cliTLSFiles()
	if err != nil {
		t.Fatal(err)
	}
	if files != pki.CLIFiles(dir) {
		t.Errorf("unexpected files %+v", files)
	}
}
