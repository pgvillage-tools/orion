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
	"encoding/json"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
	"github.com/pgvillage-tools/orion/internal/logging"
	client "github.com/pgvillage-tools/orion/pkg/api_client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cmdSpec = &cobra.Command{
	Use:   "spec",
	Run:   spec,
	Short: "Retrieve the current cluster specification",
}

type specOptions struct {
	defaults bool
	Host     string `mapstructure:"host"`
	Port     uint16 `mapstructure:"port"`
	TLS      bool   `mapstructure:"tls"`
}

var specOpts specOptions

func init() {
	_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
	cmdSpec.PersistentFlags().BoolVar(&specOpts.defaults, "defaults", false,
		"also show default values")

	cmdSpec.PersistentFlags().BoolVarP(&specOpts.TLS, "tls", "t", true, "use tls")
	if err := viper.BindPFlag("tls", cmdSpec.PersistentFlags().Lookup("tls")); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
	cmdSpec.PersistentFlags().Uint16VarP(&specOpts.Port, "port", "p", defaultAPIPort,
		"protocol for connecting to the api")
	if err := viper.BindPFlag("port", cmdSpec.PersistentFlags().Lookup("port")); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
	cmdSpec.PersistentFlags().StringVarP(&specOpts.Host, "host", "H", defaultAPIIP,
		"hostname or ip for connecting to the api")
	if err := viper.BindPFlag("host", cmdSpec.PersistentFlags().Lookup("host")); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
	CmdCLI.AddCommand(cmdSpec)
}

// ClusterSpecNoDefaults is used to mashal a json with a cluster spec if no defaults should be used
type ClusterSpecNoDefaults struct {
	SleepInterval                    *cluster.Duration         `json:"sleepInterval,omitempty"`
	RequestTimeout                   *cluster.Duration         `json:"requestTimeout,omitempty"`
	ConvergenceTimeout               *cluster.Duration         `json:"convergenceTimeout,omitempty"`
	InitTimeout                      *cluster.Duration         `json:"initTimeout,omitempty"`
	SyncTimeout                      *cluster.Duration         `json:"syncTimeout,omitempty"`
	DBWaitReadyTimeout               *cluster.Duration         `json:"dbWaitReadyTimeout,omitempty"`
	FailInterval                     *cluster.Duration         `json:"failInterval,omitempty"`
	DeadKeeperRemovalInterval        *cluster.Duration         `json:"deadKeeperRemovalInterval,omitempty"`
	ProxyCheckInterval               *cluster.Duration         `json:"proxyCheckInterval,omitempty"`
	ProxyTimeout                     *cluster.Duration         `json:"proxyTimeout,omitempty"`
	MaxStandbys                      *uint16                   `json:"maxStandbys,omitempty"`
	MaxStandbysPerSender             *uint16                   `json:"maxStandbysPerSender,omitempty"`
	MaxStandbyLag                    *uint32                   `json:"maxStandbyLag,omitempty"`
	SynchronousReplication           *bool                     `json:"synchronousReplication,omitempty"`
	MinSynchronousStandbys           *uint16                   `json:"minSynchronousStandbys,omitempty"`
	MaxSynchronousStandbys           *uint16                   `json:"maxSynchronousStandbys,omitempty"`
	AdditionalWalSenders             *uint16                   `json:"additionalWalSenders,omitempty"`
	AdditionalMasterReplicationSlots []string                  `json:"additionalMasterReplicationSlots,omitempty"`
	UsePgrewind                      *bool                     `json:"usePgrewind,omitempty"`
	InitMode                         *cluster.InitMode         `json:"initMode,omitempty"`
	MergePgParameters                *bool                     `json:"mergePgParameters,omitempty"`
	Role                             *cluster.Role             `json:"role,omitempty"`
	NewConfig                        *cluster.NewConfig        `json:"newConfig,omitempty"`
	PITRConfig                       *cluster.PITRConfig       `json:"pitrConfig,omitempty"`
	ExistingConfig                   *cluster.ExistingConfig   `json:"existingConfig,omitempty"`
	StandbyConfig                    *cluster.StandbyConfig    `json:"standbyConfig,omitempty"`
	DefaultSUReplAccessMode          *cluster.SUReplAccessMode `json:"defaultSUReplAccessMode,omitempty"`
	PGParameters                     cluster.PGParameters      `json:"pgParameters,omitempty"`
	PGHBA                            []string                  `json:"pgHBA,omitempty"`
	AutomaticPgRestart               *bool                     `json:"automaticPgRestart,omitempty"`
}

// ClusterSpecDefaults is used to mashal a json with a cluster spec if defaults should be used
type ClusterSpecDefaults struct {
	SleepInterval                    *cluster.Duration         `json:"sleepInterval"`
	RequestTimeout                   *cluster.Duration         `json:"requestTimeout"`
	ConvergenceTimeout               *cluster.Duration         `json:"convergenceTimeout"`
	InitTimeout                      *cluster.Duration         `json:"initTimeout"`
	SyncTimeout                      *cluster.Duration         `json:"syncTimeout"`
	DBWaitReadyTimeout               *cluster.Duration         `json:"dbWaitReadyTimeout"`
	FailInterval                     *cluster.Duration         `json:"failInterval"`
	DeadKeeperRemovalInterval        *cluster.Duration         `json:"deadKeeperRemovalInterval"`
	ProxyCheckInterval               *cluster.Duration         `json:"proxyCheckInterval"`
	ProxyTimeout                     *cluster.Duration         `json:"proxyTimeout"`
	MaxStandbys                      *uint16                   `json:"maxStandbys"`
	MaxStandbysPerSender             *uint16                   `json:"maxStandbysPerSender"`
	MaxStandbyLag                    *uint32                   `json:"maxStandbyLag"`
	SynchronousReplication           *bool                     `json:"synchronousReplication"`
	MinSynchronousStandbys           *uint16                   `json:"minSynchronousStandbys"`
	MaxSynchronousStandbys           *uint16                   `json:"maxSynchronousStandbys"`
	AdditionalWalSenders             *uint16                   `json:"additionalWalSenders"`
	AdditionalMasterReplicationSlots []string                  `json:"additionalMasterReplicationSlots"`
	UsePgrewind                      *bool                     `json:"usePgrewind"`
	InitMode                         *cluster.InitMode         `json:"initMode"`
	MergePgParameters                *bool                     `json:"mergePgParameters"`
	Role                             *cluster.Role             `json:"role"`
	NewConfig                        *cluster.NewConfig        `json:"newConfig"`
	PITRConfig                       *cluster.PITRConfig       `json:"pitrConfig"`
	ExistingConfig                   *cluster.ExistingConfig   `json:"existingConfig"`
	StandbyConfig                    *cluster.StandbyConfig    `json:"standbyConfig"`
	DefaultSUReplAccessMode          *cluster.SUReplAccessMode `json:"defaultSUReplAccessMode"`
	PGParameters                     cluster.PGParameters      `json:"pgParameters"`
	PGHBA                            []string                  `json:"pgHBA"`
	AutomaticPgRestart               *bool                     `json:"automaticPgRestart"`
}

func spec(_ *cobra.Command, _ []string) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	ctx, logger := logging.GetLogComponent(ctx, logging.CmdComponent)

	p := endpoints.HTTPS
	if !specOpts.TLS {
		p = endpoints.HTTP
	}
	apiClient := client.NewConnection(p, specOpts.Host, specOpts.Port)
	cd, httpCode, getClusterErr := apiClient.GetCluster()
	if getClusterErr != nil {
		logger.Fatal().
			AnErr("error", getClusterErr).
			Int("http return code", httpCode).
			Msg("failed to get clusterdata")
	}

	var specj []byte
	var err error
	if specOpts.defaults {
		cs := (*ClusterSpecDefaults)(cd.Cluster.DefSpec())
		specj, err = json.MarshalIndent(cs, "", "\t")
	} else {
		cs := (*ClusterSpecNoDefaults)(cd.Cluster.Spec)
		specj, err = json.MarshalIndent(cs, "", "\t")
	}
	if err != nil {
		logger.Fatal().
			AnErr("error", getClusterErr).
			Int("http return code", httpCode).
			Msg("failed to marshall spec")
	}

	stdout("%s", specj)
}
