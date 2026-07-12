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

	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
	"github.com/pgvillage-tools/orion/internal/logging"
	client "github.com/pgvillage-tools/orion/pkg/api_client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type deleteKeeperOptions struct {
	Host string `mapstructure:"host"`
	Port uint16 `mapstructure:"port"`
	TLS  bool   `mapstructure:"tls"`
}

var deleteKeeperOpts deleteKeeperOptions

var removeKeeperCmd = &cobra.Command{
	Use:   "removekeeper [keeper uid]",
	Short: "Removes keeper from cluster data",
	Run:   removeKeeper,
}

func init() {
	removeKeeperCmd.PersistentFlags().BoolVarP(&deleteKeeperOpts.TLS, "tls", "t", true, "use tls")
	viper.BindPFlag("tls", removeKeeperCmd.PersistentFlags().Lookup("tls"))
	removeKeeperCmd.PersistentFlags().Uint16VarP(&deleteKeeperOpts.Port, "port", "p", 8443, "protocol for connecting to the api")
	viper.BindPFlag("port", removeKeeperCmd.PersistentFlags().Lookup("port"))
	removeKeeperCmd.PersistentFlags().StringVarP(&deleteKeeperOpts.Host, "host", "H", "127.0.0.1", "hostname or ip for connecting to the api")
	viper.BindPFlag("host", removeKeeperCmd.PersistentFlags().Lookup("host"))
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

	p := endpoints.HTTPS
	if !deleteKeeperOpts.TLS {
		p = endpoints.HTTP
	}
	apiClient := client.NewConnection(p, deleteKeeperOpts.Host, deleteKeeperOpts.Port)

	httpCode, deleteErr := apiClient.PutFailKeeper(keeperID)
	if deleteErr != nil {
		logger.Fatal().
			AnErr("error", deleteErr).
			Int("http_code", httpCode).
			Str("source", initOpts.file).
			Msg("failed to fail the keeper")
	}
}
