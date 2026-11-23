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
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	client "github.com/pgvillage-tools/orion/pkg/api_client"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/pgvillage-tools/orion/pkg/bootstrap"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	// envBootstrapSecret holds the bootstrap secret, for scripts (a terminal is prompted)
	envBootstrapSecret = "ORIONCLI_BOOTSTRAP_SECRET"
	// defaultProbePort is the default probe port of the api
	defaultProbePort = 8080
)

var cmdBootstrap = &cobra.Command{
	Use:   "bootstrap",
	RunE:  runBootstrap,
	Args:  cobra.NoArgs,
	Short: "Bootstrap an api that started without certificates",
	// bootstrap mode isn't enabled in the api yet (#323 enables it)
	Hidden: true,
	Long: `Bootstrap an api that started without certificates (bootstrap mode).

The cli checks on the probe port (--probe-port) that the api is in bootstrap mode, and connects to
the api port (--host, --port). The api has a temporary self-signed certificate, which the cli can't
verify: instead, the api and the cli prove to each other that they know the bootstrap secret,
without sending it.

The bootstrap secret is the one the api logged at its start, or the one it was given. It is read from
--bootstrap-secret-file, else from ORIONCLI_BOOTSTRAP_SECRET, else the cli asks for it on the terminal.

On the first bootstrap, the cli creates its bootstrap key pair in --pki-dir (bootstrap.key and
bootstrap.pub): the api encrypts the certificates and the CA keys to it. The private key is encrypted
with the passphrase of the CA keys (--ca-passphrase-file, else ORIONCLI_CA_PASSPHRASE, else a prompt).

Not yet: receiving the certificates and the CA keys from the api. This version stops after the
proofs.`,
}

type bootstrapOptions struct {
	secretFile       string
	caPassphraseFile string
	probePort        uint16
}

var bootstrapOpts bootstrapOptions

func init() {
	flags := cmdBootstrap.Flags()
	flags.StringVar(&bootstrapOpts.secretFile, "bootstrap-secret-file", "",
		"file with the bootstrap secret (- for stdin; default: "+envBootstrapSecret+", else a prompt)")
	flags.StringVar(&bootstrapOpts.caPassphraseFile, "ca-passphrase-file", "",
		"file with the passphrase for the CA keys and the bootstrap key (- for stdin; default: "+envCAPassphrase+
			", else a prompt)")
	flags.Uint16Var(&bootstrapOpts.probePort, "probe-port", defaultProbePort, "probe port of the api")
	CmdCLI.AddCommand(cmdBootstrap)
}

// bootstrapSecret returns the bootstrap secret: from file (when set), else from ORIONCLI_BOOTSTRAP_SECRET, else it
// prompts for it on the terminal
func bootstrapSecret(file string) ([]byte, error) {
	var (
		secret []byte
		source string
	)
	switch {
	case file != "":
		data, err := readInput(file)
		if err != nil {
			return nil, err
		}
		secret, source = []byte(strings.TrimSpace(string(data))), file
	case os.Getenv(envBootstrapSecret) != "":
		secret, source = []byte(os.Getenv(envBootstrapSecret)), envBootstrapSecret
	case stdinIsTerminal():
		var err error
		if secret, err = promptPassphrase("Bootstrap secret of the api: "); err != nil {
			return nil, err
		}
		source = "the terminal"
	default:
		return nil, fmt.Errorf("the bootstrap secret is required: set --bootstrap-secret-file or %s, or run in a "+
			"terminal", envBootstrapSecret)
	}
	// a shorter secret can't be the one of the api: don't spend one of its few tries on it
	if len(secret) < bootstrap.MinSecretLength {
		return nil, fmt.Errorf("the bootstrap secret (from %s) has at least %d characters", source,
			bootstrap.MinSecretLength)
	}
	return secret, nil
}

// ensureBootstrapKeyPair creates the bootstrap key pair in dir when it doesn't exist yet
func ensureBootstrapKeyPair(dir, caPassphraseFile string) error {
	exists, err := bootstrap.KeyPairExists(dir)
	if err != nil || exists {
		return err
	}
	passphrase, err := newCAPassphrase(caPassphraseFile)
	if err != nil {
		return err
	}
	priv, err := bootstrap.GenerateKey()
	if err != nil {
		return fmt.Errorf("failed to create the bootstrap key pair: %w", err)
	}
	if err = bootstrap.WriteKeyPair(dir, priv, passphrase); err != nil {
		return fmt.Errorf("failed to store the bootstrap key pair: %w", err)
	}
	stdout("Created the bootstrap key pair in %s", dir)
	return nil
}

func runBootstrap(c *cobra.Command, _ []string) error {
	host, port, timeout := viper.GetString(flagHost), viper.GetUint16(flagPort), viper.GetDuration(flagTimeout)

	mode, _, err := client.NewConnection(endpoints.HTTP, host, bootstrapOpts.probePort, timeout).Mode()
	if err != nil {
		return fmt.Errorf("failed to get the mode of the api on its probe port %d: %w", bootstrapOpts.probePort, err)
	}
	if mode.Mode != endpoints.ModeBootstrap {
		return fmt.Errorf("the api is in %s mode, not in bootstrap mode: it has its certificates already", mode.Mode)
	}

	secret, err := bootstrapSecret(bootstrapOpts.secretFile)
	if err != nil {
		return err
	}
	files, err := cliTLSFiles()
	if err != nil {
		return err
	}
	if err = ensureBootstrapKeyPair(filepath.Dir(files.CertFile), bootstrapOpts.caPassphraseFile); err != nil {
		return err
	}

	ctx := c.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	baseURL := "https://" + net.JoinHostPort(host, strconv.Itoa(int(port)))
	if _, err = bootstrap.Start(ctx, bootstrap.NewHTTPClient(timeout), baseURL, secret); err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}
	stdout("The api proved that it has the bootstrap secret")
	stdout("Not yet: receiving the certificates from the api; this version stops here")
	return nil
}
