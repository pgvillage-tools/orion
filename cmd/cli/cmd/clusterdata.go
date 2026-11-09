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
	"encoding/json"
	"errors"
	"fmt"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"

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
	RunE:  readClusterdata,
	Short: "Retrieve the current cluster data",
}

var cmdWriteClusterData = &cobra.Command{
	Use:   "write",
	RunE:  runWriteClusterdata,
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

func readClusterdata(_ *cobra.Command, _ []string) error {
	apiClient, err := defaultAPIClient()
	if err != nil {
		return err
	}
	cd, etag, httpCode, getClusterErr := apiClient.GetClusterWithETag()
	if getClusterErr != nil {
		return apiError("failed to get the cluster data", httpCode, getClusterErr)
	}
	// on stderr, so stdout stays clean; pass it to `orion-cli clusterdata write --if-match`
	stderr("ETag: %s", etag)
	var clusterdataj []byte
	if readClusterdataOpts.pretty {
		clusterdataj, err = json.MarshalIndent(cd, "", "\t")
	} else {
		clusterdataj, err = json.Marshal(cd)
	}
	if err != nil {
		return fmt.Errorf("failed to marshal the cluster data: %w", err)
	}
	stdout("%s", clusterdataj)
	return nil
}

func runWriteClusterdata(_ *cobra.Command, args []string) error {
	var data []byte
	switch len(args) {
	case 1:
		data = []byte(args[0])
	case 0:
		if writeClusterdataOpts.file == "" {
			return errors.New("no cluster data provided: pass it as an argument or via --file")
		}
		var err error
		if data, err = readInput(writeClusterdataOpts.file); err != nil {
			return err
		}
	default:
		return errors.New("clusterdata write accepts at most one positional argument")
	}
	cd := &apiv1.Data{}
	if err := json.Unmarshal(data, cd); err != nil {
		return fmt.Errorf("invalid cluster data: %w", err)
	}

	apiClient, err := defaultAPIClient()
	if err != nil {
		return err
	}
	httpCode, _, putClusterErr := apiClient.PutClusterIfMatch(cd, writeClusterdataOpts.ifMatch)
	if putClusterErr != nil {
		return apiError("failed to write the cluster data", httpCode, putClusterErr)
	}
	return nil
}
