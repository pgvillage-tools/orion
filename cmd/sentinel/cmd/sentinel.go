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

// Package cmd holds the orion-sentinel command
package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/pgvillage-tools/orion/cmd"
	"github.com/pgvillage-tools/orion/internal/flagutil"
	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/sentinel"
	"github.com/pgvillage-tools/orion/internal/store"
	"github.com/spf13/cobra"
)

// CmdSentinel is a variable which contains the cobra command
var CmdSentinel = &cobra.Command{
	Use:     "orion-sentinel",
	Run:     runSentinel,
	Version: cmd.Version,
}

var (
	cfg   sentinel.Config
	debug bool
)

func init() {
	_, logger := logging.GetLogComponent(context.Background(), logging.SentinelComponent)
	store.AddCommonFlags(CmdSentinel, &cfg.CommonConfig)

	CmdSentinel.PersistentFlags().StringVar(
		&cfg.InitialClusterSpecFile,
		"initial-cluster-spec",
		"",
		// revive:disable-next-line
		"a file providing the initial cluster specification, used only at cluster initialization, ignored if cluster is already initialized")
	CmdSentinel.PersistentFlags().BoolVar(
		&debug,
		"debug",
		false,
		"enable debug logging (deprecated, use log-level instead)")

	if err := CmdSentinel.PersistentFlags().MarkDeprecated("debug", "use --log-level=debug instead"); err != nil {
		logger.Fatal().AnErr(logging.KeyErr, err).Msg("")
	}
}

func sigHandler(ctx context.Context, sigs chan os.Signal, cancel context.CancelFunc) {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	s := <-sigs
	logger.Debug().Any("signal", s).Msg("got signal")
	cancel()
}

// Execute is the main execute function of the sentinel file
func Execute() {
	_, logger := logging.GetLogComponent(context.Background(), logging.SentinelComponent)
	if err := flagutil.SetFlagsFromEnv(CmdSentinel.PersistentFlags(), "ORIONSENTINEL"); err != nil {
		logger.Fatal().AnErr(logging.KeyErr, err).Msg("")
	}

	if err := CmdSentinel.Execute(); err != nil {
		logger.Fatal().AnErr(logging.KeyErr, err).Msg("")
	}
}

func runSentinel(c *cobra.Command, _ []string) {
	ctx, logger := logging.GetLogComponent(context.Background(), logging.SentinelComponent)
	logging.SetStaticLevel(cfg.LogLevel)
	if debug {
		logging.SetStaticLevel("debug")
	}
	if store.IsColorLoggerEnable(c, &cfg.CommonConfig) {
		logging.EnableColor()
	}

	if err := store.CheckCommonConfig(&cfg.CommonConfig); err != nil {
		logger.Fatal().AnErr(logging.KeyErr, err).Msg("")
	}

	store.SetMetrics(&cfg.CommonConfig, "sentinel")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go sigHandler(ctx, sigs, cancel)

	if err := sentinel.Run(ctx, cfg); err != nil {
		logger.Fatal().AnErr(logging.KeyErr, err).Msg("")
	}
}
