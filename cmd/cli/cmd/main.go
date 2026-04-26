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
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/cmd"
	"github.com/pgvillage-tools/orion/internal/consensus"
	"github.com/pgvillage-tools/orion/internal/flagutil"
	"github.com/pgvillage-tools/orion/internal/logging"

	"github.com/spf13/cobra"
)

const (
	maxRetries = 3
)

// CmdCLI defines a cobra command to execute when running orion
var CmdCLI = &cobra.Command{
	Use:     "orion",
	Short:   "orion command line interface",
	Version: cmd.Version,
	PersistentPreRun: func(c *cobra.Command, _ []string) {
		if c.Name() != "orion" && c.Name() != "version" {
			if err := cmd.CheckCommonConfig(&cfg.CommonConfig); err != nil {
				die("%s", err.Error())
			}
		}
	},
	// just defined to make --version work
	Run: func(c *cobra.Command, _ []string) { _ = c.Help() },
}

type config struct {
	cmd.CommonConfig
}

var cfg config

func init() {
	cfg.IsCLI = true
	cmd.AddCommonFlags(CmdCLI, &cfg.CommonConfig)
}

var cmdVersion = &cobra.Command{
	Use: "version",
	Run: func(_ *cobra.Command, _ []string) {
		stdout("orion version %s", cmd.Version)
	},
	Short: "Display the version",
}

func init() {
	CmdCLI.AddCommand(cmdVersion)
}

// Execute is run when orion is executed
func Execute() {
	_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
	if err := flagutil.SetFlagsFromEnv(CmdCLI.PersistentFlags(), "ORIONCLI"); err != nil {
		logger.Fatal().AnErr("err", err).Msg("")
	}
	if err := CmdCLI.Execute(); err != nil {
		logger.Fatal().AnErr("err", err).Msg("")
	}
}

func stderr(format string, a ...any) {
	out := fmt.Sprintf(format, a...)
	fmt.Fprintln(os.Stderr, strings.TrimSuffix(out, "\n"))
}

func stdout(format string, a ...any) {
	out := fmt.Sprintf(format, a...)
	if _, err := fmt.Fprintln(os.Stdout, strings.TrimSuffix(out, "\n")); err != nil {
		log.Fatalf("failed to write to stdout: %v", err)
	}
}

func die(format string, a ...any) {
	stderr(format, a...)
	os.Exit(1)
}

func getClusterData(e consensus.Store) (*cluster.Data, *consensus.KVPair, error) {
	cd, pair, err := e.GetClusterData(context.TODO())
	if err != nil {
		return nil, nil, fmt.Errorf("cannot get cluster data: %v", err)
	}
	if cd == nil {
		return nil, nil, fmt.Errorf("nil cluster data: %v", err)
	}
	if cd.FormatVersion != cluster.CurrentCDFormatVersion {
		return nil, nil, fmt.Errorf("unsupported cluster data format version %d", cd.FormatVersion)
	}
	if err := cd.Cluster.Spec.Validate(); err != nil {
		return nil, nil, fmt.Errorf("clusterdata validation failed: %v", err)
	}
	return cd, pair, nil
}

func askConfirmation(message string) (bool, error) {
	in := bufio.NewReader(os.Stdin)
	for {
		if _, err := fmt.Fprint(os.Stdout, message); err != nil {
			log.Fatalf("failed to print to stdout: %v", err)
		}
		input, err := in.ReadString('\n')
		if err != nil {
			return false, fmt.Errorf("error reading input: %v", err)
		}
		switch input {
		case "yes\n":
			return true, nil
		case "no\n":
			return false, nil
		default:
			stdout("Please enter 'yes' or 'no'")
		}
	}
}
