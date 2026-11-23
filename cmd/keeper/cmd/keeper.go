// Copyright 2026 PgVillage
// Copyright 2015 Sorint.lab
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied
// See the License for the specific language governing permissions and
// limitations under the License.
// Package cmd holds the orion-keeper command
package cmd

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"

	cluster "github.com/pgvillage-tools/orion/api/v1"

	"github.com/pgvillage-tools/orion/cmd"
	"github.com/pgvillage-tools/orion/internal/flagutil"
	"github.com/pgvillage-tools/orion/internal/keeper"
	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/store"
	"github.com/pgvillage-tools/orion/internal/util"
	"github.com/spf13/cobra"
)

// CmdKeeper is the cobra command of the keeper
var CmdKeeper = &cobra.Command{
	Use:     "orion-keeper",
	Run:     runKeeper,
	Version: cmd.Version,
}

var (
	cfg   keeper.Config
	debug bool
)

func init() {
	ctx := context.Background()
	cmd.AddCommonFlags(CmdKeeper, &cfg.CommonConfig)
	// revive:disable
	CmdKeeper.PersistentFlags().StringVar(&cfg.UID, "id", "", "keeper uid (must be unique in the cluster and can contain only lower-case letters, numbers and the underscore character). If not provided a random uid will be generated.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.UID, "uid", "", "keeper uid (must be unique in the cluster and can contain only lower-case letters, numbers and the underscore character). If not provided a random uid will be generated.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.DataDir, "data-dir", "", "data directory")
	CmdKeeper.PersistentFlags().StringVar(&cfg.WALDir, "wal-dir", "", "wal directory")
	CmdKeeper.PersistentFlags().StringVar(&cfg.PGListenAddress, "pg-listen-address", "", "postgresql instance listening address, local address used for the postgres instance. For all network interface, you can set the value to '*'.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.PGAdvertiseAddress, "pg-advertise-address", "", "postgresql instance address from outside. Use it to expose ip different than local ip with a NAT networking config")
	CmdKeeper.PersistentFlags().StringVar(&cfg.PGPort, "pg-port", "5432", "postgresql instance listening port")
	CmdKeeper.PersistentFlags().StringVar(&cfg.PGAdvertisePort, "pg-advertise-port", "", "postgresql instance port from outside. Use it to expose port different than local port with a PAT networking config")
	CmdKeeper.PersistentFlags().StringVar(&cfg.PGBinPath, "pg-bin-path", "", "absolute path to postgresql binaries. If empty they will be searched in the current PATH")
	CmdKeeper.PersistentFlags().StringVar(&cfg.PGReplConnType, "pg-repl-connection-type", keeper.DefaultConnType, "postgres replication user connection type. Default is host.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.PGReplAuthMethod, "pg-repl-auth-method", keeper.DefaultAuthMethod, "postgres replication user auth method. Default is md5.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.PGReplLocalAuthMethod, "pg-repl-local-auth-method", "", "postgres replication user auth method. Default is same as pg-repl-auth-method.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.PGReplSSLMode, "pg-repl-ssl-mode", "prefer", "postgres replication user ssl-mode. Default is prefer.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.PGReplUsername, "pg-repl-username", "", "postgres replication user name. Required. It'll be created on db initialization. Must be the same for all keepers.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.PGReplPassword, "pg-repl-password", "", "postgres replication user password. Only one of --pg-repl-password or --pg-repl-passwordfile must be provided. Must be the same for all keepers.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.PGReplPasswordFile, "pg-repl-passwordfile", "", "postgres replication user password file. Only one of --pg-repl-password or --pg-repl-passwordfile must be provided. Must be the same for all keepers.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.PGSUConnType, "pg-su-connection-type", keeper.DefaultConnType, "postgres superuser connection type. Default is host.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.PGSUAuthMethod, "pg-su-auth-method", keeper.DefaultAuthMethod, "postgres superuser auth method. Default is md5.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.PGSULocalAuthMethod, "pg-su-local-auth-method", "", "postgres superuser auth method. Default is same as pg-su-auth-method.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.PGSUUsername, "pg-su-username", "", "postgres superuser user name. Used for keeper managed instance access and pg_rewind based synchronization. It'll be created on db initialization. Defaults to the name of the effective user running orion-keeper. Must be the same for all keepers.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.PGSUPassword, "pg-su-password", "", "postgres superuser password. Only one of --pg-su-password or --pg-su-passwordfile must be provided. Must be the same for all keepers.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.PGSUPasswordFile, "pg-su-passwordfile", "", "postgres superuser password file. Only one of --pg-su-password or --pg-su-passwordfile must be provided. Must be the same for all keepers)")
	CmdKeeper.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")

	CmdKeeper.PersistentFlags().BoolVar(&cfg.CanBeMaster, "can-be-master", true, "whether the keeper can be elected as master (false prevents it)")
	CmdKeeper.PersistentFlags().BoolVar(&cfg.CanBeSynchronousReplica, "can-be-synchronous-replica", true, "whether the keeper can be chosen as synchronous replica (false prevents it)")
	CmdKeeper.PersistentFlags().BoolVar(&cfg.DisableDataDirLocking, "disable-data-dir-locking", false, "disable locking on data dir. Warning! It'll cause data corruptions if two keepers are concurrently running with the same data dir.")
	// revive:enable

	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	if err := CmdKeeper.PersistentFlags().MarkDeprecated("id", "please use --uid"); err != nil {
		logger.Fatal().AnErr(logging.KeyErr, err).Msg("--id is deprecated, please use --uid")
	}
	if err := CmdKeeper.PersistentFlags().MarkDeprecated("debug", "use --log-level=debug instead"); err != nil {
		logger.Fatal().AnErr(logging.KeyErr, err).Msg("--debug is deprecated, please use --log-level=debug instead")
	}
}

func sigHandler(ctx context.Context, sigs chan os.Signal, cancel context.CancelFunc) {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	s := <-sigs
	logger.Info().Any("signal", s).Msg("got signal")
	cancel()
}

// Execute is the main executor of keeper file
func Execute() {
	_, logger := logging.GetLogComponent(context.Background(), logging.KeeperComponent)
	if err := flagutil.SetFlagsFromEnv(CmdKeeper.PersistentFlags(), "ORIONKEEPER"); err != nil {
		logger.Fatal().AnErr(logging.KeyErr, err).Msg("")
	}

	if err := CmdKeeper.Execute(); err != nil {
		logger.Fatal().AnErr(logging.KeyErr, err).Msg("")
	}
}

func runKeeper(c *cobra.Command, _ []string) {
	ctx, logger := logging.GetLogComponent(context.Background(), logging.KeeperComponent)
	var (
		err           error
		listenAddFlag = "pg-advertise-address"
	)

	flags := c.Flags()

	if !flags.Changed("pg-su-username") {
		// set the pgSuUsername to the current user
		var user string
		user, err = util.GetUser()
		if err != nil {
			logger.Fatal().AnErr(logging.KeyErr, err).Msg("cannot get current user")
		}
		cfg.PGSUUsername = user
	}

	if err := cmd.SetupLogging(c, &cfg.CommonConfig, debug); err != nil {
		logger.Fatal().AnErr(logging.KeyErr, err).Msg("")
	}

	if cfg.DataDir == "" {
		logger.Fatal().Msg("data dir required")
	}

	if err = store.CheckCommonConfig(&cfg.CommonConfig); err != nil {
		logger.Fatal().Msg(err.Error())
	}

	store.SetMetrics(&cfg.CommonConfig, "keeper")

	if cfg.PGListenAddress == "" {
		logger.Fatal().Msg("--pg-listen-address is required")
	}

	if cfg.PGAdvertiseAddress == "" {
		listenAddFlag = "pg-listen-address"
		cfg.PGAdvertiseAddress = cfg.PGListenAddress
	}

	if cfg.PGAdvertisePort == "" {
		cfg.PGAdvertisePort = cfg.PGPort
	}

	ip := net.ParseIP(cfg.PGAdvertiseAddress)
	if ip == nil {
		logger.Warn().
			Str("flag", listenAddFlag).
			Str("value", cfg.PGAdvertiseAddress).
			Msg("provided flag is not an ip address but a hostname. " +
				"This will be advertized to the other components and may have undefined behaviors " +
				"if resolved differently by other hosts")
	}

	ipAddr, err := net.ResolveIPAddr("ip", cfg.PGAdvertiseAddress)
	if err != nil {
		logger.Warn().
			AnErr(logging.KeyErr, err).
			Str("flag", listenAddFlag).
			Str("address", cfg.PGAdvertiseAddress).
			Msg("cannot resolve provided flag")
	} else {
		if ipAddr.IP.IsLoopback() {
			logger.Warn().
				Str("flag", listenAddFlag).
				Str("address", cfg.PGAdvertiseAddress).
				Msg("provided address for flag is a loopback ip. " +
					"This will be advertized to the other components " +
					"and communication will fail if they are on different hosts")
		}
	}

	if err = keeper.ValidateUsersConfig(ctx, &cfg); err != nil {
		logger.Fatal().Msg(err.Error())
	}

	if cfg.UID != "" {
		if !cluster.IsValidReplSlotName(cfg.UID) {
			// revive:disable-next-line
			logger.Fatal().
				Str("uid", cfg.UID).
				Msg("keeper uid not valid. " +
					"It can contain only lower-case letters, numbers and the underscore character")
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go sigHandler(ctx, sigs, cancel)

	if err = keeper.Run(ctx, cfg); err != nil {
		logger.Fatal().AnErr(logging.KeyErr, err).Msg("")
	}
}
