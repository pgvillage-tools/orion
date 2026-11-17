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

// Package cmd holds the orion-proxy command
package cmd

import (
	"context"

	"github.com/pgvillage-tools/orion/cmd"
	"github.com/pgvillage-tools/orion/internal/flagutil"
	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/proxy"
	"github.com/pgvillage-tools/orion/internal/store"
	"github.com/spf13/cobra"
)

// CmdProxy is the cobra command which defines running orion-proxy
var CmdProxy = &cobra.Command{
	Use:     "orion-proxy",
	Run:     runProxy,
	Version: cmd.Version,
}

var (
	cfg   proxy.Config
	debug bool
)

func init() {
	store.AddCommonFlags(CmdProxy, &cfg.CommonConfig)

	CmdProxy.PersistentFlags().StringVar(
		&cfg.ListenAddress,
		"listen-address",
		"127.0.0.1",
		"proxy listening address")
	CmdProxy.PersistentFlags().StringVar(
		&cfg.Port,
		"port",
		"5432",
		"proxy listening port")
	CmdProxy.PersistentFlags().BoolVar(
		&cfg.StopListening,
		"stop-listening",
		true,
		"stop listening on store error")
	CmdProxy.PersistentFlags().BoolVar(
		&debug,
		"debug",
		false,
		"enable debug logging")
	CmdProxy.PersistentFlags().IntVar(
		&cfg.KeepAliveIdle,
		"tcp-keepalive-idle",
		0,
		"set tcp keepalive idle (seconds)")
	CmdProxy.PersistentFlags().IntVar(
		&cfg.KeepAliveCount,
		"tcp-keepalive-count",
		0,
		"set tcp keepalive probe count number")
	CmdProxy.PersistentFlags().IntVar(
		&cfg.KeepAliveInterval,
		"tcp-keepalive-interval",
		0,
		"set tcp keepalive interval (seconds)")

	if err := CmdProxy.PersistentFlags().MarkDeprecated("debug", "use --log-level=debug instead"); err != nil {
		_, logger := logging.GetLogComponent(context.Background(), logging.ProxyComponent)
		logger.Fatal().AnErr(logging.KeyErr, err).Msg("")
	}
}

// Execute is the main executor of the proxy
func Execute() {
	_, logger := logging.GetLogComponent(context.Background(), logging.ProxyComponent)
	if err := flagutil.SetFlagsFromEnv(CmdProxy.PersistentFlags(), "ORIONPROXY"); err != nil {
		logger.Fatal().AnErr(logging.KeyErr, err).Msg("")
	}

	if err := CmdProxy.Execute(); err != nil {
		logger.Fatal().AnErr(logging.KeyErr, err).Msg("")
	}
}

func runProxy(c *cobra.Command, _ []string) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	ctx, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	defer cancelFunc()
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

	store.SetMetrics(&cfg.CommonConfig, "proxy")

	if cfg.KeepAliveIdle < 0 {
		logger.Fatal().Msgf("tcp keepalive idle value must be greater or equal to 0")
	}
	if cfg.KeepAliveCount < 0 {
		logger.Fatal().Msgf("tcp keepalive count value must be greater or equal to 0")
	}
	if cfg.KeepAliveInterval < 0 {
		logger.Fatal().Msgf("tcp keepalive interval value must be greater or equal to 0")
	}

	if err := proxy.Run(ctx, cfg); err != nil {
		logger.Fatal().Msg(err.Error())
	}
}
