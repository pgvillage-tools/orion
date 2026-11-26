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
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"path/filepath"
	"strconv"
	"time"

	"github.com/pgvillage-tools/orion/internal/pki"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	hoursPerDay = 24

	statusValid      = "valid"
	statusExpiring   = "expires soon"
	statusExpired    = "expired"
	statusNotYet     = "not valid yet"
	sourceCLI        = "cli"
	sourceAPI        = "api"
	listFormatText   = "text"
	listFormatJSON   = "json"
	listTimeFormat   = "2006-01-02 15:04"
	listMissingFiles = "no certificates found in %s: create them with `orion-cli pki init` or `orion-cli bootstrap`"
)

var cmdPKIList = &cobra.Command{
	Use:   "list",
	RunE:  runPKIList,
	Args:  cobra.NoArgs,
	Short: "List the certificates of the cli and the api, and when they expire",
	Long: `List the certificates in --pki-dir (the client certificate of the cli, the root and the
intermediates), and the certificates the api sends (its server certificate and chain).

For the api, the cli only does a TLS handshake: it doesn't need a valid client certificate or the
passphrase, so it also lists an api certificate that expired. It checks the certificates of the api
against the root in --pki-dir, and reports when they don't match. Use --local to leave out the api.

A certificate that expires within 30 days is marked "expires soon".`,
}

var pkiListOpts struct {
	local  bool
	format string
}

func init() {
	flags := cmdPKIList.Flags()
	flags.BoolVar(&pkiListOpts.local, "local", false, "only list the certificates in --pki-dir, not those of the api")
	flags.StringVarP(&pkiListOpts.format, "format", "f", listFormatText, "output format (text or json)")
	cmdPKI.AddCommand(cmdPKIList)
}

// listedCert is a certificate in the output of pki list
type listedCert struct {
	// Source is cli (a file in --pki-dir) or api (sent by the api)
	Source string `json:"source"`
	// File is the file in --pki-dir, relative to it (for the cli)
	File       string    `json:"file,omitempty"`
	CommonName string    `json:"commonName"`
	Issuer     string    `json:"issuer"`
	Serial     string    `json:"serial"`
	IsCA       bool      `json:"isCA"`
	NotBefore  time.Time `json:"notBefore"`
	NotAfter   time.Time `json:"notAfter"`
	// Status is valid, expires soon, expired or not valid yet
	Status string `json:"status"`
}

// apiCheck is the result of checking the certificates of the api
type apiCheck struct {
	Address string `json:"address"`
	// Trusted is true when the certificate of the api verifies against the root of the cli, for the api host
	Trusted bool `json:"trusted"`
	// Error explains why the certificate isn't trusted, or why the api couldn't be reached
	Error string `json:"error,omitempty"`
}

// pkiList is the output of pki list
type pkiList struct {
	Certificates []listedCert `json:"certificates"`
	API          *apiCheck    `json:"api,omitempty"`
}

// certStatus returns the status of c at now
func certStatus(c *x509.Certificate, now time.Time) string {
	switch {
	case now.After(c.NotAfter):
		return statusExpired
	case now.Before(c.NotBefore):
		return statusNotYet
	case c.NotAfter.Before(now.Add(pki.ExpiryWarning)):
		return statusExpiring
	}
	return statusValid
}

func newListedCert(source, file string, c *x509.Certificate, now time.Time) listedCert {
	return listedCert{
		Source:     source,
		File:       file,
		CommonName: c.Subject.CommonName,
		Issuer:     c.Issuer.CommonName,
		Serial:     fmt.Sprintf("%x", c.SerialNumber),
		IsCA:       c.IsCA,
		NotBefore:  c.NotBefore,
		NotAfter:   c.NotAfter,
		Status:     certStatus(c, now),
	}
}

// cliCertFiles returns the files with certificates in the cli directory, relative to it: the client certificate
// first, then the intermediates and the root
func cliCertFiles() []string {
	caDir := pki.CLICADir
	return []string{
		pki.CLICertFile,
		filepath.Join(caDir, pki.APIClientIntermediate, "ca.crt"),
		filepath.Join(caDir, pki.ServerIntermediate, "ca.crt"),
		filepath.Join(caDir, "root.crt"),
		pki.CLICAFile,
	}
}

// listLocal returns the certificates in the cli directory dir, each once (a certificate in more than one file is
// listed with the first), and the root pool of the cli (from ca.crt) when it exists
func listLocal(dir string, now time.Time) ([]listedCert, *x509.CertPool, error) {
	var (
		listed []listedCert
		roots  *x509.CertPool
		seen   = map[[sha256.Size]byte]bool{}
	)
	for _, file := range cliCertFiles() {
		pool, certs, err := pki.LoadCertPool(filepath.Join(dir, file))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		if file == pki.CLICAFile {
			roots = pool
		}
		for _, c := range certs {
			fingerprint := sha256.Sum256(c.Raw)
			if seen[fingerprint] {
				continue
			}
			seen[fingerprint] = true
			listed = append(listed, newListedCert(sourceCLI, file, c, now))
		}
	}
	if len(listed) == 0 {
		return nil, nil, fmt.Errorf(listMissingFiles, dir)
	}
	return listed, roots, nil
}

// apiCertificates does a TLS handshake with the api at address, and returns the certificates it sends. It doesn't
// verify them, and doesn't send a client certificate: it only looks.
func apiCertificates(ctx context.Context, address, serverName string, timeout time.Duration) ([]*x509.Certificate,
	error) {
	var peer []*x509.Certificate
	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: timeout},
		Config: &tls.Config{
			MinVersion: tls.VersionTLS13,
			ServerName: serverName,
			// the certificates are verified by checkAPIChain, also when they expired: this connection sends
			// nothing, and is closed right after the handshake
			InsecureSkipVerify: true, //nolint:gosec // only reads the certificates of the api; see above
			VerifyConnection: func(cs tls.ConnectionState) error {
				peer = cs.PeerCertificates
				return nil
			},
		},
	}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil && len(peer) == 0 {
		return nil, err
	}
	if conn != nil {
		_ = conn.Close()
	}
	return peer, nil
}

// checkAPIChain verifies the certificate of the api (peer[0], with peer[1:] as its chain) against roots, for host,
// at now. It returns nil when it is trusted.
func checkAPIChain(peer []*x509.Certificate, roots *x509.CertPool, host string, now time.Time) error {
	if roots == nil {
		return fmt.Errorf("%s not found in --pki-dir", pki.CLICAFile)
	}
	intermediates := x509.NewCertPool()
	for _, c := range peer[1:] {
		intermediates.AddCert(c)
	}
	_, err := peer[0].Verify(x509.VerifyOptions{
		DNSName:       host,
		Roots:         roots,
		Intermediates: intermediates,
		CurrentTime:   now,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	return err
}

func runPKIList(c *cobra.Command, _ []string) error {
	switch pkiListOpts.format {
	case listFormatText, listFormatJSON:
	default:
		return fmt.Errorf("unrecognised output format %s", pkiListOpts.format)
	}
	files, err := cliTLSFiles()
	if err != nil {
		return err
	}
	now := time.Now()
	dir := filepath.Dir(files.CertFile)
	certs, roots, err := listLocal(dir, now)
	if err != nil {
		return err
	}
	result := pkiList{Certificates: certs}
	var apiErr error
	if !pkiListOpts.local {
		host := viper.GetString(flagHost)
		address := net.JoinHostPort(host, strconv.Itoa(int(viper.GetUint16(flagPort))))
		result.API = &apiCheck{Address: address}
		peer, dialErr := apiCertificates(c.Context(), address, host, viper.GetDuration(flagTimeout))
		switch {
		case dialErr != nil:
			apiErr = fmt.Errorf("failed to get the certificates of the api at %s: %w", address, dialErr)
		case len(peer) == 0:
			apiErr = fmt.Errorf("the api at %s sent no certificate", address)
		default:
			for _, p := range peer {
				result.Certificates = append(result.Certificates, newListedCert(sourceAPI, "", p, now))
			}
			if verifyErr := checkAPIChain(peer, roots, host, now); verifyErr != nil {
				result.API.Error = verifyErr.Error()
			} else {
				result.API.Trusted = true
			}
		}
		if apiErr != nil {
			result.API.Error = apiErr.Error()
		}
	}
	if err = printPKIList(result); err != nil {
		return err
	}
	// the local certificates are listed: a missing api still fails the command, for scripts
	return apiErr
}

func printPKIList(result pkiList) error {
	if pkiListOpts.format == listFormatJSON {
		output, err := json.MarshalIndent(result, "", "\t")
		if err != nil {
			return fmt.Errorf("failed to marshal the certificates: %w", err)
		}
		stdout("%s", output)
		return nil
	}
	tabOut := newTabPrinter()
	tabOut.printf("SOURCE\tFILE\tCOMMON NAME\tISSUER\tSERIAL\tEXPIRES\tSTATUS\n")
	for _, c := range result.Certificates {
		file := c.File
		if file == "" {
			file = "-"
		}
		tabOut.printf("%s\t%s\t%s\t%s\t%s\t%s\t%s\n", c.Source, file, c.CommonName, c.Issuer, c.Serial,
			c.NotAfter.Local().Format(listTimeFormat), describeStatus(c))
	}
	tabOut.flush()
	if tabOut.err != nil {
		return tabOut.err
	}
	switch {
	case result.API == nil || !hasSource(result.Certificates, sourceAPI):
		// --local, or the api couldn't be reached: the command returns that error
	case result.API.Trusted:
		stdout("")
		stdout("The certificate of the api at %s is trusted by the root of the cli", result.API.Address)
	default:
		stdout("")
		stdout("The certificate of the api at %s is NOT trusted by the root of the cli: %s", result.API.Address,
			result.API.Error)
	}
	return nil
}

// describeStatus returns the status of c, with the days left while it is valid
func describeStatus(c listedCert) string {
	if c.Status != statusValid && c.Status != statusExpiring {
		return c.Status
	}
	days := int(time.Until(c.NotAfter).Hours() / hoursPerDay)
	return fmt.Sprintf("%s (%dd left)", c.Status, days)
}

func hasSource(certs []listedCert, source string) bool {
	for _, c := range certs {
		if c.Source == source {
			return true
		}
	}
	return false
}
