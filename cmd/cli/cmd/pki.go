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
	"strings"
	"time"

	"github.com/pgvillage-tools/orion/internal/pki"
	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const defaultAPIPKIDir = "~/.orion/api"

var cmdPKI = &cobra.Command{
	Use:   "pki",
	Short: "Manage the certificates of Orion",
}

var cmdPKIInit = &cobra.Command{
	Use:   "init",
	RunE:  runPKIInit,
	Args:  cobra.NoArgs,
	Short: "Create the CA, the api server certificate and the cli client certificate",
	Long: `Create a root CA, a server intermediate with the server certificate of the api, and an api client
intermediate with the client certificate of the cli.

The cli files (its client certificate, and the certificates and private keys of the root and the
intermediates) are stored in --pki-dir. The api files are stored in --api-dir: copy them to the
--pki-dir of the api.

The api certificate is valid for localhost, 127.0.0.1, ::1, the configured api host (--host) and
the names in --api-san.`,
}

type pkiInitOptions struct {
	apiDir               string
	apiSANs              []string
	keyAlgorithm         string
	rootValidity         string
	intermediateValidity string
	leafValidity         string
	force                bool
}

var pkiInitOpts pkiInitOptions

func init() {
	flags := cmdPKIInit.Flags()
	flags.StringVar(&pkiInitOpts.apiDir, "api-dir", defaultAPIPKIDir, "directory to store the api files in")
	flags.StringSliceVar(&pkiInitOpts.apiSANs, "api-san", nil,
		"extra name or ip address for the api certificate (can be repeated)")
	var algorithms []string
	for _, a := range oriontls.KeyAlgorithms() {
		algorithms = append(algorithms, string(a))
	}
	flags.StringVar(&pkiInitOpts.keyAlgorithm, "key-algorithm", string(oriontls.DefaultKeyAlgorithm),
		"algorithm of the private keys ("+strings.Join(algorithms, ", ")+")")
	flags.StringVar(&pkiInitOpts.rootValidity, "root-validity", "5y", "validity of the root (e.g. 5y, 365d, 8760h)")
	flags.StringVar(&pkiInitOpts.intermediateValidity, "intermediate-validity", "3y", "validity of the intermediates")
	flags.StringVar(&pkiInitOpts.leafValidity, "leaf-validity", "1y",
		"validity of the server and client certificates")
	flags.BoolVar(&pkiInitOpts.force, "force", false, "overwrite existing files")
	flags.StringP(flagHost, "H", defaultAPIIP, "hostname or ip of the api (added to the api certificate)")
	cmdPKI.AddCommand(cmdPKIInit)
	CmdCLI.AddCommand(cmdPKI)
}

func (o pkiInitOptions) initOptions(cliDir, host string) (pki.InitOptions, error) {
	opts := pki.InitOptions{
		CLIDir:       cliDir,
		APISANs:      append([]string{host}, o.apiSANs...),
		KeyAlgorithm: oriontls.KeyAlgorithm(o.keyAlgorithm),
		Force:        o.force,
	}
	var err error
	if opts.APIDir, err = pki.ExpandHome(o.apiDir); err != nil {
		return opts, err
	}
	for _, v := range []struct {
		name   string
		value  string
		target *time.Duration
	}{
		{"root-validity", o.rootValidity, &opts.RootExpiry},
		{"intermediate-validity", o.intermediateValidity, &opts.IntermediateExpiry},
		{"leaf-validity", o.leafValidity, &opts.LeafExpiry},
	} {
		if *v.target, err = pki.ParseValidity(v.value); err != nil {
			return opts, fmt.Errorf("--%s: %w", v.name, err)
		}
	}
	return opts, nil
}

func runPKIInit(c *cobra.Command, _ []string) error {
	files, err := cliTLSFiles()
	if err != nil {
		return err
	}
	host := viper.GetString(flagHost)
	if c.Flags().Changed(flagHost) {
		host, _ = c.Flags().GetString(flagHost)
	}
	opts, err := pkiInitOpts.initOptions(filepath.Dir(files.CertFile), host)
	if err != nil {
		return fmt.Errorf("invalid options: %w", err)
	}
	if _, err = pki.Init(opts); err != nil {
		return fmt.Errorf("failed to create the certificates: %w", err)
	}
	stdout("Created the CA and the cli client certificate in %s", opts.CLIDir)
	stdout("Created the api certificates in %s: copy them to the --pki-dir of the api", opts.APIDir)
	stdout("The private keys of the CA are in %s: keep them safe, and away from the api",
		filepath.Join(opts.CLIDir, pki.CLICADir))
	return nil
}
