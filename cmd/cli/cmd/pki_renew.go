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
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/pgvillage-tools/orion/internal/pki"
	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
	"github.com/spf13/cobra"
)

var cmdPKIRenew = &cobra.Command{
	Use:   "renew",
	Short: "Renew certificates of the cli and the api",
}

var cmdPKIRenewCLI = &cobra.Command{
	Use:   "cli",
	RunE:  runPKIRenewCLI,
	Args:  cobra.NoArgs,
	Short: "Renew the client certificate of the cli",
	Long: `Replace the client certificate of the cli in --pki-dir, and its private key, with a new one.

The new certificate has the same subject, and is signed by the api client intermediate in
--pki-dir, so the api accepts it right away: nothing changes on the api. The current certificate
may have expired, but it must be signed by that intermediate.

The private key of the intermediate is decrypted with the passphrase of the CA keys: it is read
from --ca-passphrase-file, else from ORIONCLI_CA_PASSPHRASE, else the cli asks for it.

The new certificate never outlives the intermediate: when the intermediate expires first, the cli
warns, and the intermediate needs to be renewed.`,
}

var pkiRenewCLIOpts struct {
	leafValidity     string
	keyAlgorithm     string
	caPassphraseFile string
}

func init() {
	flags := cmdPKIRenewCLI.Flags()
	flags.StringVar(&pkiRenewCLIOpts.leafValidity, "leaf-validity", "1y",
		"validity of the new certificate (e.g. 1y, 90d, 2160h)")
	flags.StringVar(&pkiRenewCLIOpts.keyAlgorithm, "key-algorithm", "",
		"algorithm of the new private key ("+keyAlgorithmNames()+"; default: that of the current key)")
	flags.StringVar(&pkiRenewCLIOpts.caPassphraseFile, "ca-passphrase-file", "",
		"file with the passphrase for the CA keys (- for stdin; default: "+envCAPassphrase+", else a prompt)")
	cmdPKIRenew.AddCommand(cmdPKIRenewCLI)
	cmdPKI.AddCommand(cmdPKIRenew)
}

func runPKIRenewCLI(_ *cobra.Command, _ []string) error {
	files, err := cliTLSFiles()
	if err != nil {
		return err
	}
	opts := pki.RenewCLIOptions{
		CLIDir:       filepath.Dir(files.CertFile),
		KeyAlgorithm: oriontls.KeyAlgorithm(pkiRenewCLIOpts.keyAlgorithm),
	}
	if opts.LeafExpiry, err = pki.ParseValidity(pkiRenewCLIOpts.leafValidity); err != nil {
		return fmt.Errorf("--leaf-validity: %w", err)
	}
	if opts.KeyAlgorithm != "" && !slices.Contains(oriontls.KeyAlgorithms(), opts.KeyAlgorithm) {
		return fmt.Errorf("--key-algorithm: unknown algorithm %q", opts.KeyAlgorithm)
	}
	if opts.CAPassphrase, err = existingCAPassphrase(pkiRenewCLIOpts.caPassphraseFile); err != nil {
		return err
	}
	result, err := pki.RenewCLI(opts)
	if err != nil {
		return fmt.Errorf("failed to renew the client certificate: %w", err)
	}
	stdout("Renewed the client certificate %s in %s: valid until %s (serial %x)",
		result.Renewed.Subject.CommonName, opts.CLIDir, result.Renewed.NotAfter.Format(time.RFC3339),
		result.Renewed.SerialNumber)
	if result.Capped {
		stderr("warning: the certificate expires with the intermediate %s at %s, before the requested validity: "+
			"renew the intermediate", result.Intermediate.Subject.CommonName,
			result.Intermediate.NotAfter.Format(time.RFC3339))
	}
	return nil
}

// keyAlgorithmNames returns the supported key algorithms, for the help of a flag
func keyAlgorithmNames() string {
	var names []string
	for _, a := range oriontls.KeyAlgorithms() {
		names = append(names, string(a))
	}
	return strings.Join(names, ", ")
}
