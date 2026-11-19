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
	"fmt"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/logging"
	pg "github.com/pgvillage-tools/orion/internal/postgresql"
	"github.com/rs/zerolog"
)

func (p *PostgresKeeper) usePgrewind(db *cluster.DB) bool {
	return p.pgSUUsername != "" && (p.pgSUPassword != "" || p.pgSUAuthMethod == authCert) && db.Spec.UsePgrewind
}

func (p *PostgresKeeper) resync(ctx context.Context, db, masterDB, followedDB *cluster.DB, tryPgrewind bool) error {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	pgm := p.pgm
	replConnParams := p.getReplConnParams(db, followedDB)
	standbySettings := &cluster.StandbySettings{
		PrimaryConninfo: replConnParams.ConnString(),
		PrimarySlotName: common.OrionName(db.UID)}

	// TODO(sgotti) Actually we don't check if pg_rewind is installed or if
	// postgresql version is > 9.5 since someone can also use an externally
	// installed pg_rewind for postgres 9.4. If a pg_rewind executable
	// doesn't exists pgm.SyncFromFollowedPGRewind will return an error and
	// fallback to pg_basebackup
	if tryPgrewind && p.usePgrewind(db) {
		// pg_rewind doesn't support running against a database that is in recovery, as it
		// builds temporary tables and this is not supported on a hot-standby. Orion doesn't
		// currently support cascading replication, but we should be clear when issuing a
		// rewind that it targets the current primary, rather than whatever database we
		// follow.
		connParams := p.getSUConnParams(db, masterDB)
		err := pgm.SyncFromFollowedPGRewind(ctx, connParams, p.pgSUPassword)
		logger.Info().
			Str("masterDB", masterDB.UID).
			Str("keeper", followedDB.Spec.KeeperUID).
			Msg("syncing using pg_rewind")
		if err == nil {
			pgm.SetRecoveryOptions(p.createRecoveryOptions(pg.RecoveryModeStandby, standbySettings, nil, nil))
			return nil
		}
		// log pg_rewind error and fallback to pg_basebackup
		logger.Error().AnErr(logging.KeyErr, err).Msg("error syncing with pg_rewind")
	}

	replSlot := ""
	version, err := p.pgm.BinaryVersion()
	if err != nil {
		// in case we fail to parse the binary version then log it and just don't use replSlot
		logger.Warn().AnErr(logging.KeyErr, err).Msg("failed to get postgres binary version")
	} else if version.GreaterThanEqual(pg.V96) {
		replSlot = common.OrionName(db.UID)
	}

	if err := pgm.RemoveAllIfInitialized(ctx); err != nil {
		return fmt.Errorf("failed to remove the postgres data dir: %v", err)
	}
	if logger.GetLevel() == zerolog.DebugLevel {
		logger.Debug().
			Str(followedStr, followedDB.UID).
			Str("keeper", followedDB.Spec.KeeperUID).
			Any("replConnParams", replConnParams).
			Msg("syncing from followed db")
	} else {
		logger.Info().
			Str(followedStr, followedDB.UID).
			Str("keeper", followedDB.Spec.KeeperUID).
			Msg("syncing from followed db")
	}

	if err := pgm.SyncFromFollowed(ctx, replConnParams, replSlot); err != nil {
		return fmt.Errorf("sync error: %v", err)
	}
	logger.Info().Msg("sync succeeded")

	pgm.SetRecoveryOptions(p.createRecoveryOptions(pg.RecoveryModeStandby, standbySettings, nil, nil))

	return nil
}

// TODO(sgotti) unify this with the sentinel one. They have the same logic but one uses *cluster.PostgresState
// while the other *cluster.DB
func (p *PostgresKeeper) isDifferentTimelineBranch(
	ctx context.Context, followedDB *cluster.DB, pgState *cluster.PostgresState) bool {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	if followedDB.Status.TimelineID < pgState.TimelineID {
		logger.Info().
			Uint64("followedTimeline", followedDB.Status.TimelineID).
			Uint64("timeline", pgState.TimelineID).
			Msg("followed instance timeline < than our timeline")
		return true
	}

	// if the timelines are the same check that also the switchpoints are the same.
	if followedDB.Status.TimelineID == pgState.TimelineID {
		if pgState.TimelineID <= 1 {
			// if timeline <= 1 then no timeline history file exists.
			return false
		}
		ftlh := followedDB.Status.TimelinesHistory.GetTimelineHistory(pgState.TimelineID - 1)
		tlh := pgState.TimelinesHistory.GetTimelineHistory(pgState.TimelineID - 1)
		if ftlh == nil || tlh == nil {
			// No timeline history to check
			return false
		}
		if ftlh.SwitchPoint == tlh.SwitchPoint {
			return false
		}
		logger.Info().
			Uint64("followedTimeline", followedDB.Status.TimelineID).
			Uint64("followedXlogpos", ftlh.SwitchPoint).
			Uint64("timeline", pgState.TimelineID).
			Uint64("xlogpos", tlh.SwitchPoint).
			Msg("followed instance timeline forked at a different xlog pos than our timeline")
		return true
	}

	// followedDB.Status.TimelineID > pgState.TimelineID
	ftlh := followedDB.Status.TimelinesHistory.GetTimelineHistory(pgState.TimelineID)
	if ftlh != nil {
		if ftlh.SwitchPoint < pgState.XLogPos {
			logger.Info().
				Uint64("followedTimeline", followedDB.Status.TimelineID).
				Uint64("followedXlogpos", ftlh.SwitchPoint).
				Uint64("timeline", pgState.TimelineID).
				Uint64("xlogpos", pgState.XLogPos).
				Msg("followed instance timeline forked before our current state")
			return true
		}
	}
	return false
}
