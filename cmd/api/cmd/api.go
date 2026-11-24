// Copyright 2026 PgVillage
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

// Package cmd holds the orion-api command
package cmd

import (
	"context"
	"os"

	cmdcommon "github.com/pgvillage-tools/orion/cmd"
	"github.com/pgvillage-tools/orion/internal/api"
	"github.com/pgvillage-tools/orion/internal/flagutil"
	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/pki"
	"github.com/pgvillage-tools/orion/internal/store"
	"github.com/spf13/cobra"
)

const keyError = "error"

// CmdWebApi exports the main webapi process
var CmdWebApi = &cobra.Command{
	Use:     "api",
	Run:     runAPI,
	Version: cmdcommon.Version,
}

var (
	cfg   api.Config
	debug bool
)

func init() {
	cmdcommon.AddCommonFlags(CmdWebApi, &cfg.CommonConfig)
	// revive:disable
	CmdWebApi.PersistentFlags().StringVar(&cfg.BaseURL, "baseurl", "0.0.0.0",
		"address for the api and probe ports to listen on")
	CmdWebApi.PersistentFlags().UintVar(&cfg.Port, "port", api.DefaultPort,
		"port for the api (TLS, a client certificate is required)")
	CmdWebApi.PersistentFlags().UintVar(&cfg.ProbePort, "probe-port", api.DefaultProbePort,
		"port for /livez, /readyz, /status and /metrics (plain HTTP, no authentication)")
	CmdWebApi.PersistentFlags().DurationVar(&cfg.ShutdownDelay, "shutdown-delay", api.DefaultShutdownDelay,
		"on SIGTERM, the time /readyz returns 503 before the api stops, so load balancers stop sending requests")
	CmdWebApi.PersistentFlags().BoolVar(&debug, "debug", false,
		"enable debug logging")
	CmdWebApi.PersistentFlags().StringVar(&cfg.PKIDir, "pki-dir", api.DefaultPKIDir,
		"directory with the certificates of the api, as created by orion-cli pki init")
	CmdWebApi.PersistentFlags().StringVar(&cfg.TLSCertFile, "tls-cert-file", "",
		"server certificate of the api, followed by its chain (default <pki-dir>/"+pki.APIServerCertFile+")")
	CmdWebApi.PersistentFlags().StringVar(&cfg.TLSKeyFile, "tls-key-file", "",
		"private key of the server certificate (default <pki-dir>/"+pki.APIServerKeyFile+")")
	CmdWebApi.PersistentFlags().StringVar(&cfg.TLSCAFile, "tls-ca-file", "",
		"CA that signs the client certificates the api accepts (default <pki-dir>/"+pki.APIClientCAFile+")")
	CmdWebApi.PersistentFlags().StringVar(&cfg.BootstrapSecretFile, "bootstrap-secret-file", "",
		"file with the bootstrap secret (default: "+api.BootstrapSecretEnv+", or a generated secret that is logged)")
	CmdWebApi.PersistentFlags().DurationVar(&cfg.BootstrapSecretTTL, "bootstrap-secret-ttl",
		api.DefaultBootstrapSecretTTL, "time the bootstrap secret is valid, from the start of bootstrap mode")
	CmdWebApi.PersistentFlags().BoolVar(&cfg.Rebootstrap, "rebootstrap", false,
		"remove the files of a bootstrap the cli didn't confirm (e.g. it lost them), and start a new one")
	// revive:enable
}

// Execute can be used to start and run the API
func Execute() {
	_, logger := logging.GetLogComponent(context.Background(), logging.WebApiComponent)
	if err := flagutil.SetFlagsFromEnv(CmdWebApi.PersistentFlags(), "ORIONAPI"); err != nil {
		logger.Fatal().AnErr("err", err).Msg("")
	}

	if err := CmdWebApi.Execute(); err != nil {
		logger.Fatal().AnErr("err", err).Msg("")
	}
}

func runAPI(c *cobra.Command, _ []string) {
	ctx, logger := logging.GetLogComponent(context.Background(), logging.WebApiComponent)

	// also --log-level, which the api didn't apply before
	if err := cmdcommon.SetupLogging(c, &cfg.CommonConfig, debug); err != nil {
		logger.Fatal().AnErr(keyError, err).Msg("")
	}

	if err := store.CheckCommonConfig(&cfg.CommonConfig); err != nil {
		logger.Fatal().AnErr(keyError, err).Msg("")
	}

	// the secret itself comes from the environment only: arguments are visible in ps
	cfg.BootstrapSecret = os.Getenv(api.BootstrapSecretEnv)
	if err := api.Run(ctx, cfg); err != nil {
		logger.Fatal().AnErr(keyError, err).Msg("")
	}
}
