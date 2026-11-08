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
	"os"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/logging"

	"github.com/spf13/cobra"
)

var cmdUpdate = &cobra.Command{
	Use:   "update",
	Run:   update,
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

	CmdCLI.AddCommand(cmdUpdate)
}

func update(_ *cobra.Command, args []string) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	ctx, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
	if len(args) > 1 {
		die("too many arguments")
	}
	if updateOpts.file == "" && len(args) < 1 {
		die("no cluster spec provided as argument and no file provided (--file/-f option)")
	}
	if updateOpts.file != "" && len(args) == 1 {
		die("only one of cluster spec provided as argument or file must provided (--file/-f option)")
	}

	var patch []byte
	if len(args) == 1 {
		patch = []byte(args[0])
	} else {
		var err error
		if updateOpts.file == "-" {
			patch, err = io.ReadAll(os.Stdin)
			if err != nil {
				die("cannot read from stdin: %v", err)
			}
		} else {
			patch, err = os.ReadFile(updateOpts.file)
			if err != nil {
				die("cannot read file: %v", err)
			}
		}
	}

	apiClient := defaultAPIClient(ctx)
	var httpCode int
	var updateErr error
	if updateOpts.patch {
		httpCode, _, updateErr = apiClient.PatchClusterSpecIfMatch(patch, updateOpts.ifMatch)
	} else {
		cd := &cluster.Spec{}
		if encodingErr := json.Unmarshal(patch, cd); encodingErr != nil {
			logger.Fatal().
				AnErr(logging.KeyError, encodingErr).
				Str("source", updateOpts.file).
				Str("spec", string(patch)).
				Msg("invalid cluster data spec")
		}
		httpCode, _, updateErr = apiClient.PutClusterSpecIfMatch(cd, updateOpts.ifMatch)
	}
	if updateErr != nil {
		logger.Fatal().
			AnErr(logging.KeyError, updateErr).
			Int("http return code", httpCode).
			Msg("failed to get clusterdata")
	}
}
