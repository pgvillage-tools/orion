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
	"encoding/json"
	"errors"
	"fmt"

	cluster "github.com/pgvillage-tools/orion/api/v1"

	"github.com/spf13/cobra"
)

var cmdUpdate = &cobra.Command{
	Use:   "update",
	RunE:  update,
	Short: "Update a cluster specification",
}

type updateOptions struct {
	patch   bool
	file    string
	ifMatch string
}

var updateOpts updateOptions

func init() {
	cmdUpdate.PersistentFlags().BoolVarP(&updateOpts.patch, flagPatch, "P", false,
		"patch the current cluster specification instead of replacing it")
	cmdUpdate.PersistentFlags().StringVar(&updateOpts.ifMatch, flagIfMatch, "",
		"only update when the cluster data still has this ETag (as printed by orion-cli spec)")
	cmdUpdate.PersistentFlags().StringVarP(&updateOpts.file, flagFile, "f", "",
		"file containing a complete cluster specification or a patch to apply to the current cluster specification")

	addWaitFlags(cmdUpdate, &updateWait)
	CmdCLI.AddCommand(cmdUpdate)
}

func update(c *cobra.Command, args []string) error {
	if len(args) > 1 {
		return errors.New("too many arguments")
	}
	if updateOpts.file == "" && len(args) < 1 {
		return errors.New("no cluster spec provided as argument and no file provided (--file/-f option)")
	}
	if updateOpts.file != "" && len(args) == 1 {
		return errors.New("only one of cluster spec provided as argument or file must provided (--file/-f option)")
	}
	var patch []byte
	if len(args) == 1 {
		patch = []byte(args[0])
	} else {
		var err error
		if patch, err = readInput(updateOpts.file); err != nil {
			return err
		}
	}

	var newSpec *cluster.Spec
	if !updateOpts.patch {
		newSpec = &cluster.Spec{}
		if err := json.Unmarshal(patch, newSpec); err != nil {
			return fmt.Errorf("invalid cluster spec: %w", err)
		}
	}
	apiClient, err := defaultAPIClient()
	if err != nil {
		return err
	}
	var httpCode int
	var updateErr error
	if updateOpts.patch {
		httpCode, _, updateErr = apiClient.PatchClusterSpecIfMatch(patch, updateOpts.ifMatch)
	} else {
		httpCode, _, updateErr = apiClient.PutClusterSpecIfMatch(newSpec, updateOpts.ifMatch)
	}
	if updateErr != nil {
		return apiError("failed to update the cluster spec", httpCode, updateErr)
	}
	return waitUntilConverged(c.Context(), apiClient, updateWait, "")
}

var updateWait waitOptions
