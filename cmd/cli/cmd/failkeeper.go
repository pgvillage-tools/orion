// Copyright 2026 PgVillage
// Copyright 2018 Sorint.lab
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

// revive:disable

var failKeeperCmd = &cobra.Command{
	Use:   "failkeeper [keeper uid]",
	Short: `Force keeper as "temporarily" failed. The sentinel will compute a new clusterdata considering it as failed and then restore its state to the real one.`,
	Long:  `Force keeper as "temporarily" failed. It's just a one shot operation, the sentinel will compute a new clusterdata considering the keeper as failed and then restore its state to the real one. For example, if the force failed keeper is a master, the sentinel will try to elect a new master. If no new master can be elected, the force failed keeper, if really healthy, will be re-elected as master`,
	RunE:  failKeeper,
}

//revive:enable

var failKeeperWait waitOptions

func init() {
	addWaitFlags(failKeeperCmd, &failKeeperWait)
	CmdCLI.AddCommand(failKeeperCmd)
}

func failKeeper(c *cobra.Command, args []string) error {
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
	if httpCode, failErr := apiClient.FailKeeper(keeperID); failErr != nil {
		return apiError("failed to fail the keeper", httpCode, failErr)
	}
	// when it ran the primary, the change is done once another keeper runs it
	return waitUntilConverged(c.Context(), apiClient, failKeeperWait, keeperID)
}
