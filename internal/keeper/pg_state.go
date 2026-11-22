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

package keeper

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/logging"
	pg "github.com/pgvillage-tools/orion/internal/postgresql"
	"github.com/pgvillage-tools/orion/internal/util"

	"github.com/davecgh/go-spew/spew"
)

func (p *PostgresKeeper) updateKeeperInfo(ctx context.Context) error {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	p.localStateMutex.Lock()
	keeperUID := p.keeperLocalState.UID
	clusterUID := p.keeperLocalState.ClusterUID
	p.localStateMutex.Unlock()

	if clusterUID == "" {
		return nil
	}

	var binaryVersion cluster.PostgresBinaryVersion
	version, err := p.pgm.BinaryVersion()
	if err != nil {
		// in case we fail to parse the binary version then log it and just report maj and min as 0
		logger.Warn().AnErr(logging.KeyErr, err).Msg("failed to get postgres binary version")
	} else {
		binaryVersion = cluster.PostgresBinaryVersion{Maj: int(version.Major()), Min: int(version.Minor())}
	}

	keeperInfo := &cluster.KeeperInfo{
		InfoUID:               common.UID(),
		UID:                   keeperUID,
		ClusterUID:            clusterUID,
		BootUUID:              p.bootUUID,
		PostgresBinaryVersion: binaryVersion,
		PostgresState:         p.getLastPGState(ctx),

		CanBeMaster:             p.canBeMaster,
		CanBeSynchronousReplica: p.canBeSynchronousReplica,
	}

	// The time to live is just to automatically remove old entries, it's
	// not used to determine if the keeper info has been updated.
	return p.e.SetKeeperInfo(ctx, keeperUID, keeperInfo, p.sleepInterval)
}

func (p *PostgresKeeper) updatePGState(ctx context.Context) {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	p.pgStateMutex.Lock()
	defer p.pgStateMutex.Unlock()
	pgState, err := p.GetPGState(ctx)
	if err != nil {
		logger.Error().AnErr(logging.KeyErr, err).Msg("failed to get pg state")
	}
	p.lastPGState = pgState
}

// parseSynchronousStandbyNames extracts the standby names from the
// "synchronous_standby_names" postgres parameter.
//
// Since postgres 9.6 (https://www.postgresql.org/docs/9.6/static/runtime-config-replication.html)
// `synchronous_standby_names` can be in one of two formats:
//
//	num_sync ( standby_name [, ...] )
//	standby_name [, ...]
//
// two examples for this:
//
//	2 (node1,node2)
//	node1,node2
//
// TODO(sgotti) since postgres 10 (https://www.postgresql.org/docs/10/static/runtime-config-replication.html)
// `synchronous_standby_names` can be in one of three formats:
//
//	[FIRST] num_sync ( standby_name [, ...] )
//	ANY num_sync ( standby_name [, ...] )
//	standby_name [, ...]
//
// since we are writing ourself the synchronous_standby_names we don't handle this case.
// If needed, to better handle all the cases with also a better validation of
// standby names we could use something like the parser used by postgres
func parseSynchronousStandbyNames(s string) ([]string, error) {
	var spacesSplit []string = strings.Split(s, " ")
	var entries []string
	if len(spacesSplit) < 2 {
		// We're parsing format: standby_name [, ...]
		entries = strings.Split(s, ",")
	} else {
		// We don't know yet which of the 2 formats we're parsing
		_, err := strconv.Atoi(spacesSplit[0])
		if err == nil {
			// We're parsing format: num_sync ( standby_name [, ...] )
			rest := strings.Join(spacesSplit[1:], " ")
			inBrackets := strings.TrimSpace(rest)
			if !strings.HasPrefix(inBrackets, "(") || !strings.HasSuffix(inBrackets, ")") {
				return nil, errors.New("synchronous standby string has number but lacks brackets")
			}
			withoutBrackets := strings.TrimRight(strings.TrimLeft(inBrackets, "("), ")")
			entries = strings.Split(withoutBrackets, ",")
		} else {
			// We're parsing format: standby_name [, ...]
			entries = strings.Split(s, ",")
		}
	}
	for i, e := range entries {
		entries[i] = strings.TrimSpace(e)
	}
	return entries, nil
}

// GetInSyncStandbys is a function that returns the InSyncStandbys
func (p *PostgresKeeper) GetInSyncStandbys(ctx context.Context) ([]string, error) {
	inSyncStandbysFullName, err := p.pgm.GetSyncStandbys(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve current sync standbys status from instance: %v", err)
	}

	inSyncStandbys := []string{}
	for _, s := range inSyncStandbysFullName {
		if common.IsOrionName(s) {
			inSyncStandbys = append(inSyncStandbys, common.NameFromOrionName(s))
		}
	}

	return inSyncStandbys, nil
}

// GetPGState is a function that returns the state of the postgresserver
func (p *PostgresKeeper) GetPGState(ctx context.Context) (*cluster.PostgresState, error) {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	p.getPGStateMutex.Lock()
	defer p.getPGStateMutex.Unlock()
	// Just get one pgstate at a time to avoid exausting available connections
	pgState := &cluster.PostgresState{}

	dbls := p.dbLocalStateCopy()
	pgState.UID = dbls.UID
	pgState.Generation = dbls.Generation

	pgState.ListenAddress = p.pgAdvertiseAddress
	pgState.Port = p.pgAdvertisePort

	initialized, err := p.pgm.IsInitialized()
	if err != nil {
		return pgState, err
	}
	if initialized {
		pgParameters, err := p.pgm.GetConfigFilePGParameters(ctx)
		if err != nil {
			p.logPGStateUnavailable(ctx, err, "cannot get configured pg parameters")
			return pgState, nil
		}
		logger.Debug().Any("pgParameters", pgParameters).Msg("got configured pg parameters")
		filteredPGParameters := common.Parameters{}
		for k, v := range pgParameters {
			if !util.StringInSlice(managedPGParameters, k) {
				filteredPGParameters[k] = v
			}
		}
		logger.Debug().
			Any("filteredPGParameters", filteredPGParameters).
			Msg("filtered out managed pg parameters")
		pgState.PGParameters = filteredPGParameters

		inSyncStandbys, err := p.GetInSyncStandbys(ctx)
		if err != nil {
			p.logPGStateUnavailable(ctx, err, "failed to retrieve current in sync standbys from instance")
			return pgState, nil
		}

		pgState.SynchronousStandbys = inSyncStandbys

		sd, err := p.pgm.GetSystemData(ctx)
		if err != nil {
			p.logPGStateUnavailable(ctx, err, "error getting pg state")
			return pgState, nil
		}
		pgState.SystemID = sd.SystemID
		pgState.TimelineID = sd.TimelineID
		pgState.XLogPos = sd.XLogPos

		ctlsh, err := getTimeLinesHistory(ctx, pgState, p.pgm, maxPostgresTimelinesHistory)
		if err != nil {
			p.logPGStateUnavailable(ctx, err, "error getting timeline history")
			return pgState, nil
		}
		pgState.TimelinesHistory = ctlsh

		ow, err := p.pgm.OlderWalFile()
		if err != nil {
			logger.Warn().AnErr(logging.KeyErr, err).Msg("error getting older wal file")
		} else {
			logger.Debug().Str("filename", ow).Msg("older wal file")
			pgState.OlderWalFile = ow
		}
		pgState.Healthy = true
		if p.pgStateUnavailable.changed(false) {
			logger.Info().Msg("pg state available again")
		}
	}

	return pgState, nil
}

// logPGStateUnavailable logs that GetPGState can't query the running instance (the keeper then reports it unhealthy):
// a warning when that starts, and debug while it lasts, because it fails on every check while postgres doesn't accept
// connections (e.g. during a restart or a resync). It must be called with getPGStateMutex held.
func (p *PostgresKeeper) logPGStateUnavailable(ctx context.Context, err error, msg string) {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	if p.pgStateUnavailable.changed(true) {
		logger.Warn().AnErr(logging.KeyErr, err).Msg(msg)
		return
	}
	logger.Debug().AnErr(logging.KeyErr, err).Msg(msg)
}

func getTimeLinesHistory(
	ctx context.Context,
	pgState *cluster.PostgresState,
	pgm pg.PGManager,
	maxPostgresTimelinesHistory int) (
	cluster.PostgresTimelinesHistory,
	error) {
	ctlsh := cluster.PostgresTimelinesHistory{}
	// if timeline <= 1 then no timeline history file exists.
	if pgState.TimelineID > 1 {
		tlsh, err := pgm.GetTimelinesHistory(ctx, pgState.TimelineID)
		if err != nil {
			return ctlsh, err
		}
		if len(tlsh) > maxPostgresTimelinesHistory {
			tlsh = tlsh[len(tlsh)-maxPostgresTimelinesHistory:]
		}
		ctlsh = append(ctlsh, tlsh...)
	}
	return ctlsh, nil
}

func (p *PostgresKeeper) getLastPGState(ctx context.Context) *cluster.PostgresState {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	p.pgStateMutex.Lock()
	pgState := p.lastPGState.DeepCopy()
	p.pgStateMutex.Unlock()
	logger.Debug().Str("pgstate dump", spew.Sdump(pgState)).Msg("")
	return pgState
}
