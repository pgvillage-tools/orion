// Copyright 2026 PgVillage
// Copyright 2017 Sorint.lab
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
	"time"

	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type deleteKeeperOptions struct {
	Host    string        `mapstructure:"host"`
	Port    uint16        `mapstructure:"port"`
	Timeout time.Duration `mapstructure:"timeout"`
}

var deleteKeeperOpts deleteKeeperOptions

var removeKeeperCmd = &cobra.Command{
	Use:   "removekeeper [keeper uid]",
	Short: "Removes keeper from cluster data",
	Run:   removeKeeper,
}

func init() {
	_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
	removeKeeperCmd.PersistentFlags().Uint16VarP(&deleteKeeperOpts.Port, "port", "p", defaultAPIPort,
		"protocol for connecting to the api")
	if err := viper.BindPFlag("port", removeKeeperCmd.PersistentFlags().Lookup("port")); err != nil {
		logger.Fatal().AnErr(logging.KeyError, err).Msg("")
	}
	removeKeeperCmd.PersistentFlags().StringVarP(&deleteKeeperOpts.Host, "host", "H", defaultAPIIP,
		"hostname or ip for connecting to the api")
	if err := viper.BindPFlag("host", removeKeeperCmd.PersistentFlags().Lookup("host")); err != nil {
		logger.Fatal().AnErr(logging.KeyError, err).Msg("")
	}
	removeKeeperCmd.PersistentFlags().DurationVarP(&deleteKeeperOpts.Timeout, flagTimeout, "T", defaultTimeout,
		"connection timeout for api endpoint")
	if err := viper.BindPFlag(flagTimeout, removeKeeperCmd.PersistentFlags().Lookup(flagTimeout)); err != nil {
		logger.Fatal().AnErr(logging.KeyError, err).Msg("")
	}
	CmdCLI.AddCommand(removeKeeperCmd)
}

func removeKeeper(_ *cobra.Command, args []string) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	ctx, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
	defer cancelFunc()
	if len(args) > 1 {
		die("too many arguments")
	}

	if len(args) == 0 {
		die("keeper uid required")
	}

	keeperID := args[0]

	apiClient := defaultAPIClient(ctx)

	httpCode, deleteErr := apiClient.PutFailKeeper(keeperID)
	if deleteErr != nil {
		logger.Fatal().
			AnErr(logging.KeyError, deleteErr).
			Int("http_code", httpCode).
			Msg("failed to fail the keeper")
	}
}
