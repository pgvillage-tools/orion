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
	"strconv"
	"strings"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/logging"
	pg "github.com/pgvillage-tools/orion/internal/postgresql"
	"github.com/pgvillage-tools/orion/internal/util"
)

var managedPGParameters = []string{
	"unix_socket_directories",
	"wal_keep_segments",
	"wal_keep_size",
	"hot_standby",
	"listen_addresses",
	"port",
	"max_replication_slots",
	"max_wal_senders",
	"wal_log_hints",
	"synchronous_standby_names",

	// parameters moved from recovery.conf to postgresql.conf in PostgresSQL 12
	"primary_conninfo",
	"primary_slot_name",
	"recovery_min_apply_delay",
	"restore_command",
	"recovery_target_timeline",
	"recovery_target",
	"recovery_target_lsn",
	"recovery_target_name",
	"recovery_target_time",
	"recovery_target_xid",
	"recovery_target_timeline",
	"recovery_target_action",
}

// walLevel returns the wal_level value to use.
// if there's an user provided wal_level pg parameters and if its value is
// "logical" then returns it, otherwise returns the default ("hot_standby" for
// pg < 9.6 or "replica" for pg >= 9.6).
func (p *PostgresKeeper) walLevel(ctx context.Context, db *cluster.DB) string {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	var additionalValidWalLevels = []string{
		"logical", // pg >= 10
	}

	version, err := p.pgm.BinaryVersion()
	if err != nil {
		// in case we fail to parse the binary version then log it and just use "hot_standby"
		// that works for all versions
		logger.Warn().AnErr(logging.KeyErr, err).Msg("failed to get postgres binary version")
		return "hot_standby"
	}

	// set default wal_level
	walLevel := "hot_standby"
	if version.GreaterThanEqual(pg.V96) {
		walLevel = "replica"
	}

	if db.Spec.PGParameters != nil {
		if l, ok := db.Spec.PGParameters["wal_level"]; ok {
			if util.StringInSlice(additionalValidWalLevels, l) {
				walLevel = l
			}
		}
	}

	return walLevel
}

func (p *PostgresKeeper) walKeepSegments(db *cluster.DB) int {
	walKeepSegments := minWalKeepSegments
	if db.Spec.PGParameters != nil {
		if v, ok := db.Spec.PGParameters["wal_keep_segments"]; ok {
			// ignore wrong wal_keep_segments values
			if configuredWalKeepSegments, err := strconv.Atoi(v); err == nil {
				if configuredWalKeepSegments > walKeepSegments {
					walKeepSegments = configuredWalKeepSegments
				}
			}
		}
	}

	return walKeepSegments
}

func (p *PostgresKeeper) walKeepSize(db *cluster.DB) string {
	// assume default 16Mib wal segment size
	walKeepSize := strconv.Itoa(minWalKeepSegments * defaultWalSegmentSizeMB)

	// TODO(sgotti) currently we ignore if wal_keep_size value is less than our
	// min value or wrong and just return it as is
	if db.Spec.PGParameters != nil {
		if v, ok := db.Spec.PGParameters["wal_keep_size"]; ok {
			return v
		}
	}

	return walKeepSize
}

func (p *PostgresKeeper) mandatoryPGParameters(ctx context.Context, db *cluster.DB) common.Parameters {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	params := common.Parameters{
		"unix_socket_directories": common.PgUnixSocketDirectories,
		"wal_level":               p.walLevel(ctx, db),
		"hot_standby":             "on",
	}

	version, err := p.pgm.BinaryVersion()
	if err != nil {
		// in case we fail to parse the binary version don't return any wal_keep_segments or wal_keep_size
		logger.Warn().AnErr(logging.KeyErr, err).Msg("failed to get postgres binary version")
		return params
	}

	if version.GreaterThanEqual(pg.V13) {
		params["wal_keep_size"] = p.walKeepSize(db)
	} else {
		params["wal_keep_segments"] = fmt.Sprintf("%d", p.walKeepSegments(db))
	}

	return params
}

func (p *PostgresKeeper) createPGParameters(ctx context.Context, db *cluster.DB) common.Parameters {
	parameters := common.Parameters{}

	// Include init parameters if include config is required
	dbls := p.dbLocalStateCopy()
	if db.Spec.IncludeConfig {
		for k, v := range dbls.InitPGParameters {
			parameters[k] = v
		}
	}

	// Copy user defined pg parameters
	for k, v := range db.Spec.PGParameters {
		parameters[k] = v
	}

	// Add/Replace mandatory PGParameters
	for k, v := range p.mandatoryPGParameters(ctx, db) {
		parameters[k] = v
	}

	parameters["listen_addresses"] = p.pgListenAddress

	parameters["port"] = p.pgPort
	// TODO(sgotti) max_replication_slots needs to be at least the
	// number of existing replication slots or startup will
	// fail.
	// TODO(sgotti) changing max_replication_slots requires an
	// instance restart.
	parameters["max_replication_slots"] = strconv.FormatUint(uint64(db.Spec.MaxStandbys), decimal)
	// Add some more wal senders, since also the keeper will use them
	// TODO(sgotti) changing max_wal_senders requires an instance restart.
	parameters["max_wal_senders"] = strconv.FormatUint(uint64((db.Spec.MaxStandbys*2)+2+db.Spec.AdditionalWalSenders),
		decimal)

	// required by pg_rewind (if data checksum is enabled it's ignored)
	if db.Spec.UsePgrewind {
		parameters["wal_log_hints"] = "on"
	}

	// Setup synchronous replication
	if db.Spec.SynchronousReplication &&
		(len(db.Spec.SynchronousStandbys) > 0 ||
			len(db.Spec.ExternalSynchronousStandbys) > 0) {
		synchronousStandbys := []string{}
		for _, synchronousStandby := range db.Spec.SynchronousStandbys {
			synchronousStandbys = append(synchronousStandbys, common.OrionName(synchronousStandby))
		}
		synchronousStandbys = append(synchronousStandbys, db.Spec.ExternalSynchronousStandbys...)

		// We deliberately don't use postgres FIRST or ANY methods with N
		// different than len(synchronousStandbys) because we need that all the
		// defined standbys are synchronous (so just only one failed standby
		// will block the primary).
		// This is needed for consistency. If we have 3 standbys and we use
		// FIRST 2 (a, b, c), the sentinel, when the master fails, won't be able to know
		// which of the 3 standbys is really synchronous and in sync with the
		// master. And choosing the non synchronous one will cause the loss of
		// the transactions contained in the wal records not transmitted.
		if len(synchronousStandbys) > 1 {
			parameters["synchronous_standby_names"] = fmt.Sprintf(
				"%d (%s)",
				len(synchronousStandbys),
				strings.Join(synchronousStandbys,
					","))
		} else {
			parameters["synchronous_standby_names"] = strings.Join(synchronousStandbys, ",")
		}
	} else {
		parameters["synchronous_standby_names"] = ""
	}

	return parameters
}

func (p *PostgresKeeper) createRecoveryOptions(
	recoveryMode pg.RecoveryMode,
	standbySettings *cluster.StandbySettings,
	archiveRecoverySettings *cluster.ArchiveRecoverySettings,
	recoveryTargetSettings *cluster.RecoveryTargetSettings) *pg.RecoveryOptions {
	parameters := common.Parameters{}

	if standbySettings != nil {
		if standbySettings.PrimaryConninfo != "" {
			parameters["primary_conninfo"] = standbySettings.PrimaryConninfo
		}
		if standbySettings.PrimarySlotName != "" {
			parameters["primary_slot_name"] = standbySettings.PrimarySlotName
		}
		if standbySettings.RecoveryMinApplyDelay != "" {
			parameters["recovery_min_apply_delay"] = standbySettings.RecoveryMinApplyDelay
		}
	}

	if archiveRecoverySettings != nil {
		parameters["restore_command"] = archiveRecoverySettings.RestoreCommand
	}

	if recoveryTargetSettings == nil {
		parameters["recovery_target_timeline"] = "latest"
	} else {
		if recoveryTargetSettings.RecoveryTarget != "" {
			parameters["recovery_target"] = recoveryTargetSettings.RecoveryTarget
		}
		if recoveryTargetSettings.RecoveryTargetLsn != "" {
			parameters["recovery_target_lsn"] = recoveryTargetSettings.RecoveryTargetLsn
		}
		if recoveryTargetSettings.RecoveryTargetName != "" {
			parameters["recovery_target_name"] = recoveryTargetSettings.RecoveryTargetName
		}
		if recoveryTargetSettings.RecoveryTargetTime != "" {
			parameters["recovery_target_time"] = recoveryTargetSettings.RecoveryTargetTime
		}
		if recoveryTargetSettings.RecoveryTargetXid != "" {
			parameters["recovery_target_xid"] = recoveryTargetSettings.RecoveryTargetXid
		}
		if recoveryTargetSettings.RecoveryTargetTimeline != "" {
			parameters["recovery_target_timeline"] = recoveryTargetSettings.RecoveryTargetTimeline
		}
		parameters["recovery_target_action"] = "promote"
	}

	return &pg.RecoveryOptions{
		RecoveryMode:       recoveryMode,
		RecoveryParameters: parameters,
	}
}
