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
	"errors"

	"github.com/spf13/cobra"
)

var cmdPromote = &cobra.Command{
	Use:   "promote",
	RunE:  promote,
	Short: "Promotes a standby cluster to a primary cluster",
}

var promoteWait waitOptions

func init() {
	addWaitFlags(cmdPromote, &promoteWait)
	CmdCLI.AddCommand(cmdPromote)
}

func promote(c *cobra.Command, args []string) error {
	if len(args) > 0 {
		return errors.New("too many arguments")
	}
	apiClient, err := defaultAPIClient()
	if err != nil {
		return err
	}
	if httpCode, promoteErr := apiClient.PromoteCluster(); promoteErr != nil {
		return apiError("promote failed", httpCode, promoteErr)
	}
	return waitUntilConverged(c.Context(), apiClient, promoteWait, "")
}
