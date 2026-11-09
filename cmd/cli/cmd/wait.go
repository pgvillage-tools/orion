// Copyright 2026 PgVillage
// Copyright 2015 Sorint.lab
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
	"fmt"
	"time"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/logging"
	client "github.com/pgvillage-tools/orion/pkg/api_client"
	"github.com/spf13/cobra"
)

const (
	flagWait           = "wait"
	flagWaitTimeout    = "wait-timeout"
	defaultWaitTimeout = 5 * time.Minute
	waitPollInterval   = 2 * time.Second
)

// waitOptions are the --wait and --wait-timeout flags of a command
type waitOptions struct {
	wait    bool
	timeout time.Duration
}

// addWaitFlags adds --wait and --wait-timeout to c
func addWaitFlags(c *cobra.Command, opts *waitOptions) {
	c.Flags().BoolVar(&opts.wait, flagWait, false,
		"wait until the cluster applied the change (available, and every healthy keeper converged)")
	c.Flags().DurationVar(&opts.timeout, flagWaitTimeout, defaultWaitTimeout, "how long --wait waits at most")
}

// converged returns whether the cluster in status applied all changes: it is available, and the database of every
// healthy keeper is at its wanted generation. When notPrimary is set, another keeper must run the primary (after
// failing that keeper). It returns why not, for the progress message.
func converged(status *apiv1.InfoCluster, notPrimary string) (bool, string) {
	if available, ok := status.Status["available"].(bool); !ok || !available {
		return false, "the cluster isn't available"
	}
	if primary, ok := status.Status["primaryKeeper"].(string); ok && notPrimary != "" && primary == notPrimary {
		return false, "keeper " + notPrimary + " still runs the primary"
	}
	for _, k := range status.Keepers {
		if k.Healthy && k.PgCurrentGeneration != k.PgWantedGeneration {
			return false, "keeper " + k.UID + " hasn't applied the change yet"
		}
	}
	return true, ""
}

// waitUntilConverged polls the cluster status until converged (see converged), or until the timeout of opts. It does
// nothing without --wait.
func waitUntilConverged(ctx context.Context, apiClient client.Connection, opts waitOptions, notPrimary string) {
	if !opts.wait {
		return
	}
	_, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
	deadline := time.Now().Add(opts.timeout)
	reason := ""
	for {
		status, _, err := apiClient.GetStatus()
		if err == nil {
			var done bool
			if done, reason = converged(status, notPrimary); done {
				stdout("the cluster applied the change")
				return
			}
		} else {
			reason = err.Error()
		}
		if time.Now().After(deadline) {
			logger.Fatal().Str("waiting for", reason).Msg(fmt.Sprintf("--wait timed out after %s", opts.timeout))
		}
		time.Sleep(waitPollInterval)
	}
}
