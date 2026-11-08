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
	"github.com/pgvillage-tools/orion/internal/logging"

	"github.com/spf13/cobra"
)

const (
	flagPort    = "port"
	flagHost    = "host"
	flagFile    = "file"
	flagPatch   = "patch"
	flagIfMatch = "if-match"
	flagTimeout = "timeout"
)

var cmdClusterData = &cobra.Command{
	Use:   "clusterdata",
	Short: "Manage current cluster data",
}

type clusterdataReadOptions struct {
	pretty bool
}

var readClusterdataOpts clusterdataReadOptions

type clusterdataWriteOptions struct {
	file    string
	ifMatch string
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
	cmdClusterData.AddCommand(cmdReadClusterData)

	cmdWriteClusterData.PersistentFlags().StringVar(&writeClusterdataOpts.ifMatch, flagIfMatch, "",
		"only write when the cluster data still has this ETag (as printed by clusterdata read)")
	cmdWriteClusterData.PersistentFlags().StringVarP(&writeClusterdataOpts.file, flagFile, "f", "",
		"file containing the new cluster data")
	cmdClusterData.AddCommand(cmdWriteClusterData)

	CmdCLI.AddCommand(cmdClusterData)
}

func readClusterdata(_ *cobra.Command, _ []string) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	_, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
	apiClient := defaultAPIClient(ctx)
	cd, etag, httpCode, getClusterErr := apiClient.GetClusterWithETag()
	if getClusterErr != nil {
		logger.Fatal().
			AnErr(logging.KeyError, getClusterErr).
			Int("http return code", httpCode).
			Msg("failed to get clusterdata")
	}
	// on stderr, so stdout stays clean; pass it to `orion-cli clusterdata write --if-match`
	stderr("ETag: %s", etag)
	var clusterdataj []byte
	var marshalErr error
	if readClusterdataOpts.pretty {
		clusterdataj, marshalErr = json.MarshalIndent(cd, "", "\t")
		if marshalErr != nil {
			logger.Fatal().AnErr(logging.KeyError, marshalErr).Msg("failed to marshall clusterdata")
		}
	} else {
		clusterdataj, marshalErr = json.Marshal(cd)
		if marshalErr != nil {
			logger.Fatal().AnErr(logging.KeyError, marshalErr).Msg("failed to marshall clusterdata")
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
				AnErr(logging.KeyError, encodingErr).
				Str("input", "argument").
				Str("spec", args[0]).
				Msg("invalid cluster data spec")
		}
	case 0:
		if writeClusterdataOpts.file == "" {
			logger.Fatal().Msg("no cluster data provided: pass it as an argument or via --file")
		}
		var readErr error
		var data []byte
		if writeClusterdataOpts.file == "-" {
			data, readErr = io.ReadAll(os.Stdin)
		} else {
			data, readErr = os.ReadFile(writeClusterdataOpts.file)
		}
		if readErr != nil {
			logger.Fatal().
				AnErr(logging.KeyError, readErr).
				Str("source", writeClusterdataOpts.file).
				Msg("cannot read from stdin")
		}
		if encodingErr := json.Unmarshal(data, cd); encodingErr != nil {
			logger.Fatal().
				AnErr(logging.KeyError, encodingErr).
				Str("source", writeClusterdataOpts.file).
				Str("spec", string(data)).
				Msg("invalid cluster data spec")
		}
	default:
		logger.Fatal().Msg("clusterdata write accepts at most one positional argument")
	}

	apiClient := defaultAPIClient(ctx)
	httpCode, _, putClusterErr := apiClient.PutClusterIfMatch(cd, writeClusterdataOpts.ifMatch)
	if putClusterErr != nil {
		logger.Fatal().
			AnErr(logging.KeyError, putClusterErr).
			Int("http return code", httpCode).
			Msg("failed to get clusterdata")
	}
}
