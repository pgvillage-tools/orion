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

	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
	"github.com/pgvillage-tools/orion/internal/logging"
	client "github.com/pgvillage-tools/orion/pkg/api_client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type promoteOptions struct {
	Host    string        `mapstructure:"host"`
	Port    uint16        `mapstructure:"port"`
	TLS     bool          `mapstructure:"tls"`
	Timeout time.Duration `mapstructure:"timeout"`
}

var promoteOpts promoteOptions

var cmdPromote = &cobra.Command{
	Use:   "promote",
	Run:   promote,
	Short: "Promotes a standby cluster to a primary cluster",
}

func init() {
	_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
	cmdPromote.PersistentFlags().BoolVarP(&promoteOpts.TLS, "tls", "t", true, "use tls")
	if err := viper.BindPFlag("tls", cmdPromote.PersistentFlags().Lookup("tls")); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
	cmdPromote.PersistentFlags().Uint16VarP(&promoteOpts.Port, "port", "p", defaultAPIPort,
		"protocol for connecting to the api")
	if err := viper.BindPFlag("port", cmdPromote.PersistentFlags().Lookup("port")); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
	cmdPromote.PersistentFlags().StringVarP(&promoteOpts.Host, "host", "H", defaultAPIIP,
		"hostname or ip for connecting to the api")
	if err := viper.BindPFlag("host", cmdPromote.PersistentFlags().Lookup("host")); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
	cmdPromote.PersistentFlags().DurationVarP(&promoteOpts.Timeout, flagTimeout, "T", defaultTimeout,
		"connection timeout for api endpoint")
	if err := viper.BindPFlag(flagTimeout, cmdPromote.PersistentFlags().Lookup(flagTimeout)); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}

	CmdCLI.AddCommand(cmdPromote)
}

func promote(_ *cobra.Command, args []string) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	ctx, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
	defer cancelFunc()
	if len(args) > 0 {
		logger.Fatal().Msg("too many arguments")
	}

	p := endpoints.HTTPS
	if !viper.GetBool(flagTLS) {
		p = endpoints.HTTP
	}
	apiClient := client.NewConnection(p, viper.GetString(flagHost), viper.GetUint16(flagPort),
		viper.GetDuration(flagTimeout))

	httpCode, putErr := apiClient.PutPromoteReplicaSet()
	if putErr != nil {
		logger.Fatal().
			AnErr("error", putErr).
			Int("http_code", httpCode).
			Msg("promote failed")
	}
}
