// Copyright 2026 PgVillage
// Copyright 2015 Sorint.lab
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
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/pgvillage-tools/orion/cmd"
	"github.com/pgvillage-tools/orion/internal/flagutil"
	"github.com/pgvillage-tools/orion/internal/logging"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	readWrite      = 0755
	defaultAPIPort = 8443
	defaultAPIIP   = "127.0.0.1"
	defaultTimeout = time.Second * 30
)

// CmdCLI defines a cobra command to execute when running orion
var CmdCLI = &cobra.Command{
	Use:     "orion-cli",
	Short:   "orion command line interface",
	Version: cmd.Version,
	// Errors are returned by the commands (RunE) and reported once, by Execute
	SilenceErrors: true,
	PersistentPreRunE: func(c *cobra.Command, _ []string) error {
		// The flags and arguments are fine once we get here: from now on an error isn't a usage error, so don't
		// print the usage with it
		c.SilenceUsage = true
		return initConfig()
	},
	// just defined to make --version work
	Run: func(c *cobra.Command, _ []string) { _ = c.Help() },
}

// The cli only talks to the api: unlike the other components, it has no store options (--cluster-name,
// --store-backend, ...).
func init() {
	flags := CmdCLI.PersistentFlags()
	flags.String(flagPKIDir, defaultCLIPKIDir, "directory with the client certificate of the cli, and the CA")
	// The connection to the api, for every command. They are defined (and bound to viper) once, on the root: a flag
	// bound to a viper key per command made only the last bound one work.
	flags.StringP(flagHost, "H", defaultAPIIP, "hostname or ip of the api")
	flags.Uint16P(flagPort, "p", defaultAPIPort, "port of the api")
	flags.DurationP(flagTimeout, "T", defaultTimeout, "timeout for requests to the api")
	for _, name := range []string{flagPKIDir, flagHost, flagPort, flagTimeout} {
		if err := viper.BindPFlag(name, flags.Lookup(name)); err != nil {
			log.Fatal(err)
		}
	}
}

var cmdVersion = &cobra.Command{
	Use: "version",
	Run: func(_ *cobra.Command, _ []string) {
		stdout("orion-cli version %s", cmd.Version)
	},
	Short: "Display the version",
}

func init() {
	CmdCLI.AddCommand(cmdVersion)
}

// Execute is run when orion is executed. It is the only place where the cli exits with an error: the commands return
// theirs.
func Execute() {
	_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
	if err := flagutil.SetFlagsFromEnv(CmdCLI.PersistentFlags(), "ORIONCLI"); err != nil {
		logger.Fatal().Msg(err.Error())
	}
	if err := CmdCLI.Execute(); err != nil {
		logger.Fatal().Msg(err.Error())
	}
}

// stderr writes a line to the error output of the cli (os.Stderr, unless a test set another one)
func stderr(format string, a ...any) {
	out := fmt.Sprintf(format, a...)
	// nowhere left to report a failed write to stderr
	_, _ = fmt.Fprintln(CmdCLI.ErrOrStderr(), strings.TrimSuffix(out, "\n"))
}

// stdout writes a line to the output of the cli (os.Stdout, unless a test set another one)
func stdout(format string, a ...any) {
	out := fmt.Sprintf(format, a...)
	if _, err := fmt.Fprintln(CmdCLI.OutOrStdout(), strings.TrimSuffix(out, "\n")); err != nil {
		log.Fatalf("failed to write to stdout: %v", err)
	}
}

// apiError is the error of a failed api request: what failed, why, and the http code when the api answered
func apiError(what string, httpCode int, err error) error {
	if httpCode > 0 {
		return fmt.Errorf("%s: %w (http %d)", what, err, httpCode)
	}
	return fmt.Errorf("%s: %w", what, err)
}

// readInput returns the contents of file, or of the input of the cli (os.Stdin) for "-"
func readInput(file string) ([]byte, error) {
	if file == "-" {
		data, err := io.ReadAll(CmdCLI.InOrStdin())
		if err != nil {
			return nil, fmt.Errorf("cannot read from stdin: %w", err)
		}
		return data, nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", file, err)
	}
	return data, nil
}
