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
	"reflect"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/logging"
	pg "github.com/pgvillage-tools/orion/internal/postgresql"
	"github.com/pgvillage-tools/orion/internal/util"
)

func (p *PostgresKeeper) postgresKeeperSM(ctx context.Context) {
	ctx, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	e := p.e
	pgm := p.pgm

	cd, _, err := e.GetClusterData(ctx)
	if err != nil {
		logger.Error().AnErr(logging.KeyErr, err).Msg("error retrieving cluster data")
		return
	}
	logger.Debug().Any("clusterdata", cd).Msg("cd dump")

	if cd == nil {
		logger.Info().Msg("no cluster data available, waiting for it to appear")
		return
	}
	if cd.FormatVersion != cluster.CurrentCDFormatVersion {
		logger.Error().Uint64("version", cd.FormatVersion).Msg("unsupported clusterdata format version")
		return
	}
	if err = cd.Cluster.Spec.Validate(); err != nil {
		logger.Error().AnErr(logging.KeyErr, err).Msg("clusterdata validation failed")
		return
	}

	// Mark that the clusterdata we've received is valid. We'll use this metric to detect
	// when our store is failing to serve a valid clusterdata, so it's important we only
	// update the metric here.
	clusterdataLastValidUpdateSeconds.SetToCurrentTime()

	if cd.Cluster != nil {
		p.sleepInterval = cd.Cluster.DefSpec().SleepInterval.Duration
		p.requestTimeout = cd.Cluster.DefSpec().RequestTimeout.Duration

		if p.keeperLocalState.ClusterUID != cd.Cluster.UID {
			p.keeperLocalState.ClusterUID = cd.Cluster.UID
			if err = p.saveKeeperLocalState(); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("failed to save keeper local state")
				return
			}
		}
	}

	k, ok := cd.Keepers[p.keeperLocalState.UID]
	if !ok {
		logger.Info().Msg("our keeper data is not available, waiting for it to appear")
		return
	}

	db := cd.FindDB(k)
	if db == nil {
		// a spare keeper has no db for as long as it is spare: log when that starts, not on every check
		if p.noDB.changed(true) {
			logger.Info().Msg("no db assigned: postgres stays stopped until the sentinel assigns one")
		} else {
			logger.Debug().Msg("no db assigned")
		}
		if err = pgm.StopIfStarted(ctx, true); err != nil {
			logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgPgInst)
		}
		return
	}

	if p.noDB.changed(false) {
		logger.Info().Str("db", db.UID).Msg("db assigned")
	}

	if p.bootUUID != k.Status.BootUUID {
		logger.Info().
			Str("bootUUID", p.bootUUID).
			Str("clusterBootUUID", k.Status.BootUUID).
			Msg("our db boot UID is different than the cluster data one, waiting for it to be updated")
		if err = pgm.StopIfStarted(ctx, true); err != nil {
			logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgPgInst)
		}
		return
	}

	// Generate hba auth from clusterData
	pgm.SetHba(p.generateHBA(ctx, cd, db, p.waitSyncStandbysSynced))

	var pgParameters common.Parameters

	dbls := p.dbLocalStateCopy()
	if dbls.Initializing {
		// If we are here this means that the db initialization or
		// resync has failed so we have to clean up stale data
		logger.Error().Msg("db failed to initialize or resync")

		if err = pgm.StopIfStarted(ctx, true); err != nil {
			logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgPgInst)
			return
		}

		// Clean up cluster db datadir
		if err = pgm.RemoveAllIfInitialized(ctx); err != nil {
			logger.Error().AnErr(logging.KeyErr, err).Msg("failed to remove the postgres data dir")
			return
		}
		// Reset current db local state since it's not valid anymore
		ndbls := &DBLocalState{
			UID:          "",
			Generation:   cluster.NoGeneration,
			Initializing: false,
		}
		if err = p.saveDBLocalState(ndbls); err != nil {
			logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgDbState)
			return
		}
	}

	if p.dbLocalState.UID != db.UID {
		var initialized bool
		initialized, err = pgm.IsInitialized()
		if err != nil {
			logger.Error().AnErr(logging.KeyErr, err).Msg("failed to detect if instance is initialized")
			return
		}
		logger.Info().
			Str("db", p.dbLocalState.UID).
			Str("cdDB", db.UID).
			Msg("current db UID different than cluster data db UID")

		pgm.SetRecoveryOptions(nil)
		p.waitSyncStandbysSynced = false

		switch db.Spec.InitMode {
		case cluster.NewDB:
			logger.Info().Msg("initializing the database cluster")
			ndbls := &DBLocalState{
				UID: db.UID,
				// Set a no generation since we aren't already converged.
				Generation:   cluster.NoGeneration,
				Initializing: true,
			}
			if err = p.saveDBLocalState(ndbls); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgDbState)
				return
			}

			// create postgres parameters with empty InitPGParameters
			pgParameters = p.createPGParameters(ctx, db)
			// update pgm postgres parameters
			pgm.SetParameters(pgParameters)

			if err = pgm.StopIfStarted(ctx, true); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgPgInst)
				return
			}
			if err = pgm.RemoveAllIfInitialized(ctx); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("failed to remove the postgres data dir")
				return
			}
			if err = pgm.Init(ctx, db.Spec.NewConfig); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("failed to initialize postgres database cluster")
				return
			}

			if err = pgm.StartTmpMerged(ctx); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("failed to start instance")
				return
			}
			if err = pgm.WaitReady(ctx, cd.Cluster.DefSpec().DBWaitReadyTimeout.Duration); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("timeout waiting for instance to be ready")
				return
			}
			if db.Spec.IncludeConfig {
				pgParameters, err = pgm.GetConfigFilePGParameters(ctx)
				if err != nil {
					logger.Error().AnErr(logging.KeyErr, err).Msg("failed to retrieve postgres parameters")
					return
				}
				ndbls.InitPGParameters = pgParameters
				if err = p.saveDBLocalState(ndbls); err != nil {
					logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgDbState)
					return
				}
			}

			logger.Info().Msg("setting roles")
			if err = pgm.SetupRoles(ctx); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("failed to setup roles")
				return
			}

			if err = pgm.StopIfStarted(ctx, true); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgPgInst)
				return
			}
		case cluster.PITRDB:
			logger.Info().Msg("restoring the database cluster")
			ndbls := &DBLocalState{
				UID: db.UID,
				// Set a no generation since we aren't already converged.
				Generation:   cluster.NoGeneration,
				Initializing: true,
			}
			if err = p.saveDBLocalState(ndbls); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgDbState)
				return
			}

			// create postgres parameters with empty InitPGParameters
			pgParameters = p.createPGParameters(ctx, db)
			// update pgm postgres parameters
			pgm.SetParameters(pgParameters)

			if err = pgm.StopIfStarted(ctx, true); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgPgInst)
				return
			}
			if err = pgm.RemoveAllIfInitialized(ctx); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("failed to remove the postgres data dir")
				return
			}
			logger.Info().Msg("executing DataRestoreCommand")
			if err = pgm.Restore(ctx, db.Spec.PITRConfig.DataRestoreCommand); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("failed to restore postgres database cluster")
				return
			}

			recoveryMode := pg.RecoveryModeRecovery
			var standbySettings *cluster.StandbySettings
			if db.Spec.FollowConfig != nil && db.Spec.FollowConfig.Type == cluster.FollowTypeExternal {
				recoveryMode = pg.RecoveryModeStandby
				standbySettings = db.Spec.FollowConfig.StandbySettings
			}

			pgm.SetRecoveryOptions(p.createRecoveryOptions(
				recoveryMode,
				standbySettings,
				db.Spec.PITRConfig.ArchiveRecoverySettings,
				db.Spec.PITRConfig.RecoveryTargetSettings))

			if err = pgm.StartTmpMerged(ctx); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("failed to start instance")
				return
			}

			if recoveryMode == pg.RecoveryModeRecovery {
				// wait for the db having replayed all the wals
				logger.Info().Msg("waiting for recovery to be completed")
				if err = pgm.WaitRecoveryDone(cd.Cluster.DefSpec().SyncTimeout.Duration); err != nil {
					logger.Error().AnErr(logging.KeyErr, err).Msg("recovery not finished")
					return
				}
				logger.Info().Msg("recovery completed")
			}
			if err = pgm.WaitReady(ctx, cd.Cluster.DefSpec().SyncTimeout.Duration); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("timeout waiting for instance to be ready")
				return
			}

			if db.Spec.IncludeConfig {
				pgParameters, err = pgm.GetConfigFilePGParameters(ctx)
				if err != nil {
					logger.Error().AnErr(logging.KeyErr, err).Msg("failed to retrieve postgres parameters")
					return
				}
				ndbls.InitPGParameters = pgParameters
				if err = p.saveDBLocalState(ndbls); err != nil {
					logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgDbState)
					return
				}
			}

			if err = pgm.StopIfStarted(ctx, true); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgDbState)
				return
			}

		case cluster.ResyncDB:
			logger.Info().Msg("resyncing the database cluster")
			ndbls := &DBLocalState{
				// replace our current db uid with the required one.
				UID: db.UID,
				// Set a no generation since we aren't already converged.
				Generation:   cluster.NoGeneration,
				Initializing: true,
			}
			if err = p.saveDBLocalState(ndbls); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgDbState)
				return
			}

			if err = pgm.StopIfStarted(ctx, true); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgDbState)
				return
			}

			// create postgres parameters with empty InitPGParameters
			pgParameters = p.createPGParameters(ctx, db)
			// update pgm postgres parameters
			pgm.SetParameters(pgParameters)

			var systemID string
			if !initialized {
				logger.Info().Msg("database cluster not initialized")
			} else {
				systemID, err = pgm.GetSystemID()
				if err != nil {
					logger.Error().AnErr(logging.KeyErr, err).Msg("error retrieving systemd ID")
					return
				}
			}

			followedUID := db.Spec.FollowConfig.DBUID
			followedDB, ok := cd.DBs[followedUID]
			if !ok {
				logger.Error().Str(followedStr, followedUID).Msg("no db data available for followed db")
				return
			}

			tryPgrewind := true
			if !initialized {
				tryPgrewind = false
			}
			if systemID != followedDB.Status.SystemID {
				tryPgrewind = false
			}

			masterDB, ok := cd.DBs[cd.Cluster.Status.Master]
			if tryPgrewind && !ok {
				logger.Warn().Msg("no current master, disabling pg_rewind for this resync")
				tryPgrewind = false
			}

			// TODO(sgotti) pg_rewind considers databases on the same timeline
			// as in sync and doesn't check if they diverged at different
			// position in previous timelines.
			// So check that the db as been synced or resync again with
			// pg_rewind disabled. Will need to report this upstream.

			// TODO(sgotti) The rewinded standby needs wal from the master
			// starting from the common ancestor, if they aren't available the
			// instance will keep waiting for them, now we assume that if the
			// instance isn't ready after the start timeout, it's waiting for
			// wals and we'll force a full resync.
			// We have to find a better way to detect if a standby is waiting
			// for unavailable wals.
			if err = p.resync(ctx, db, masterDB, followedDB, tryPgrewind); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("failed to resync from followed instance")
				return
			}
			if err = pgm.Start(ctx); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("failed to start instance")
				return
			}

			if tryPgrewind {
				fullResync := false
				// if not accepting connection assume that it's blocked waiting for missing wal
				// (see above TODO), so do a full resync using pg_basebackup.
				if err = pgm.WaitReady(ctx, cd.Cluster.DefSpec().DBWaitReadyTimeout.Duration); err != nil {
					// revive:disable-next-line
					logger.Warn().Msg("pg_rewinded standby is not accepting connection. it's probably waiting for unavailable wals. Forcing a full resync")
					fullResync = true
				} else {
					// Check again if it was really synced
					var pgState *cluster.PostgresState
					pgState, err = p.GetPGState(ctx)
					if err != nil {
						logger.Error().AnErr(logging.KeyErr, err).Msg("cannot get current pgstate")
						return
					}
					if p.isDifferentTimelineBranch(ctx, followedDB, pgState) {
						fullResync = true
					}
				}

				if fullResync {
					if err = pgm.StopIfStarted(ctx, true); err != nil {
						logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgDbState)
						return
					}
					if err = p.resync(ctx, db, masterDB, followedDB, false); err != nil {
						logger.Error().AnErr(logging.KeyErr, err).Msg("failed to resync from followed instance")
						return
					}
				}
			}

		case cluster.ExistingDB:
			ndbls := &DBLocalState{
				// replace our current db uid with the required one.
				UID: db.UID,
				// Set a no generation since we aren't already converged.
				Generation:   cluster.NoGeneration,
				Initializing: false,
			}
			if err = p.saveDBLocalState(ndbls); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgDbState)
				return
			}

			// create postgres parameters with empty InitPGParameters
			pgParameters = p.createPGParameters(ctx, db)
			// update pgm postgres parameters
			pgm.SetParameters(pgParameters)

			if err = pgm.StopIfStarted(ctx, true); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgDbState)
				return
			}
			if err = pgm.StartTmpMerged(ctx); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("failed to start instance")
				return
			}
			if err = pgm.WaitReady(ctx, cd.Cluster.DefSpec().DBWaitReadyTimeout.Duration); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("timeout waiting for instance to be ready")
				return
			}
			if db.Spec.IncludeConfig {
				pgParameters, err = pgm.GetConfigFilePGParameters(ctx)
				if err != nil {
					logger.Error().AnErr(logging.KeyErr, err).Msg("failed to retrieve postgres parameters")
					return
				}
				ndbls.InitPGParameters = pgParameters
				if err = p.saveDBLocalState(ndbls); err != nil {
					logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgDbState)
					return
				}
			}
			if err = pgm.StopIfStarted(ctx, true); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgDbState)
				return
			}
		case cluster.NoDB:
			// revive:disable-next-line
			logger.Error().Msg("different local dbUID but init mode is none, this shouldn't happen. Something bad happened to the keeper data. Check that keeper data is on a persistent volume and that the keeper state files weren't removed")
			return
		default:
			logger.Error().Str("initMode", string(db.Spec.InitMode)).Msg("unknown db init mode")
			return
		}
	}

	initialized, err := pgm.IsInitialized()
	if err != nil {
		logger.Error().AnErr(logging.KeyErr, err).Msg("failed to detect if instance is initialized")
		return
	}

	if initialized {
		var started bool
		started, err = pgm.IsStarted()
		if err != nil {
			// log error getting instance state but go ahead.
			logger.Error().AnErr(logging.KeyErr, err).Msg("failed to retrieve instance status")
		}
		logger.Debug().Bool("initialized", true).Bool("started", started).Msg("db status")
	} else {
		logger.Debug().Bool("initialized", false).Bool("started", false).Msg("db status")
	}

	// create postgres parameters
	pgParameters = p.createPGParameters(ctx, db)
	// update pgm postgres parameters
	pgm.SetParameters(pgParameters)

	var localRole common.Role
	if !initialized {
		logger.Info().Msg("database cluster not initialized")
		localRole = common.RoleUndefined
	} else {
		localRole, err = pgm.GetRole()
		if err != nil {
			logger.Error().AnErr(logging.KeyErr, err).Msg("error retrieving current pg role")
			return
		}
	}

	targetRole := db.Spec.Role
	logger.Debug().Str("targetRole", string(targetRole)).Msg("target role")

	// Set metrics to power alerts about mismatched roles
	setRole(localRoleGauge, &localRole)
	setRole(targetRoleGauge, &targetRole)

	switch targetRole {
	case common.RolePrimary:
		// We are the elected master
		logger.Debug().Msg("our db requested role is master")
		if localRole == common.RoleUndefined {
			logger.Error().Msg("database cluster not initialized but requested role is master. This shouldn't happen!")
			return
		}

		pgm.SetRecoveryOptions(nil)

		started, err := pgm.IsStarted()
		if err != nil {
			logger.Error().AnErr(logging.KeyErr, err).Msg("failed to retrieve instance status")
			return
		}
		if !started {
			// if we have syncrepl enabled and the postgres instance is stopped, before opening connections to normal
			// users wait for having the defined synchronousStandbys in sync state.
			if db.Spec.SynchronousReplication {
				p.waitSyncStandbysSynced = true
				// revive:disable-next-line
				logger.Info().Msg("not allowing connection as normal users since synchronous replication is enabled and instance was down")
				pgm.SetHba(p.generateHBA(ctx, cd, db, true))
			}

			if err = pgm.Start(ctx); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("failed to start postgres")
				return
			}
			if err = pgm.WaitReady(ctx, cd.Cluster.DefSpec().DBWaitReadyTimeout.Duration); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("timeout waiting for instance to be ready")
				return
			}
		}

		if localRole == common.RoleReplica {
			logger.Info().Msg("promoting to master")
			if err = pgm.Promote(ctx); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("failed to promote instance")
				return
			}
		} else {
			logger.Debug().Msg("already master")
		}

		if err := p.refreshReplicationSlots(ctx, cd, db); err != nil {
			logger.Error().AnErr(logging.KeyErr, err).Msg("error updating replication slots")
			return
		}

	case common.RoleReplica:
		// We are a standby
		var standbySettings *cluster.StandbySettings
		switch db.Spec.FollowConfig.Type {
		case cluster.FollowTypeInternal:
			followedUID := db.Spec.FollowConfig.DBUID
			logger.Debug().Str(followedStr, followedUID).Msg("our db requested role is standby")
			followedDB, ok := cd.DBs[followedUID]
			if !ok {
				logger.Error().Str(followedStr, followedUID).Msg("no db data available for followed db")
				return
			}
			replConnParams := p.getReplConnParams(db, followedDB)
			standbySettings = &cluster.StandbySettings{
				PrimaryConninfo: replConnParams.ConnString(),
				PrimarySlotName: common.OrionName(db.UID)}
		case cluster.FollowTypeExternal:
			standbySettings = db.Spec.FollowConfig.StandbySettings
		default:
			logger.Error().Str("followType", string(db.Spec.FollowConfig.Type)).Msg("unknown follow type")
			return
		}
		switch localRole {
		case common.RolePrimary:
			logger.Error().Msg("cannot move from master role to standby role")
			return
		case common.RoleReplica:
			logger.Debug().Msg("already standby")
			started, err := pgm.IsStarted()
			if err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("failed to retrieve instance status")
				return
			}
			if !started {
				pgm.SetRecoveryOptions(p.createRecoveryOptions(pg.RecoveryModeStandby, standbySettings, nil, nil))
				if err = pgm.Start(ctx); err != nil {
					logger.Error().AnErr(logging.KeyErr, err).Msg("failed to start postgres")
					return
				}
			}

			// Update our primary_conninfo if replConnString changed
			switch db.Spec.FollowConfig.Type {
			case cluster.FollowTypeInternal:
				followedUID := db.Spec.FollowConfig.DBUID
				followedDB, ok := cd.DBs[followedUID]
				if !ok {
					logger.Error().Str(followedStr, followedUID).Msg("no db data available for followed db")
					return
				}
				newReplConnParams := p.getReplConnParams(db, followedDB)
				logger.Debug().Any("newReplConnParams", newReplConnParams).Msg("newReplConnParams")

				standbySettings := &cluster.StandbySettings{
					PrimaryConninfo: newReplConnParams.ConnString(),
					PrimarySlotName: common.OrionName(db.UID)}

				curRecoveryOptions := pgm.CurRecoveryOptions()
				newRecoveryOptions := p.createRecoveryOptions(pg.RecoveryModeStandby, standbySettings, nil, nil)

				// Update recovery conf if parameters has changed
				if !curRecoveryOptions.RecoveryParameters.Equals(newRecoveryOptions.RecoveryParameters) {
					logger.Info().
						Any("curRecoveryParameters", curRecoveryOptions.RecoveryParameters).
						Any("newRecoveryParameters", newRecoveryOptions.RecoveryParameters).
						Msg("recovery parameters changed, restarting postgres instance")
					pgm.SetRecoveryOptions(newRecoveryOptions)

					if err = pgm.Restart(ctx, true); err != nil {
						logger.Error().AnErr(logging.KeyErr, err).Msg("failed to restart postgres instance")
						return
					}
				}

				if err = p.refreshReplicationSlots(ctx, cd, db); err != nil {
					logger.Error().AnErr(logging.KeyErr, err).Msg("error updating replication slots")
				}

			case cluster.FollowTypeExternal:
				curRecoveryOptions := pgm.CurRecoveryOptions()
				newRecoveryOptions := p.createRecoveryOptions(
					pg.RecoveryModeStandby,
					db.Spec.FollowConfig.StandbySettings,
					db.Spec.FollowConfig.ArchiveRecoverySettings,
					nil)

				// Update recovery conf if parameters has changed
				if !curRecoveryOptions.RecoveryParameters.Equals(newRecoveryOptions.RecoveryParameters) {
					logger.Info().
						Any("curRecoveryParameters", curRecoveryOptions.RecoveryParameters).
						Any("newRecoveryParameters", newRecoveryOptions.RecoveryParameters).
						Msg("recovery parameters changed, restarting postgres instance")
					pgm.SetRecoveryOptions(newRecoveryOptions)

					if err = pgm.Restart(ctx, true); err != nil {
						logger.Error().AnErr(logging.KeyErr, err).Msg("failed to restart postgres instance")
						return
					}
				}

				if err = p.refreshReplicationSlots(ctx, cd, db); err != nil {
					logger.Error().AnErr(logging.KeyErr, err).Msg("error updating replication slots")
				}
			}

		case common.RoleUndefined:
			logger.Info().Msg("our db role is none")
			return
		}
	case common.RoleUndefined:
		logger.Info().Msg("our db requested role is none")
		return
	}

	// update pg parameters
	pgParameters = p.createPGParameters(ctx, db)

	// Log synchronous replication changes
	prevSyncStandbyNames := pgm.CurParameters()["synchronous_standby_names"]
	syncStandbyNames := pgParameters["synchronous_standby_names"]
	if db.Spec.SynchronousReplication {
		if prevSyncStandbyNames != syncStandbyNames {
			logger.Info().
				Str("prevSyncStandbyNames", prevSyncStandbyNames).
				Str("syncStandbyNames", syncStandbyNames).
				Msg("needed synchronous_standby_names changed")
		}
	} else {
		if prevSyncStandbyNames != "" {
			logger.Info().
				Str("syncStandbyNames", prevSyncStandbyNames).
				Msg("sync replication disabled, removing current synchronous_standby_names")
		}
	}

	needsReload := false
	changedParams := pgParameters.Diff(pgm.CurParameters())

	if !pgParameters.Equals(pgm.CurParameters()) {
		logger.Info().Msg("postgres parameters changed, reloading postgres instance")
		pgm.SetParameters(pgParameters)
		needsReload = true
	} else {
		logger.Debug().Msg("postgres parameters not changed")
	}

	// Generate hba auth from clusterData

	// if we have syncrepl enabled and the postgres instance is stopped, before opening connections to normal users
	// wait for having the defined synchronousStandbys in sync state.
	if db.Spec.SynchronousReplication && p.waitSyncStandbysSynced {
		inSyncStandbys, err := p.GetInSyncStandbys(ctx)
		if err != nil {
			logger.Error().AnErr(logging.KeyErr, err).Msg("failed to retrieve current in sync standbys from instance")
			return
		}
		if !util.CompareStringSliceNoOrder(inSyncStandbys, db.Spec.SynchronousStandbys) {
			// revive:disable-next-line
			logger.Info().Msg("not allowing connection as normal users since synchronous replication is enabled, instance was down and not all sync standbys are synced")
		} else {
			p.waitSyncStandbysSynced = false
		}
	} else {
		p.waitSyncStandbysSynced = false
	}
	newHBA := p.generateHBA(ctx, cd, db, p.waitSyncStandbysSynced)
	if !reflect.DeepEqual(newHBA, pgm.CurHba()) {
		logger.Info().Msg("postgres hba entries changed, reloading postgres instance")
		pgm.SetHba(newHBA)
		needsReload = true
	} else {
		// the e2e tests wait for this line (they run the keeper at log level debug)
		logger.Debug().Msg("postgres hba entries not changed")
	}

	if needsReload {
		needsReloadGauge.Set(1) // mark as reload needed
		if err := pgm.Reload(ctx); err != nil {
			logger.Error().AnErr(logging.KeyErr, err).Msg("failed to reload postgres instance")
		} else {
			needsReloadGauge.Set(0) // successful reload implies no longer required
		}
	}

	{
		clusterSpec := cd.Cluster.DefSpec()
		automaticPgRestartEnabled := *clusterSpec.AutomaticPgRestart

		needsRestart, err := pgm.IsRestartRequired(ctx, changedParams)
		if err != nil {
			logger.Error().AnErr(logging.KeyErr, err).Msg("failed to check if restart is required")
		}

		if needsRestart {
			needsRestartGauge.Set(1) // mark as restart needed
			if automaticPgRestartEnabled {
				logger.Info().Msg("restarting postgres")
				if err := pgm.Restart(ctx, true); err != nil {
					logger.Error().AnErr(logging.KeyErr, err).Msg("failed to restart postgres instance")
				} else {
					needsRestartGauge.Set(0) // successful restart implies no longer required
				}
			}
		}
	}

	// If we are here, then all went well and we can update the db generation and save it locally
	ndbls := p.dbLocalStateCopy()
	ndbls.Generation = db.Generation
	ndbls.Initializing = false
	if err := p.saveDBLocalState(ndbls); err != nil {
		logger.Error().AnErr(logging.KeyErr, err).Msg(errorMsgDbState)
		return
	}

	// We want to set this only if no error has occurred. We should be able to identify
	// keeper issues by watching for this value becoming stale.
	lastSyncSuccessSeconds.SetToCurrentTime()
}
