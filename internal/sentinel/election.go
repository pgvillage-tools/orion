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
	"errors"
	"sort"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/logging"

	"github.com/davecgh/go-spew/spew"
)

func (s *Sentinel) findInitialKeeper(cd *cluster.Data) (*cluster.Keeper, error) {
	if len(cd.Keepers) < 1 {
		return nil, errors.New("no keepers registered")
	}
	r := s.RandFn(len(cd.Keepers))
	keys := []string{}
	for k := range cd.Keepers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return cd.Keepers[keys[r]], nil
}

// isDifferentTimelineBranch tells if db is on another timeline branch than followedDB (see
// cluster.TimelinePosition.OnOtherBranch), and logs why
func (s *Sentinel) isDifferentTimelineBranch(ctx context.Context, followedDB *cluster.DB, db *cluster.DB) bool {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	other, reason := dbTimelinePosition(db).OnOtherBranch(dbTimelinePosition(followedDB))
	if other {
		logger.Info().Str("db", db.UID).Str("followedDB", followedDB.UID).Msg(reason)
	}
	return other
}

// dbTimelinePosition returns the timeline position of db, from its status
func dbTimelinePosition(db *cluster.DB) cluster.TimelinePosition {
	return cluster.TimelinePosition{
		TimelineID:       db.Status.TimelineID,
		XLogPos:          db.Status.XLogPos,
		TimelinesHistory: db.Status.TimelinesHistory,
	}
}

// isLagBelowMax checks if the db reported lag is below MaxStandbyLag from the
// master reported lag
func (s *Sentinel) isLagBelowMax(ctx context.Context, cd *cluster.Data, curMasterDB, db *cluster.DB) bool {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	logger.Debug().
		Uint64("curMasterDB.Status.XLogPos", curMasterDB.Status.XLogPos).
		Uint64("db.STatus.XLogPod", db.Status.XLogPos).
		Uint64("delta", curMasterDB.Status.XLogPos-db.Status.XLogPos).
		Msg("")
	if int64(curMasterDB.Status.XLogPos-db.Status.XLogPos) > int64(*cd.Cluster.DefSpec().MaxStandbyLag) {
		logger.Debug().
			Str(logDB, db.UID).
			Uint64("dbXLogPos", db.Status.XLogPos).
			Uint64("masterXLogPos", curMasterDB.Status.XLogPos).
			Msg("ignoring keeper since its behind that maximum xlog position")
		return false
	}
	return true
}

func (s *Sentinel) freeKeepers(cd *cluster.Data) []*cluster.Keeper {
	freeKeepers := []*cluster.Keeper{}
K:
	for _, keeper := range cd.Keepers {
		if !keeper.Status.Healthy {
			continue
		}
		for _, db := range cd.DBs {
			if db.Spec.KeeperUID == keeper.UID {
				continue K
			}
		}
		freeKeepers = append(freeKeepers, keeper)
	}
	return freeKeepers
}

// dbSlice implements sort interface to sort by XLogPos
type dbSlice []*cluster.DB

func (p dbSlice) Len() int           { return len(p) }
func (p dbSlice) Less(i, j int) bool { return p[i].Status.XLogPos < p[j].Status.XLogPos }
func (p dbSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func (s *Sentinel) findBestStandbys(ctx context.Context, cd *cluster.Data, masterDB *cluster.DB) []*cluster.DB {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	goodStandbys, _, _ := s.validDBsByStatus(ctx, cd, dbTypeStandby)
	bestDBs := []*cluster.DB{}
	for _, db := range goodStandbys {
		if db.Status.TimelineID != masterDB.Status.TimelineID {
			logger.Debug().
				Str(logDB, db.UID).
				Uint64("dbTimeline", db.Status.TimelineID).
				Uint64("masterTimeline", masterDB.Status.TimelineID).
				Msg("ignoring keeper since its pg timeline is different than master timeline")
			continue
		}
		// do this only when not using synchronous replication since in sync repl we
		// have to ignore the last reported xlogpos or valid sync standby will be
		// skipped
		if !s.syncRepl(cd.Cluster.DefSpec()) {
			if !s.isLagBelowMax(ctx, cd, masterDB, db) {
				logger.Debug().
					Str(logDB, db.UID).
					Uint64("dbXLogPos", db.Status.XLogPos).
					Uint64("masterXLogPos", masterDB.Status.XLogPos).
					Msg("ignoring keeper since its lag is above the max configured lag")
				continue
			}
		}
		bestDBs = append(bestDBs, db)
	}
	// Sort by XLogPos, the highest (the least behind) first
	sort.Sort(sort.Reverse(dbSlice(bestDBs)))
	return bestDBs
}

// findBestNewMasters identifies the DBs that are elegible to become a new master. We do
// this by selecting from valid standbys (those keepers that follow the same timeline as
// our master, and have an acceptable replication lag) and also selecting from those nodes
// that are valid to become master by their status.
func (s *Sentinel) findBestNewMasters(ctx context.Context, cd *cluster.Data, masterDB *cluster.DB) []*cluster.DB {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	bestNewMasters := []*cluster.DB{}
	for _, db := range s.findBestStandbys(ctx, cd, masterDB) {
		if k, ok := cd.Keepers[db.Spec.KeeperUID]; ok && (k.Status.CanBeMaster != nil && !*k.Status.CanBeMaster) {
			logger.Info().
				Str(logDB, db.UID).
				Str(logKeeper, db.Spec.KeeperUID).
				Msg("ignoring keeper since it cannot be master (--can-be-master=false)")
			continue
		}

		bestNewMasters = append(bestNewMasters, db)
	}

	// Add the previous masters to the best standbys (if valid and in good state)
	validMastersByStatus, _, _ := s.validDBsByStatus(ctx, cd, dbTypeMaster)
	logger.Debug().Any("validMastersByStatus", spew.Sdump(validMastersByStatus)).Msg("validMastersByStatus")
	for _, db := range validMastersByStatus {
		if db.UID == masterDB.UID {
			logger.Debug().
				Str(logDB, db.UID).
				Str(logKeeper, db.Spec.KeeperUID).
				Msg("ignoring db since it's the current master")
			continue
		}

		if db.Status.TimelineID != masterDB.Status.TimelineID {
			logger.Debug().
				Str(logDB, db.UID).
				Uint64("dbTimeline", db.Status.TimelineID).
				Uint64("masterTimeline", masterDB.Status.TimelineID).
				Msg("ignoring keeper since its pg timeline is different than master timeline")
			continue
		}

		// do this only when not using synchronous replication since in sync repl we
		// have to ignore the last reported xlogpos or valid sync standby will be
		// skipped
		if !s.syncRepl(cd.Cluster.DefSpec()) {
			if !s.isLagBelowMax(ctx, cd, masterDB, db) {
				logger.Debug().
					Str(logDB, db.UID).
					Uint64("dbXLogPos", db.Status.XLogPos).
					Uint64("masterXLogPos", masterDB.Status.XLogPos).
					Msg("ignoring keeper since its lag is above the max configured lag")
				continue
			}
		}

		bestNewMasters = append(bestNewMasters, db)
	}

	// Sort by XLogPos, the highest first: with async replication the first one is elected, so the least data is lost
	sort.Sort(sort.Reverse(dbSlice(bestNewMasters)))
	logger.Debug().Any("bestNewMasters", spew.Sdump(bestNewMasters)).Msg("bestNewMasters")
	return bestNewMasters
}
