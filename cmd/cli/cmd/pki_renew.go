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
	flags.StringVar(&pkiRenewAPIOpts.apiDir, "api-dir", "",
		"renew offline: directory to write the api files to (default: renew online, on the api)")
	flags.StringSliceVar(&pkiRenewAPIOpts.apiSANs, "api-san", nil,
		"extra name or ip address for the api certificate (can be repeated)")
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
	sans := append([]string{host}, pkiRenewAPIOpts.apiSANs...)
	cliDir := filepath.Dir(files.CertFile)
	validity, algorithm, passphrase, err := pkiRenewAPIOpts.parse()
	if err != nil {
		return err
	}
	if pkiRenewAPIOpts.apiDir == "" {
		return renewAPIOnline(c, files, host, sans, validity, algorithm, passphrase)
	}
	opts := pki.RenewAPIOptions{
		CLIDir: cliDir, APISANs: sans, LeafExpiry: validity, KeyAlgorithm: algorithm, CAPassphrase: passphrase,
	}
	if opts.APIDir, err = pki.ExpandHome(pkiRenewAPIOpts.apiDir); err != nil {
		return err
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

// keyAlgorithmNames returns the supported key algorithms, for the help of a flag
func keyAlgorithmNames() string {
	var names []string
	for _, a := range oriontls.KeyAlgorithms() {
		names = append(names, string(a))
	}
	return strings.Join(names, ", ")
}
