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
	"crypto/x509"
	"fmt"
	"net"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
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
	Short: "Renew the server certificate of the api",
	Long: `Renew the server certificate of the api, signed by the server intermediate in --pki-dir.

Online (without --api-dir): the api creates a new private key, which never leaves the api, and
sends a certificate request for it. The cli signs it, and the api stores the new certificate and
serves new connections with it, without a restart. The api must have a valid certificate: an
expired one is renewed offline.

Offline (with --api-dir): the cli writes a new certificate and key to --api-dir, together with the
api client intermediate (client-ca.crt): the same files as pki init. Copy them to the --pki-dir of
the api: it loads them within a minute, or right away on SIGHUP, without a restart.

The new certificate keeps the names of the current one (the one the api serves with; offline, the
one in --api-dir, when there is one), and gets localhost, 127.0.0.1, ::1, the configured api host
(--host) and the names in --api-san.

The private key of the intermediate is decrypted with the passphrase of the CA keys: it is read
from --ca-passphrase-file, else from ORIONCLI_CA_PASSPHRASE, else the cli asks for it.

The new certificate never outlives the intermediate: when the intermediate expires first, the cli
warns, and the intermediate needs to be renewed.`,
}

var cmdPKIRenewIntermediate = &cobra.Command{
	Use:       "intermediate {" + strings.Join(pki.Intermediates(), "|") + "}",
	RunE:      runPKIRenewIntermediate,
	Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	ValidArgs: pki.Intermediates(),
	Short:     "Renew an intermediate, and the certificates it signs",
	Long: `Replace an intermediate in --pki-dir (its certificate and private key) with a new one, with the
same subject, signed by the root, and renew the certificates it signs: they don't chain to the
new key.

  server       the server certificate of the api: written to --api-dir, like pki renew api.
  api-clients  the client certificate of the cli, in --pki-dir, like pki renew cli; the new
               intermediate is written to --api-dir as client-ca.crt.

Copy the files in --api-dir to the --pki-dir of the api: it loads them within a minute, or right
away on SIGHUP, without a restart. After renewing api-clients, the cli can't reach the api until
the api has the new client-ca.crt.

The current certificate of the intermediate is kept as ca/<intermediate>/previous.crt: pki renew
cli and pki renew api still renew certificates it signed, e.g. when this renewal stopped halfway.

The private keys of the root and the intermediates are decrypted with the passphrase of the CA
keys, which also encrypts the new key: it is read from --ca-passphrase-file, else from
ORIONCLI_CA_PASSPHRASE, else the cli asks for it.

The new intermediate never outlives the root: when the root expires first, the cli warns.`,
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
	pkiRenewIntermediateOpts struct {
		renewFlags
		intermediateValidity string
		apiDir               string
		apiSANs              []string
	}
)

func init() {
	pkiRenewCLIOpts.add(cmdPKIRenewCLI)
	pkiRenewAPIOpts.add(cmdPKIRenewAPI)
	flags := cmdPKIRenewAPI.Flags()
	flags.StringVar(&pkiRenewAPIOpts.apiDir, "api-dir", "",
		"renew offline: directory to write the api files to (default: renew online, on the api)")
	flags.StringSliceVar(&pkiRenewAPIOpts.apiSANs, "api-san", nil,
		"extra name or ip address for the api certificate (can be repeated)")
	pkiRenewIntermediateOpts.add(cmdPKIRenewIntermediate)
	flags = cmdPKIRenewIntermediate.Flags()
	flags.StringVar(&pkiRenewIntermediateOpts.intermediateValidity, "intermediate-validity", "3y",
		"validity of the new intermediate (e.g. 3y, 1000d)")
	flags.StringVar(&pkiRenewIntermediateOpts.apiDir, "api-dir", "", "directory to write the api files to")
	flags.StringSliceVar(&pkiRenewIntermediateOpts.apiSANs, "api-san", nil,
		"extra name or ip address for the api certificate (server; can be repeated)")
	_ = cmdPKIRenewIntermediate.MarkFlagRequired("api-dir")
	cmdPKIRenew.AddCommand(cmdPKIRenewCLI, cmdPKIRenewAPI, cmdPKIRenewIntermediate)
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
	host := apiHost(c)
	sans := append([]string{host}, pkiRenewAPIOpts.apiSANs...)
	cliDir := filepath.Dir(files.CertFile)
	validity, algorithm, passphrase, err := pkiRenewAPIOpts.parse()
	if err != nil {
		return err
	}
	if pkiRenewAPIOpts.apiDir == "" {
		return renewAPIOnline(c, files, host, sans, validity, algorithm, passphrase)
	}
	apiDir, err := pki.ExpandHome(pkiRenewAPIOpts.apiDir)
	if err != nil {
		return err
	}
	return renewAPIOffline(c, cliDir, apiDir, pkiRenewAPIOpts.apiSANs, validity, algorithm, passphrase)
}

// apiHost returns the configured host of the api (--host)
func apiHost(c *cobra.Command) string {
	if c.Flags().Changed(flagHost) {
		host, _ := c.Flags().GetString(flagHost)
		return host
	}
	return viper.GetString(flagHost)
}

// renewAPIOffline writes a new certificate and key for the api to apiDir, with the files the api needs
func renewAPIOffline(c *cobra.Command, cliDir, apiDir string, extraSANs []string, validity time.Duration,
	algorithm oriontls.KeyAlgorithm, passphrase []byte) error {
	opts := pki.RenewAPIOptions{
		CLIDir: cliDir, APIDir: apiDir, APISANs: append([]string{apiHost(c)}, extraSANs...), LeafExpiry: validity,
		KeyAlgorithm: algorithm, CAPassphrase: passphrase,
	}
	result, names, err := pki.RenewAPI(opts)
	if err != nil {
		return fmt.Errorf("failed to renew the api certificate: %w", err)
	}
	printRenewed("api certificate", opts.APIDir, result)
	stdout("Names on the certificate: %s", strings.Join(names, ", "))
	stdout("Copy %s, %s and %s to the --pki-dir of the api: it loads them within a minute, or right away on SIGHUP",
		pki.APIServerCertFile, pki.APIServerKeyFile, pki.APIClientCAFile)
	return nil
}

// trustedAPICertificate returns the certificate the api at host serves with, after checking it against the root of
// the cli
func trustedAPICertificate(c *cobra.Command, files pki.TLSFiles, host string) (*x509.Certificate, error) {
	address := net.JoinHostPort(host, strconv.Itoa(int(viper.GetUint16(flagPort))))
	peer, err := apiCertificates(c.Context(), address, host, viper.GetDuration(flagTimeout))
	if err != nil {
		return nil, fmt.Errorf("failed to get the certificate of the api at %s: %w", address, err)
	}
	if len(peer) == 0 {
		return nil, fmt.Errorf("the api at %s sent no certificate", address)
	}
	roots, _, err := pki.LoadCertPool(files.CAFile)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if now.After(peer[0].NotAfter) {
		return nil, fmt.Errorf("the certificate of the api at %s expired at %s: renew it offline, with --api-dir, "+
			"and copy the files to the api", address, peer[0].NotAfter.Format(time.RFC3339))
	}
	if err = checkAPIChain(peer, roots, host, now); err != nil {
		return nil, fmt.Errorf("the certificate of the api at %s isn't trusted by the root of the cli: %w", address,
			err)
	}
	return peer[0], nil
}

// renewAPIOnline renews the certificate of the api on the api: the api makes the key and a certificate request, the
// cli signs it, and the api serves with the new certificate
func renewAPIOnline(c *cobra.Command, files pki.TLSFiles, host string, sans []string, validity time.Duration,
	algorithm oriontls.KeyAlgorithm, passphrase []byte) error {
	current, err := trustedAPICertificate(c, files, host)
	if err != nil {
		return err
	}
	apiClient, err := defaultAPIClient()
	if err != nil {
		return err
	}
	request, code, err := apiClient.RequestAPICertificate(endpoints.APICertificateRequestRequest{
		KeyAlgorithm: string(algorithm),
	})
	if err != nil {
		return apiError("the api didn't create a certificate request", code, err)
	}
	signed, err := pki.SignAPIRequest(pki.SignAPIRequestOptions{
		CLIDir: filepath.Dir(files.CertFile), CAPassphrase: passphrase, Request: request.Request, Current: current,
		APISANs: sans, LeafExpiry: validity,
	})
	if err != nil {
		return fmt.Errorf("failed to sign the certificate request of the api: %w", err)
	}
	if _, code, err = apiClient.PutAPICertificate(endpoints.APICertificate{Chain: signed.Chain}); err != nil {
		return apiError("the api didn't take the new certificate", code, err)
	}
	// a new connection must get the new certificate
	apiClient.CloseIdleConnections()
	serving, err := trustedAPICertificate(c, files, host)
	if err != nil {
		return fmt.Errorf("the api took the new certificate, but: %w", err)
	}
	if serving.SerialNumber.Cmp(signed.Renewed.SerialNumber) != 0 {
		return fmt.Errorf("the api took the new certificate (serial %x), but serves with serial %x",
			signed.Renewed.SerialNumber, serving.SerialNumber)
	}
	printRenewed("api certificate", "the api at "+net.JoinHostPort(host, strconv.Itoa(int(viper.GetUint16(flagPort)))),
		&signed.RenewResult)
	stdout("Names on the certificate: %s", strings.Join(signed.SANs, ", "))
	stdout("The api serves new connections with it; its private key never left the api")
	return nil
}

func runPKIRenewIntermediate(c *cobra.Command, args []string) error {
	files, err := cliTLSFiles()
	if err != nil {
		return err
	}
	cliDir := filepath.Dir(files.CertFile)
	name := args[0]
	leafValidity, algorithm, passphrase, err := pkiRenewIntermediateOpts.parse()
	if err != nil {
		return err
	}
	validity, err := pki.ParseValidity(pkiRenewIntermediateOpts.intermediateValidity)
	if err != nil {
		return fmt.Errorf("--intermediate-validity: %w", err)
	}
	apiDir, err := pki.ExpandHome(pkiRenewIntermediateOpts.apiDir)
	if err != nil {
		return err
	}
	// check the api directory before anything changes
	if err = (pki.InitOptions{CLIDir: cliDir, APIDir: apiDir}).CheckDirs(); err != nil {
		return err
	}
	result, err := pki.RenewIntermediate(pki.RenewIntermediateOptions{
		CLIDir: cliDir, Name: name, CAPassphrase: passphrase, Expiry: validity, KeyAlgorithm: algorithm,
	})
	if err != nil {
		return fmt.Errorf("failed to renew the intermediate %s: %w", name, err)
	}
	stdout("Renewed the intermediate %s in %s: valid until %s (serial %x)", name,
		filepath.Join(cliDir, pki.CLICADir, name), result.Renewed.NotAfter.Format(time.RFC3339),
		result.Renewed.SerialNumber)
	if result.Capped {
		stderr("warning: the intermediate expires with the root at %s, before the requested validity",
			result.Intermediate.NotAfter.Format(time.RFC3339))
	}
	if name == pki.ServerIntermediate {
		return renewAPIOffline(c, cliDir, apiDir, pkiRenewIntermediateOpts.apiSANs, leafValidity, algorithm,
			passphrase)
	}
	cliResult, err := pki.RenewCLI(pki.RenewCLIOptions{
		CLIDir: cliDir, CAPassphrase: passphrase, LeafExpiry: leafValidity, KeyAlgorithm: algorithm,
	})
	if err != nil {
		return fmt.Errorf("failed to renew the client certificate (renew it with `orion-cli pki renew cli`): %w", err)
	}
	printRenewed("client certificate", cliDir, cliResult)
	path, err := pki.WriteAPIClientCA(cliDir, apiDir)
	if err != nil {
		return err
	}
	stdout("Copy %s to the --pki-dir of the api: it loads it within a minute, or right away on SIGHUP. Until then, "+
		"the api refuses the new client certificate of the cli", path)
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
