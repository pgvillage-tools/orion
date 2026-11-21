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

package sentinel

import (
	"context"
	"fmt"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/logging"
	pg "github.com/pgvillage-tools/orion/internal/postgresql"
)

type dbType int
type dbValidity int
type dbStatus int

const (
	// TODO(sgotti) change "master" and "standby" to different name to
	// better differentiate with with master and standby db roles.
	dbTypeMaster dbType = iota
	dbTypeStandby

	dbValidityValid dbValidity = iota
	dbValidityInvalid
	dbValidityUnknown

	dbStatusGood dbStatus = iota
	dbStatusFailed
	dbStatusConverging
)

// dbType returns the db type
// A master is a db that:
// * Has a master db role or a standby db role with followtype external
// A standby is a db that:
// * Has a standby db role with followtype internal
func (s *Sentinel) dbType(cd *cluster.Data, dbUID string) dbType {
	db, ok := cd.DBs[dbUID]
	if !ok {
		panic(fmt.Errorf("requested unexisting db uid %q", dbUID))
	}
	switch db.Spec.Role {
	case common.RolePrimary:
		return dbTypeMaster
	case common.RoleReplica:
		if db.Spec.FollowConfig.Type == cluster.FollowTypeExternal {
			return dbTypeMaster
		}
		return dbTypeStandby
	default:
		panic("invalid db type in db Spec")
	}
}

// dbValidity return the validity of a db
// a db isn't valid when it has a different postgres systemdID or is on a
// different timeline branch
// dbs with CurrentGeneration == NoGeneration (0) are reported as
// dbValidityUnknown since the db status is empty.
func (s *Sentinel) dbValidity(ctx context.Context, cd *cluster.Data, dbUID string) dbValidity {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	db, ok := cd.DBs[dbUID]
	if !ok {
		panic(fmt.Errorf("requested unexisting db uid %q", dbUID))
	}

	if db.Status.CurrentGeneration == cluster.NoGeneration {
		return dbValidityUnknown
	}

	masterDB := cd.DBs[cd.Cluster.Status.Master]

	// ignore empty (not provided) systemid
	if db.Status.SystemID != "" {
		// if with a different postgres systemID it's invalid
		if db.Status.SystemID != masterDB.Status.SystemID {
			logger.Info().
				Str(logDB, db.UID).
				Str(logKeeper, db.Spec.KeeperUID).
				Str(logDbSysID, db.Status.SystemID).
				Str(logMasterSystemID, masterDB.Status.SystemID).
				Msg("invalid db since the postgres systemdID is different that the master one")
			return dbValidityInvalid
		}
	}

	// If on a different timeline branch it's invalid
	if s.isDifferentTimelineBranch(ctx, masterDB, db) {
		return dbValidityInvalid
	}

	// db is valid
	return dbValidityValid
}

func (s *Sentinel) dbCanSync(ctx context.Context, cd *cluster.Data, dbUID string) bool {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	db, ok := cd.DBs[dbUID]
	if !ok {
		panic(fmt.Errorf("requested unexisting db uid %q", dbUID))
	}
	masterDB := cd.DBs[cd.Cluster.Status.Master]

	// ignore if master doesn't provide the older wal file
	if masterDB.Status.OlderWalFile == "" {
		return true
	}

	// skip current master
	if dbUID == masterDB.UID {
		return true
	}

	// skip the standbys
	if s.dbType(cd, db.UID) != dbTypeStandby {
		return true
	}

	// only check when db isn't initializing
	if db.Generation == cluster.InitialGeneration {
		return true
	}

	// check only if the db isn't healty.
	if !db.Status.Healthy {
		return true
	}

	if db.Status.XLogPos == masterDB.Status.XLogPos {
		return true
	}

	// check only if the xlogpos isn't increasing for some time. This can also
	// happen when no writes are happening on the master but the standby should
	// be, if syncing at the same xlogpos.
	if s.isDBIncreasingXLogPos(db) {
		return true
	}

	required := pg.XlogPosToWalFileNameNoTimeline(db.Status.XLogPos)
	older, err := pg.WalFileNameNoTimeLine(masterDB.Status.OlderWalFile)
	if err != nil {
		// warn on wrong file name (shouldn't happen...)
		logger.Warn().
			Str("filename", masterDB.Status.OlderWalFile).
			Msg("wrong wal file name")
	}
	logger.Debug().
		Str(logDB, db.UID).
		Str(logKeeper, db.Spec.KeeperUID).
		Str(logRequiredWal, required).
		Str(logOldMasterWal, older).
		Msg("xlog pos isn't advancing on standby, checking if the master has the required wals")
	// compare the required wal file with the older wal file name ignoring the timelineID
	if required >= older {
		return true
	}

	logger.Info().
		Str(logDB, db.UID).
		Str(logKeeper, db.Spec.KeeperUID).
		Str(logRequiredWal, required).
		Str(logOldMasterWal, older).
		Msg("db won't be able to sync due to missing required wals on master")
	return false
}

func (s *Sentinel) dbStatus(cd *cluster.Data, dbUID string) dbStatus {
	db, ok := cd.DBs[dbUID]
	if !ok {
		panic(fmt.Errorf("requested unexisting db uid %q", dbUID))
	}

	// if keeper failed then mark as failed
	keeper := cd.Keepers[db.Spec.KeeperUID]
	if !keeper.Status.Healthy {
		return dbStatusFailed
	}

	convergenceTimeout := cd.Cluster.DefSpec().ConvergenceTimeout.Duration
	// check if db should be in init mode and adjust convergence timeout
	if db.Generation == cluster.InitialGeneration {
		if db.Spec.InitMode == cluster.ResyncDB {
			convergenceTimeout = cd.Cluster.DefSpec().SyncTimeout.Duration
		}
	}
	convergenceState := s.dbConvergenceState(db, convergenceTimeout)
	switch convergenceState {
	// if convergence failed then mark as failed
	case ConvergenceFailed:
		return dbStatusFailed
	// if converging then it's not failed (it can also be not healthy since it could be resyncing)
	case Converging:
		return dbStatusConverging
	}
	// if converged but not healthy mark as failed
	if !db.Status.Healthy {
		return dbStatusFailed
	}

	// TODO(sgotti) Check that the standby is successfully syncing with the
	// master (there can be different reasons:
	// * standby cannot connect to the master (network problems)
	// * missing wal segments (this shouldn't happen while keeping the same
	// master since we aren't removing replication slots for the life of a
	// standbydb in the cluster data, but could happen when electing a new
	// master if the elected standby db cluster doesn't have all the wals)

	// db is good
	return dbStatusGood
}

// validDBsByStatus returns the valid dbs of type typ (masters or standbys) by their status: good, failed and
// converging
func (s *Sentinel) validDBsByStatus(ctx context.Context, cd *cluster.Data, typ dbType) (
	good map[string]*cluster.DB,
	failed map[string]*cluster.DB,
	converging map[string]*cluster.DB,
) {
	good = map[string]*cluster.DB{}
	failed = map[string]*cluster.DB{}
	converging = map[string]*cluster.DB{}

	for _, db := range cd.DBs {
		// keep only valid dbs of the type
		if s.dbValidity(ctx, cd, db.UID) != dbValidityValid || s.dbType(cd, db.UID) != typ {
			continue
		}
		switch s.dbStatus(cd, db.UID) {
		case dbStatusGood:
			good[db.UID] = db
		case dbStatusFailed:
			failed[db.UID] = db
		case dbStatusConverging:
			converging[db.UID] = db
		}
	}
	return good, failed, converging
}
