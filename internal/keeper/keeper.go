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

// Package cmd holds all CLI code for the keeper
package keeper

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/consensus"
	"github.com/pgvillage-tools/orion/internal/logging"
	pg "github.com/pgvillage-tools/orion/internal/postgresql"
	"github.com/pgvillage-tools/orion/internal/store"
)

const (
	// defaultWalSegmentSizeMB is the default PostgreSQL wal segment size in MB
	defaultWalSegmentSizeMB = 16

	errorMsgPgInst    = "failed to stop pg instance"
	errorMsgDbState   = "failed to save db local state"
	followedStr       = "followedDB"
	ownerRWPermisions = 0600
	// groupOtherPermissions are the permission bits for the group and others; a password file should have none
	groupOtherPermissions   = 0o077
	ownerRWExecPermisions   = 0700
	ownerRWOtherRPermisions = 0644
	decimal                 = 10

	authTrust = "trust"
	authMd5   = "md5"
	authCert  = "cert"
	authIdent = "ident"
	authPeer  = "peer"

	connTypeHost         = "host"
	connTypeHostSsl      = "hostssl"
	connTypeHostNoSsl    = "hostnossl"
	connTypeHostGssEnc   = "hostgssenc"
	connTypeHostNoGssEnc = "hostnogssenc"

	defaultDatabase = "postgres"
)

const (
	maxPostgresTimelinesHistory = 2
	minWalKeepSegments          = 8
)

// PostgresKeeper is a struct containing information about the postgres server like
// directories, addresses and binpath etc.
type PostgresKeeper struct {
	cfg *Config

	bootUUID string

	dataDir               string
	walDir                string
	pgListenAddress       string
	pgAdvertiseAddress    string
	pgPort                string
	pgAdvertisePort       string
	pgBinPath             string
	pgReplConnType        string
	pgReplAuthMethod      string
	pgReplLocalAuthMethod string
	pgReplSslMode         string
	pgReplUsername        string
	pgReplPassword        string
	pgSUConnType          string
	pgSUAuthMethod        string
	pgSULocalAuthMethod   string
	pgSUUsername          string
	pgSUPassword          string

	sleepInterval  time.Duration
	requestTimeout time.Duration

	e   consensus.Store
	pgm *pg.Manager
	end chan error

	localStateMutex  sync.Mutex
	keeperLocalState *LocalState
	dbLocalState     *DBLocalState

	pgStateMutex    sync.Mutex
	getPGStateMutex sync.Mutex
	lastPGState     *cluster.PostgresState

	waitSyncStandbysSynced bool

	canBeMaster             *bool
	canBeSynchronousReplica *bool
}

// NewPostgresKeeper is function which makes a new postgreskeeper
func NewPostgresKeeper(ctx context.Context, cfg *Config, end chan error) (*PostgresKeeper, error) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	ctx, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	e, err := store.NewStore(ctx, &cfg.CommonConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot create store: %v", err)
	}

	// Clean and get absolute datadir path
	dataDir, err := filepath.Abs(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("cannot get absolute datadir path for %q: %v", cfg.DataDir, err)
	}

	p := &PostgresKeeper{
		cfg: cfg,

		bootUUID: common.UUID(),

		dataDir: dataDir,
		walDir:  cfg.WALDir,

		pgListenAddress:       cfg.PGListenAddress,
		pgAdvertiseAddress:    cfg.PGAdvertiseAddress,
		pgPort:                cfg.PGPort,
		pgAdvertisePort:       cfg.PGAdvertisePort,
		pgBinPath:             cfg.PGBinPath,
		pgReplConnType:        cfg.PGReplConnType,
		pgReplAuthMethod:      cfg.PGReplAuthMethod,
		pgReplLocalAuthMethod: cfg.PGReplLocalAuthMethod,
		pgReplSslMode:         cfg.PGReplSSLMode,
		pgReplUsername:        cfg.PGReplUsername,
		pgReplPassword:        cfg.PGReplPassword,
		pgSUConnType:          cfg.PGSUConnType,
		pgSUAuthMethod:        cfg.PGSUAuthMethod,
		pgSULocalAuthMethod:   cfg.PGSULocalAuthMethod,
		pgSUUsername:          cfg.PGSUUsername,
		pgSUPassword:          cfg.PGSUPassword,

		sleepInterval:  cluster.DefaultSleepInterval,
		requestTimeout: cluster.DefaultRequestTimeout,

		keeperLocalState: &LocalState{},
		dbLocalState:     &DBLocalState{},

		canBeMaster:             &cfg.CanBeMaster,
		canBeSynchronousReplica: &cfg.CanBeSynchronousReplica,

		e:   e,
		end: end,
	}

	err = p.loadKeeperLocalState()
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load keeper local state file: %v", err)
	}
	if p.keeperLocalState.UID != "" && p.cfg.UID != "" && p.keeperLocalState.UID != p.cfg.UID {
		logger.Fatal().
			Str("saved uid", p.keeperLocalState.UID).
			Str("config uid", cfg.UID).
			Msg("saved uid differs from configuration uid")
	}
	if p.keeperLocalState.UID == "" {
		p.keeperLocalState.UID = cfg.UID
		if cfg.UID == "" {
			p.keeperLocalState.UID = common.UID()
			logger.Info().
				Str("uid", p.keeperLocalState.UID).
				Msg("uid generated")
		}
		if err = p.saveKeeperLocalState(); err != nil {
			logger.Info().
				AnErr(logging.KeyErr, err).
				Msg("error while saving local state")
		}
	}

	logger.Info().Str("keeper uid", p.keeperLocalState.UID).Msg("")

	err = p.loadDBLocalState()
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load db local state file: %v", err)
	}
	return p, nil
}

// Start is function that starts PostgresKeeper
func (p *PostgresKeeper) Start(ctx context.Context) {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	endSMCh := make(chan struct{})
	endPgStatecheckerCh := make(chan struct{})
	endUpdateKeeperInfo := make(chan struct{})

	var err error
	var cd *cluster.Data
	cd, _, err = p.e.GetClusterData(ctx)
	if err != nil {
		logger.Error().AnErr(logging.KeyErr, err).Msg("error retrieving cluster data")
	} else if cd != nil {
		if cd.FormatVersion != cluster.CurrentCDFormatVersion {
			logger.Error().Uint64("version", cd.FormatVersion).Msg("unsupported clusterdata format version")
		} else if cd.Cluster != nil {
			p.sleepInterval = cd.Cluster.DefSpec().SleepInterval.Duration
			p.requestTimeout = cd.Cluster.DefSpec().RequestTimeout.Duration
		}
	}

	logger.Debug().Any("cd dump", cd).Msg("")

	// TODO(sgotti) reconfigure the various configurations options
	// (RequestTimeout) after a changed cluster config
	pgm := pg.NewManager(
		p.pgBinPath,
		p.dataDir,
		p.walDir,
		p.getLocalConnParams(),
		p.getLocalReplConnParams(),
		p.pgSUAuthMethod,
		p.pgSUUsername,
		p.pgSUPassword,
		p.pgReplAuthMethod,
		p.pgReplUsername,
		p.pgReplPassword,
		p.requestTimeout)

	p.pgm = pgm

	_ = p.pgm.StopIfStarted(ctx, true)

	smTimerCh := time.NewTimer(0).C
	updatePGStateTimerCh := time.NewTimer(0).C
	updateKeeperInfoTimerCh := time.NewTimer(0).C
	for {
		// The sleepInterval can be updated during normal execution. Ensure we regularly
		// refresh the metric to account for those changes.
		sleepInterval.Set(float64(p.sleepInterval / time.Second))

		select {
		case <-ctx.Done():
			logger.Debug().Msg("stopping orion keeper")
			if err = p.pgm.StopIfStarted(ctx, true); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgPgInst)
			}
			p.end <- nil
			return

		case <-smTimerCh:
			go func() {
				p.postgresKeeperSM(ctx)
				endSMCh <- struct{}{}
			}()

		case <-endSMCh:
			smTimerCh = time.NewTimer(p.sleepInterval).C

		case <-updatePGStateTimerCh:
			// updateKeeperInfo two times faster than the sleep interval
			go func() {
				p.updatePGState(ctx)
				endPgStatecheckerCh <- struct{}{}
			}()

		case <-endPgStatecheckerCh:
			// updateKeeperInfo two times faster than the sleep interval
			updatePGStateTimerCh = time.NewTimer(p.sleepInterval / 2).C

		case <-updateKeeperInfoTimerCh:
			go func() {
				if err := p.updateKeeperInfo(ctx); err != nil {
					logger.Error().AnErr(logging.KeyErr, err).Msg("failed to update keeper info")
				}
				endUpdateKeeperInfo <- struct{}{}
			}()

		case <-endUpdateKeeperInfo:
			updateKeeperInfoTimerCh = time.NewTimer(p.sleepInterval).C
		}
	}
}
