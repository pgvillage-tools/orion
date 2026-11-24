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
	"crypto/hpke"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pgvillage-tools/orion/internal/pki"

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
	// bootstrapSwitchWait is how long the cli waits for the api to switch to normal mode after the hand-over
	bootstrapSwitchWait = 30 * time.Second
	// bootstrapPollInterval is how often the cli checks the mode of the api meanwhile
	bootstrapPollInterval = 200 * time.Millisecond
)

var cmdBootstrap = &cobra.Command{
	Use:   "bootstrap",
	RunE:  runBootstrap,
	Args:  cobra.NoArgs,
	Short: "Bootstrap an api that started without certificates",
	Long: `Bootstrap an api that started without certificates (bootstrap mode).

The cli checks on the probe port (--probe-port) that the api is in bootstrap mode, and connects to
the api port (--host, --port). The api has a temporary self-signed certificate, which the cli can't
verify: instead, the api and the cli prove to each other that they know the bootstrap secret,
without sending it.

The bootstrap secret is the one the api logged at its start, or the one it was given. It is read from
--bootstrap-secret-file, else from ORIONCLI_BOOTSTRAP_SECRET, else the cli asks for it on the terminal.

On the first bootstrap, the cli creates its bootstrap key pair in --pki-dir (bootstrap.key and
bootstrap.pub): the api encrypts the CA keys and the private key of the cli certificate to it. The
private key is encrypted with the passphrase of the CA keys (--ca-passphrase-file, else
ORIONCLI_CA_PASSPHRASE, else a prompt).

Then the api creates the root, the intermediates, its own certificate (with the names in --api-san
too) and the client certificate of the cli, and hands them over. The cli checks them, and stores them
in --pki-dir like ` + "`orion-cli pki init`" + ` does, with the CA keys encrypted with the passphrase. It
doesn't overwrite existing files. The api then switches to normal mode, and the cli confirms with its
new client certificate, after which the api deletes the archive with the CA keys.`,
}

var cmdBootstrapConfirm = &cobra.Command{
	Use:   "confirm",
	RunE:  runBootstrapConfirm,
	Args:  cobra.NoArgs,
	Short: "Confirm a bootstrap with the client certificate of the cli",
	Long: `Confirm a bootstrap with the client certificate of the cli (over mTLS), after which the api
deletes the archive with the CA keys. ` + "`orion-cli bootstrap`" + ` does this itself; use this command when that
step failed. It can be repeated.`,
}

type bootstrapOptions struct {
	secretFile       string
	caPassphraseFile string
	probePort        uint16
	apiSANs          []string
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
	flags.StringSliceVar(&bootstrapOpts.apiSANs, "api-san", nil,
		"extra name or address for the api certificate, on top of the ones the api detects (repeatable)")
	cmdBootstrap.AddCommand(cmdBootstrapConfirm)
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

// bootstrapKeyPair returns the bootstrap private key in dir and the passphrase of the CA keys: it creates the key
// pair (with a new passphrase) when it doesn't exist yet, and decrypts the existing one otherwise
func bootstrapKeyPair(dir, caPassphraseFile string) (hpke.PrivateKey, []byte, error) {
	exists, err := bootstrap.KeyPairExists(dir)
	if err != nil {
		return nil, nil, err
	}
	if exists {
		passphrase, passErr := existingCAPassphrase(caPassphraseFile)
		if passErr != nil {
			return nil, nil, passErr
		}
		priv, readErr := bootstrap.ReadPrivateKey(dir, passphrase)
		if readErr != nil {
			return nil, nil, fmt.Errorf("failed to read the bootstrap key pair in %s: %w", dir, readErr)
		}
		return priv, passphrase, nil
	}
	passphrase, err := newCAPassphrase(caPassphraseFile)
	if err != nil {
		return nil, nil, err
	}
	priv, err := bootstrap.GenerateKey()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create the bootstrap key pair: %w", err)
	}
	if err = bootstrap.WriteKeyPair(dir, priv, passphrase); err != nil {
		return nil, nil, fmt.Errorf("failed to store the bootstrap key pair: %w", err)
	}
	stdout("Created the bootstrap key pair in %s", dir)
	return priv, passphrase, nil
}

// waitForNormalMode waits until the api reports normal mode on its probe port
func waitForNormalMode(ctx context.Context, probe client.Connection) error {
	deadline := time.Now().Add(bootstrapSwitchWait)
	for {
		mode, _, err := probe.Mode()
		if err == nil && mode.Mode == endpoints.ModeNormal {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the api didn't switch to normal mode within %s (mode %q, error %v)",
				bootstrapSwitchWait, mode.Mode, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(bootstrapPollInterval):
		}
	}
}

func runBootstrap(c *cobra.Command, _ []string) error {
	host, port, timeout := viper.GetString(flagHost), viper.GetUint16(flagPort), viper.GetDuration(flagTimeout)

	probe := client.NewConnection(endpoints.HTTP, host, bootstrapOpts.probePort, timeout)
	mode, _, err := probe.Mode()
	if err != nil {
		return fmt.Errorf("failed to get the mode of the api on its probe port %d: %w", bootstrapOpts.probePort, err)
	}
	if mode.Mode != endpoints.ModeBootstrap {
		return fmt.Errorf("the api is in %s mode, not in bootstrap mode: it has its certificates already", mode.Mode)
	}
	files, err := cliTLSFiles()
	if err != nil {
		return err
	}
	dir := filepath.Dir(files.CertFile)
	// before anything is asked or sent: the bootstrap stores files that must not exist yet
	if err = pki.CheckNoCLIFiles(dir); err != nil {
		return err
	}

	secret, err := bootstrapSecret(bootstrapOpts.secretFile)
	if err != nil {
		return err
	}
	priv, passphrase, err := bootstrapKeyPair(dir, bootstrapOpts.caPassphraseFile)
	if err != nil {
		return err
	}

	ctx := c.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	baseURL := "https://" + net.JoinHostPort(host, strconv.Itoa(int(port)))
	session, err := bootstrap.Start(ctx, bootstrap.NewHTTPClient(timeout), baseURL, secret)
	if err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}
	stdout("The api proved that it has the bootstrap secret")
	certs, keys, err := session.Archive(ctx, priv, bootstrapOpts.apiSANs)
	if err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}
	if err = pki.VerifyArchive(certs, keys, time.Now()); err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}
	if err = pki.StoreArchive(dir, certs, keys, passphrase); err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}
	stdout("Stored the certificates and the CA keys in %s", dir)
	if err = session.Stored(ctx); err != nil {
		return fmt.Errorf("the api didn't take the bootstrap over (run the bootstrap again after moving the files "+
			"in %s away): %w", dir, err)
	}
	if err = waitForNormalMode(ctx, probe); err != nil {
		return err
	}
	if err = confirmBootstrap(host, port); err != nil {
		return fmt.Errorf("%w (retry with `orion-cli bootstrap confirm`)", err)
	}
	return nil
}

// confirmBootstrap confirms the bootstrap to the api on host and port with the client certificate of the cli
func confirmBootstrap(host string, port uint16) error {
	apiClient, err := newAPIClient(host, port)
	if err != nil {
		return err
	}
	if httpCode, confirmErr := apiClient.ConfirmBootstrap(); confirmErr != nil {
		return apiError("confirming the bootstrap with the client certificate failed", httpCode, confirmErr)
	}
	stdout("The api is in normal mode and deleted the CA keys: the bootstrap is done")
	return nil
}

func runBootstrapConfirm(_ *cobra.Command, _ []string) error {
	return confirmBootstrap(viper.GetString(flagHost), viper.GetUint16(flagPort))
}
