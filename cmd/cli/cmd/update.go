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
	"fmt"
	"io"
	"os"
	"time"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
	"github.com/pgvillage-tools/orion/internal/logging"
	client "github.com/pgvillage-tools/orion/pkg/api_client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

var cmdUpdate = &cobra.Command{
	Use:   "update",
	Run:   update,
	Short: "Update a cluster specification",
}

type updateOptions struct {
	patch   bool
	file    string
	Host    string        `mapstructure:"host"`
	Port    uint16        `mapstructure:"port"`
	TLS     bool          `mapstructure:"tls"`
	Timeout time.Duration `mapstructure:"timeout"`
}

var updateOpts updateOptions

func init() {
	_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
	cmdUpdate.PersistentFlags().BoolVarP(&updateOpts.patch, flagPatch, "P", false,
		"patch the current cluster specification instead of replacing it")
	cmdUpdate.PersistentFlags().StringVarP(&updateOpts.file, flagFile, "f", "",
		"file containing a complete cluster specification or a patch to apply to the current cluster specification")

	cmdUpdate.PersistentFlags().BoolVarP(&updateOpts.TLS, "tls", "t", true, "use tls")
	if err := viper.BindPFlag("tls", cmdUpdate.PersistentFlags().Lookup("tls")); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
	cmdUpdate.PersistentFlags().Uint16VarP(&updateOpts.Port, "port", "p", defaultAPIPort,
		"protocol for connecting to the api")
	if err := viper.BindPFlag("port", cmdUpdate.PersistentFlags().Lookup("port")); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
	cmdUpdate.PersistentFlags().StringVarP(&updateOpts.Host, "host", "H", defaultAPIIP,
		"hostname or ip for connecting to the api")
	if err := viper.BindPFlag("host", cmdUpdate.PersistentFlags().Lookup("host")); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
	cmdUpdate.PersistentFlags().DurationVarP(&updateOpts.Timeout, flagTimeout, "T", defaultTimeout,
		"connection timeout for api endpoint")
	if err := viper.BindPFlag(flagTimeout, cmdUpdate.PersistentFlags().Lookup(flagTimeout)); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
	CmdCLI.AddCommand(cmdUpdate)
}

func patchClusterSpec(cs *cluster.Spec, p []byte) (*cluster.Spec, error) {
	csj, err := json.Marshal(cs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cluster spec: %v", err)
	}

	newcsj, err := strategicpatch.StrategicMergePatch(csj, p, &cluster.Spec{})
	if err != nil {
		return nil, fmt.Errorf("failed to merge patch cluster spec: %v", err)
	}
	var newcs *cluster.Spec
	if err := json.Unmarshal(newcsj, &newcs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal patched cluster spec: %v", err)
	}
	return newcs, nil
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

	p := endpoints.HTTPS
	if !viper.GetBool(flagTLS) {
		p = endpoints.HTTP
	}
	apiClient := client.NewConnection(p, viper.GetString(flagHost), viper.GetUint16(flagPort),
		viper.GetDuration(flagTimeout))
	var httpCode int
	var updateErr error
	if updateOpts.patch {
		httpCode, updateErr = apiClient.PatchClusterSpec(patch)
	} else {
		cd := &cluster.Spec{}
		if encodingErr := json.Unmarshal(patch, cd); encodingErr != nil {
			logger.Fatal().
				AnErr("error", encodingErr).
				Str("source", updateOpts.file).
				Str("spec", string(patch)).
				Msg("invalid cluster data spec")
		}
		httpCode, updateErr = apiClient.PutClusterSpec(cd)
	}
	if updateErr != nil {
		logger.Fatal().
			AnErr("error", updateErr).
			Int("http return code", httpCode).
			Msg("failed to get clusterdata")
	}
}
