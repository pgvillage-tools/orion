// Copyright 2026 PgVillage
// Copyright 2016 Sorint.lab
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
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cmdConnect = &cobra.Command{
	Use:   "connect",
	Run:   connectCluster,
	Short: "connect to an API",
}

// InitOptions is a struct which can contain initiation options
type connectOptions struct {
	file    string
	Host    string        `mapstructure:"host"`
	Port    uint16        `mapstructure:"port"`
	Timeout time.Duration `mapstructure:"timeout"`
}

var connOpts connectOptions

func init() {
	_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
	cmdConnect.PersistentFlags().Uint16VarP(&connOpts.Port, "port", "p", defaultAPIPort,
		"protocol for connecting to the api")
	if err := viper.BindPFlag("port", cmdConnect.PersistentFlags().Lookup("port")); err != nil {
		logger.Fatal().AnErr(logging.KeyError, err).Msg("")
	}
	cmdConnect.PersistentFlags().StringVarP(&connOpts.Host, "host", "H", defaultAPIIP,
		"hostname or ip for connecting to the api")
	if err := viper.BindPFlag("host", cmdConnect.PersistentFlags().Lookup("host")); err != nil {
		logger.Fatal().AnErr(logging.KeyError, err).Msg("")
	}
	cmdConnect.PersistentFlags().DurationVarP(&connOpts.Timeout, flagTimeout, "T", defaultTimeout,
		"connection timeout for api endpoint")
	if err := viper.BindPFlag(flagTimeout, cmdConnect.PersistentFlags().Lookup(flagTimeout)); err != nil {
		logger.Fatal().AnErr(logging.KeyError, err).Msg("")
	}
	CmdCLI.AddCommand(cmdConnect)
}

func connectCluster(_ *cobra.Command, args []string) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	ctx, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
	defer cancelFunc()

	apiClient := defaultAPIClient(ctx)

	identity, httpCode, whoAmIErr := apiClient.WhoAmI()
	if whoAmIErr != nil {
		logger.Fatal().
			AnErr(logging.KeyError, whoAmIErr).
			Int("http_code", httpCode).
			Str("url", apiClient.EndpointURL(endpoints.WhoAmIEndPoint)).
			Msg("connecting to the api failed")
	}
	stdout("Connected to the api as %s (%s)", identity.Name, identity.Method)
	if writeErr := viper.WriteConfig(); writeErr != nil {
		logger.Fatal().
			AnErr(logging.KeyError, writeErr).
			Msg("updating cli config failed")
	}
}
