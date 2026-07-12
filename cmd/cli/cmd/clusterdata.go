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

// Package cmd is a package which provides utilities that underly the specific command
package cmd

import (
	"context"
	"encoding/json"
	"io"
	"os"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
	"github.com/pgvillage-tools/orion/internal/logging"
	client "github.com/pgvillage-tools/orion/pkg/api_client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cmdClusterData = &cobra.Command{
	Use:   "clusterdata",
	Short: "Manage current cluster data",
}

type clusterdataReadOptions struct {
	pretty bool
	Host   string `mapstructure:"host"`
	Port   uint16 `mapstructure:"port"`
	TLS    bool   `mapstructure:"tls"`
}

var readClusterdataOpts clusterdataReadOptions

type clusterdataWriteOptions struct {
	file string
	Host string `mapstructure:"host"`
	Port uint16 `mapstructure:"port"`
	TLS  bool   `mapstructure:"tls"`
}

var writeClusterdataOpts clusterdataWriteOptions

var cmdReadClusterData = &cobra.Command{
	Use:   "read",
	Run:   readClusterdata,
	Short: "Retrieve the current cluster data",
}

var cmdWriteClusterData = &cobra.Command{
	Use:   "write",
	Run:   runWriteClusterdata,
	Short: "Write cluster data",
}

func init() {
	cmdReadClusterData.PersistentFlags().BoolVar(&readClusterdataOpts.pretty, "pretty", false, "pretty print")
	cmdReadClusterData.PersistentFlags().BoolVarP(&readClusterdataOpts.TLS, "tls", "t", true, "use tls")
	viper.BindPFlag("tls", cmdReadClusterData.PersistentFlags().Lookup("tls"))
	cmdReadClusterData.PersistentFlags().Uint16VarP(&readClusterdataOpts.Port, "port", "p", 8443, "protocol for connecting to the api")
	viper.BindPFlag("port", cmdReadClusterData.PersistentFlags().Lookup("port"))
	cmdReadClusterData.PersistentFlags().StringVarP(&readClusterdataOpts.Host, "host", "H", "127.0.0.1", "hostname or ip for connecting to the api")
	viper.BindPFlag("host", cmdReadClusterData.PersistentFlags().Lookup("host"))
	cmdClusterData.AddCommand(cmdReadClusterData)

	cmdWriteClusterData.PersistentFlags().StringVarP(&writeClusterdataOpts.file, "file", "f", "",
		"file containing the new cluster data")
	cmdWriteClusterData.PersistentFlags().BoolVarP(&writeClusterdataOpts.TLS, "tls", "t", true, "use tls")
	viper.BindPFlag("tls", cmdWriteClusterData.PersistentFlags().Lookup("tls"))
	cmdWriteClusterData.PersistentFlags().Uint16VarP(&writeClusterdataOpts.Port, "port", "p", 8443, "protocol for connecting to the api")
	viper.BindPFlag("port", cmdWriteClusterData.PersistentFlags().Lookup("port"))
	cmdWriteClusterData.PersistentFlags().StringVarP(&writeClusterdataOpts.file, "host", "H", "host", "host/ip for connecting to the api")
	viper.BindPFlag("host", cmdWriteClusterData.PersistentFlags().Lookup("host"))
	cmdClusterData.AddCommand(cmdWriteClusterData)

	CmdCLI.AddCommand(cmdClusterData)
}

func readClusterdata(_ *cobra.Command, _ []string) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	_, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
	p := endpoints.HTTPS
	if !readClusterdataOpts.TLS {
		p = endpoints.HTTP
	}
	apiClient := client.NewConnection(p, readClusterdataOpts.Host, readClusterdataOpts.Port)
	cd, httpCode, getClusterErr := apiClient.GetCluster()
	if getClusterErr != nil {
		logger.Fatal().
			AnErr("error", getClusterErr).
			Int("http return code", httpCode).
			Msg("failed to get clusterdata")
	}
	var clusterdataj []byte
	var marshalErr error
	if readClusterdataOpts.pretty {
		clusterdataj, marshalErr = json.MarshalIndent(cd, "", "\t")
		if marshalErr != nil {
			logger.Fatal().AnErr("error", marshalErr).Msg("failed to marshall clusterdata")
		}
	} else {
		clusterdataj, marshalErr = json.Marshal(cd)
		if marshalErr != nil {
			logger.Fatal().AnErr("error", marshalErr).Msg("failed to marshall clusterdata")
		}
	}
	stdout("%s", clusterdataj)
}

func runWriteClusterdata(_ *cobra.Command, args []string) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	ctx, logger := logging.GetLogComponent(ctx, logging.CmdComponent)

	cd := &apiv1.Data{}
	switch len(args) {
	case 1:
		encodingErr := json.Unmarshal([]byte(args[0]), cd)
		if encodingErr != nil {
			logger.Fatal().
				AnErr("error", encodingErr).
				Str("input", "argument").
				Str("spec", args[0]).
				Msg("invalid cluster data spec")
		}
	case 0:
		if writeClusterdataOpts.file != "" {
			var readErr error
			var data []byte
			if writeClusterdataOpts.file == "-" {
				data, readErr = io.ReadAll(os.Stdin)
			} else {
				data, readErr = os.ReadFile(writeClusterdataOpts.file)
			}
			if readErr != nil {
				logger.Fatal().
					AnErr("error", readErr).
					Str("source", writeClusterdataOpts.file).
					Msg("cannot read from stdin")
			}
			if encodingErr := json.Unmarshal(data, cd); encodingErr != nil {
				logger.Fatal().
					AnErr("error", encodingErr).
					Str("source", writeClusterdataOpts.file).
					Str("spec", string(data)).
					Msg("invalid cluster data spec")
			}
		}
	}

	p := endpoints.HTTPS
	if !writeClusterdataOpts.TLS {
		p = endpoints.HTTP
	}
	apiClient := client.NewConnection(p, readClusterdataOpts.Host, readClusterdataOpts.Port)
	httpCode, putClusterErr := apiClient.PutCluster(cd)
	if putClusterErr != nil {
		logger.Fatal().
			AnErr("error", putClusterErr).
			Int("http return code", httpCode).
			Msg("failed to get clusterdata")
	}
}
