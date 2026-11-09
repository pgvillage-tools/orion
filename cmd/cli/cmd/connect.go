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
	"fmt"

	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cmdConnect = &cobra.Command{
	Use:   "connect",
	RunE:  connectCluster,
	Short: "connect to an API",
}

// InitOptions is a struct which can contain initiation options
type connectOptions struct {
	file string
}

var connOpts connectOptions

func init() {
	CmdCLI.AddCommand(cmdConnect)
}

func connectCluster(_ *cobra.Command, _ []string) error {
	apiClient, err := defaultAPIClient()
	if err != nil {
		return err
	}
	identity, httpCode, whoAmIErr := apiClient.WhoAmI()
	if whoAmIErr != nil {
		return apiError("connecting to the api ("+apiClient.EndpointURL(endpoints.WhoAmIEndPoint)+") failed", httpCode,
			whoAmIErr)
	}
	stdout("Connected to the api as %s (%s)", identity.Name, identity.Method)
	if writeErr := viper.WriteConfig(); writeErr != nil {
		return fmt.Errorf("updating cli config failed: %w", writeErr)
	}
	return nil
}
