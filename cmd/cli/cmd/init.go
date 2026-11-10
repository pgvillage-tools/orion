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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/util"
	"github.com/spf13/cobra"
)

var cmdInit = &cobra.Command{
	Use:   "init [cluster spec]",
	RunE:  initCluster,
	Short: "Initialize a new cluster",
}

// InitOptions is a struct which can contain initiation options
type InitOptions struct {
	file string
}

var initOpts InitOptions

func init() {
	cmdInit.PersistentFlags().StringVarP(&initOpts.file, flagFile, "f", "",
		"file with the cluster spec, - for stdin (without a spec: a new cluster with the default settings)")

	addWaitFlags(cmdInit, &initWait)
	CmdCLI.AddCommand(cmdInit)
}

func initCluster(c *cobra.Command, args []string) error {
	if len(args) > 1 {
		return errors.New("too many arguments")
	}
	if len(args) == 1 && initOpts.file != "" {
		return errors.New("pass the cluster spec as an argument or with --file, not both")
	}

	var data []byte
	switch {
	case len(args) == 1:
		data = []byte(args[0])
	case initOpts.file != "":
		var err error
		if data, err = readInput(initOpts.file); err != nil {
			return err
		}
	}
	initSpec := &apiv1.Spec{}
	// empty input is no spec: a new cluster with the default settings
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, initSpec); err != nil {
			return fmt.Errorf("invalid cluster spec: %w", err)
		}
	}
	if initSpec.InitMode == nil {
		initSpec.InitMode = util.ToPtr(apiv1.New)
	}

	apiClient, err := defaultAPIClient()
	if err != nil {
		return err
	}
	_, httpCode, getClusterErr := apiClient.GetCluster()
	if httpCode != http.StatusNotFound {
		if getClusterErr != nil {
			return apiError("failed to get the cluster data", httpCode, getClusterErr)
		}
		return errors.New("cannot initialize an already existing cluster")
	}
	if httpCode, initErr := apiClient.PostCluster(initSpec); initErr != nil {
		return apiError("init failed", httpCode, initErr)
	}
	return waitUntilConverged(c.Context(), apiClient, initWait, "")
}

var initWait waitOptions
