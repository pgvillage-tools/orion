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

	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
	"github.com/pgvillage-tools/orion/internal/logging"
	client "github.com/pgvillage-tools/orion/pkg/api_client"
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
	file string
	Host string `mapstructure:"host"`
	Port uint16 `mapstructure:"port"`
	TLS  bool   `mapstructure:"tls"`
}

var connOpts connectOptions

func init() {
	cmdConnect.PersistentFlags().BoolVarP(&connOpts.TLS, "tls", "t", true, "use tls")
	viper.BindPFlag("tls", cmdConnect.PersistentFlags().Lookup("tls"))
	cmdConnect.PersistentFlags().Uint16VarP(&connOpts.Port, "port", "p", 8443, "protocol for connecting to the api")
	viper.BindPFlag("port", cmdConnect.PersistentFlags().Lookup("port"))
	cmdConnect.PersistentFlags().StringVarP(&connOpts.Host, "host", "H", "127.0.0.1", "hostname or ip for connecting to the api")
	viper.BindPFlag("host", cmdConnect.PersistentFlags().Lookup("host"))
	CmdCLI.AddCommand(cmdConnect)
}

func connectCluster(_ *cobra.Command, args []string) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	ctx, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
	defer cancelFunc()

	p := endpoints.HTTPS
	if !connOpts.TLS {
		p = endpoints.HTTP
	}
	apiClient := client.NewConnection(p, connOpts.Host, connOpts.Port)

	if httpCode, healthErr := apiClient.Healthy(); healthErr != nil {
		logger.Fatal().
			AnErr("error", healthErr).
			Int("http_code", httpCode).
			Str("url", apiClient.EndpointUrl(endpoints.HealthEndPoint)).
			Msg("connecting to the api failed")
	}
	if writeErr := viper.WriteConfig(); writeErr != nil {
		logger.Fatal().
			AnErr("error", writeErr).
			Msg("updating cli config failed")
	}
}
