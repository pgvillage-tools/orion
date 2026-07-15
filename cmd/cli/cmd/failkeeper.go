// Copyright 2026 PgVillage
// Copyright 2018 Sorint.lab
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

	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
	"github.com/pgvillage-tools/orion/internal/logging"
	client "github.com/pgvillage-tools/orion/pkg/api_client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type failKeeperOptions struct {
	Host string `mapstructure:"host"`
	Port uint16 `mapstructure:"port"`
	TLS  bool   `mapstructure:"tls"`
}

var failKeeperOpts failKeeperOptions

// revive:disable

var failKeeperCmd = &cobra.Command{
	Use:   "failkeeper [keeper uid]",
	Short: `Force keeper as "temporarily" failed. The sentinel will compute a new clusterdata considering it as failed and then restore its state to the real one.`,
	Long:  `Force keeper as "temporarily" failed. It's just a one shot operation, the sentinel will compute a new clusterdata considering the keeper as failed and then restore its state to the real one. For example, if the force failed keeper is a master, the sentinel will try to elect a new master. If no new master can be elected, the force failed keeper, if really healthy, will be re-elected as master`,
	Run:   failKeeper,
}

//revive:enable

func init() {
	_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
	failKeeperCmd.PersistentFlags().BoolVarP(&failKeeperOpts.TLS, "tls", "t", true, "use tls")
	if err := viper.BindPFlag("tls", failKeeperCmd.PersistentFlags().Lookup("tls")); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
	failKeeperCmd.PersistentFlags().Uint16VarP(&failKeeperOpts.Port, "port", "p", defaultAPIPort,
		"protocol for connecting to the api")
	if err := viper.BindPFlag("port", failKeeperCmd.PersistentFlags().Lookup("port")); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
	failKeeperCmd.PersistentFlags().StringVarP(&failKeeperOpts.Host, "host", "H", defaultAPIIP,
		"hostname or ip for connecting to the api")
	if err := viper.BindPFlag("host", failKeeperCmd.PersistentFlags().Lookup("host")); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
	CmdCLI.AddCommand(failKeeperCmd)
}

func failKeeper(_ *cobra.Command, args []string) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	ctx, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
	defer cancelFunc()
	if len(args) > 1 {
		logger.Fatal().Msg("too many arguments")
	}
	if len(args) == 0 {
		logger.Fatal().Msg("keeper uid required")
	}
	keeperID := args[0]

	p := endpoints.HTTPS
	if !failKeeperOpts.TLS {
		p = endpoints.HTTP
	}
	apiClient := client.NewConnection(p, failKeeperOpts.Host, failKeeperOpts.Port)

	httpCode, putErr := apiClient.PutFailKeeper(keeperID)
	if putErr != nil {
		logger.Fatal().
			AnErr("error", putErr).
			Int("http_code", httpCode).
			Str("source", initOpts.file).
			Msg("failed to fail the keeper")
	}
}
