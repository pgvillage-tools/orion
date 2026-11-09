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

package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var removeKeeperCmd = &cobra.Command{
	Use:   "removekeeper [keeper uid]",
	Short: "Removes a keeper that is gone for good from the cluster data",
	Long: "Removes a keeper, and its database, from the cluster data, for a keeper that is gone for good (e.g. its " +
		"node is lost). A keeper that still runs registers itself again. The keeper of the primary can't be removed: " +
		"fail it first (orion-cli failkeeper), and remove it once another keeper runs the primary.",
	RunE: removeKeeper,
}

var removeKeeperWait waitOptions

func init() {
	addWaitFlags(removeKeeperCmd, &removeKeeperWait)
	CmdCLI.AddCommand(removeKeeperCmd)
}

func removeKeeper(c *cobra.Command, args []string) error {
	if len(args) > 1 {
		return errors.New("too many arguments")
	}
	if len(args) == 0 {
		return errors.New("keeper uid required")
	}
	keeperID := args[0]
	apiClient, err := defaultAPIClient()
	if err != nil {
		return err
	}
	if httpCode, deleteErr := apiClient.DeleteKeeper(keeperID); deleteErr != nil {
		return apiError("failed to remove the keeper", httpCode, deleteErr)
	}
	return waitUntilConverged(c.Context(), apiClient, removeKeeperWait, "")
}
