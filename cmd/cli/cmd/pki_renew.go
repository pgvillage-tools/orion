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
	"github.com/spf13/viper"
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

var cmdPKIRenewAPI = &cobra.Command{
	Use:   "api",
	RunE:  runPKIRenewAPI,
	Args:  cobra.NoArgs,
	Short: "Renew the server certificate of the api, in a directory to copy to the api",
	Long: `Write a new server certificate and private key for the api to --api-dir, signed by the server
intermediate in --pki-dir, together with the api client intermediate (client-ca.crt): the same
files as pki init. Copy them to the --pki-dir of the api: it loads them within a minute, or right
away on SIGHUP, without a restart.

The new certificate keeps the names of the certificate in --api-dir (when there is one; it must be
of this CA), and gets localhost, 127.0.0.1, ::1, the configured api host (--host) and the names
in --api-san.

The private key of the intermediate is decrypted with the passphrase of the CA keys: it is read
from --ca-passphrase-file, else from ORIONCLI_CA_PASSPHRASE, else the cli asks for it.

The new certificate never outlives the intermediate: when the intermediate expires first, the cli
warns, and the intermediate needs to be renewed.`,
}

// renewFlags are the flags of every renewal
type renewFlags struct {
	leafValidity     string
	keyAlgorithm     string
	caPassphraseFile string
}

func (f *renewFlags) add(c *cobra.Command) {
	flags := c.Flags()
	flags.StringVar(&f.leafValidity, "leaf-validity", "1y", "validity of the new certificate (e.g. 1y, 90d, 2160h)")
	flags.StringVar(&f.keyAlgorithm, "key-algorithm", "",
		"algorithm of the new private key ("+keyAlgorithmNames()+"; default: that of the current key)")
	flags.StringVar(&f.caPassphraseFile, "ca-passphrase-file", "",
		"file with the passphrase for the CA keys (- for stdin; default: "+envCAPassphrase+", else a prompt)")
}

// parse returns the validity, the key algorithm and the passphrase of the flags
func (f *renewFlags) parse() (time.Duration, oriontls.KeyAlgorithm, []byte, error) {
	validity, err := pki.ParseValidity(f.leafValidity)
	if err != nil {
		return 0, "", nil, fmt.Errorf("--leaf-validity: %w", err)
	}
	algorithm := oriontls.KeyAlgorithm(f.keyAlgorithm)
	if algorithm != "" && !slices.Contains(oriontls.KeyAlgorithms(), algorithm) {
		return 0, "", nil, fmt.Errorf("--key-algorithm: unknown algorithm %q", algorithm)
	}
	passphrase, err := existingCAPassphrase(f.caPassphraseFile)
	if err != nil {
		return 0, "", nil, err
	}
	return validity, algorithm, passphrase, nil
}

var (
	pkiRenewCLIOpts renewFlags
	pkiRenewAPIOpts struct {
		renewFlags
		apiDir  string
		apiSANs []string
	}
)

func init() {
	pkiRenewCLIOpts.add(cmdPKIRenewCLI)
	pkiRenewAPIOpts.add(cmdPKIRenewAPI)
	flags := cmdPKIRenewAPI.Flags()
	flags.StringVar(&pkiRenewAPIOpts.apiDir, "api-dir", "", "directory to write the api files to (required)")
	flags.StringSliceVar(&pkiRenewAPIOpts.apiSANs, "api-san", nil,
		"extra name or ip address for the api certificate (can be repeated)")
	_ = cmdPKIRenewAPI.MarkFlagRequired("api-dir")
	cmdPKIRenew.AddCommand(cmdPKIRenewCLI, cmdPKIRenewAPI)
	cmdPKI.AddCommand(cmdPKIRenew)
}

// printRenewed reports the renewed certificate in dir, and warns when it expires with its intermediate
func printRenewed(what, dir string, result *pki.RenewResult) {
	stdout("Renewed the %s %s in %s: valid until %s (serial %x)", what, result.Renewed.Subject.CommonName, dir,
		result.Renewed.NotAfter.Format(time.RFC3339), result.Renewed.SerialNumber)
	if result.Capped {
		stderr("warning: the certificate expires with the intermediate %s at %s, before the requested validity: "+
			"renew the intermediate", result.Intermediate.Subject.CommonName,
			result.Intermediate.NotAfter.Format(time.RFC3339))
	}
}

func runPKIRenewCLI(_ *cobra.Command, _ []string) error {
	files, err := cliTLSFiles()
	if err != nil {
		return err
	}
	opts := pki.RenewCLIOptions{CLIDir: filepath.Dir(files.CertFile)}
	if opts.LeafExpiry, opts.KeyAlgorithm, opts.CAPassphrase, err = pkiRenewCLIOpts.parse(); err != nil {
		return err
	}
	result, err := pki.RenewCLI(opts)
	if err != nil {
		return fmt.Errorf("failed to renew the client certificate: %w", err)
	}
	printRenewed("client certificate", opts.CLIDir, result)
	return nil
}

func runPKIRenewAPI(c *cobra.Command, _ []string) error {
	files, err := cliTLSFiles()
	if err != nil {
		return err
	}
	host := viper.GetString(flagHost)
	if c.Flags().Changed(flagHost) {
		host, _ = c.Flags().GetString(flagHost)
	}
	opts := pki.RenewAPIOptions{
		CLIDir:  filepath.Dir(files.CertFile),
		APISANs: append([]string{host}, pkiRenewAPIOpts.apiSANs...),
	}
	if opts.APIDir, err = pki.ExpandHome(pkiRenewAPIOpts.apiDir); err != nil {
		return err
	}
	if opts.LeafExpiry, opts.KeyAlgorithm, opts.CAPassphrase, err = pkiRenewAPIOpts.parse(); err != nil {
		return err
	}
	result, sans, err := pki.RenewAPI(opts)
	if err != nil {
		return fmt.Errorf("failed to renew the api certificate: %w", err)
	}
	printRenewed("api certificate", opts.APIDir, result)
	stdout("Names on the certificate: %s", strings.Join(sans, ", "))
	stdout("Copy %s, %s and %s to the --pki-dir of the api: it loads them within a minute, or right away on SIGHUP",
		pki.APIServerCertFile, pki.APIServerKeyFile, pki.APIClientCAFile)
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
