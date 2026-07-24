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
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
	"github.com/pgvillage-tools/orion/internal/logging"
	client "github.com/pgvillage-tools/orion/pkg/api_client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cmdInit = &cobra.Command{
	Use:   "init",
	Run:   initCluster,
	Short: "Initialize a new cluster",
}

// InitOptions is a struct which can contain initiation options
type InitOptions struct {
	file    string
	Host    string        `mapstructure:"host"`
	Port    uint16        `mapstructure:"port"`
	TLS     bool          `mapstructure:"tls"`
	Timeout time.Duration `mapstructure:"timeout"`
}

var initOpts InitOptions

func init() {
	_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
	cmdInit.PersistentFlags().StringVarP(&initOpts.file, flagFile, "f", "-", "file to read as input, use - for stdin")
	cmdInit.PersistentFlags().BoolVarP(&initOpts.TLS, flagTLS, "t", true, "use tls")
	if err := viper.BindPFlag("tls", cmdInit.PersistentFlags().Lookup(flagTLS)); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
	cmdInit.PersistentFlags().Uint16VarP(&initOpts.Port, flagPort, "p", defaultAPIPort,
		"protocol for connecting to the api")
	if err := viper.BindPFlag("port", cmdInit.PersistentFlags().Lookup(flagPort)); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
	cmdInit.PersistentFlags().StringVarP(&initOpts.Host, flagHost, "H", defaultAPIIP,
		"hostname or ip for connecting to the api")
	if err := viper.BindPFlag("host", cmdInit.PersistentFlags().Lookup(flagHost)); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
	cmdInit.PersistentFlags().DurationVarP(&initOpts.Timeout, flagTimeout, "T", defaultTimeout,
		"connection timeout for api endpoint")
	if err := viper.BindPFlag(flagTimeout, cmdInit.PersistentFlags().Lookup(flagTimeout)); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}

	CmdCLI.AddCommand(cmdInit)
}

func initCluster(_ *cobra.Command, args []string) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	ctx, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
	if len(args) > 1 {
		die("too many arguments")
	}

	initSpec := &apiv1.Spec{}
	switch len(args) {
	case 1:
		encodingErr := json.Unmarshal([]byte(args[0]), initSpec)
		if encodingErr != nil {
			logger.Fatal().
				AnErr("error", encodingErr).
				Str("input", "argument").
				Str("spec", args[0]).
				Msg("invalid cluster data spec")
		}
	case 0:
		if initOpts.file != "" {
			var readErr error
			var data []byte
			if initOpts.file == "-" {
				data, readErr = io.ReadAll(os.Stdin)
			} else {
				data, readErr = os.ReadFile(initOpts.file)
			}
			if readErr != nil {
				logger.Fatal().
					AnErr("error", readErr).
					Str("source", initOpts.file).
					Msg("cannot read from stdin")
			}
			if encodingErr := json.Unmarshal(data, initSpec); encodingErr != nil {
				logger.Fatal().
					AnErr("error", encodingErr).
					Str("source", initOpts.file).
					Str("spec", string(data)).
					Msg("invalid cluster data spec")
			}
		}
	}

	p := endpoints.HTTPS
	if !viper.GetBool(flagTLS) {
		p = endpoints.HTTP
	}
	apiClient := client.NewConnection(p, viper.GetString(flagHost), viper.GetUint16(flagPort),
		viper.GetDuration(flagTimeout))

	_, httpCode, getClusterErr := apiClient.GetCluster()
	if httpCode != http.StatusNotFound {
		if getClusterErr != nil {
			logger.Fatal().
				AnErr("error", getClusterErr).
				Str("source", initOpts.file).
				Msg("failed to get clusterdata")
		}
		logger.Fatal().Msg("cannot initialize an already existing cluster")
	}
	if httpCode, initErr := apiClient.PostClusterSpec(initSpec); initErr != nil {
		logger.Fatal().
			AnErr("error", initErr).
			Int("http_code", httpCode).
			Str("source", initOpts.file).
			Any("spec", initSpec).
			Msg("init failed")
	}
}
