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
	"fmt"
	"reflect"
	"slices"
	"sort"
	"time"

	"github.com/Masterminds/semver/v3"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/postgresql"
	"github.com/pgvillage-tools/orion/internal/util"

	"github.com/davecgh/go-spew/spew"
)

// updateCluster returns the new cluster data: the cluster view the keepers and proxies converge to, computed from cd
// (with the status the keepers reported) and the active proxies pis. It doesn't change cd.
func (s *Sentinel) updateCluster(ctx context.Context, cd *cluster.Data,
	pis cluster.ProxiesInfo) (*cluster.Data, error) {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	// take a cd deepCopy to check that the code isn't changing it (it'll be a bug)
	origcd := cd.DeepCopy()
	newcd := cd.DeepCopy()
	clusterSpec := cd.Cluster.DefSpec()

	var err error
	switch cd.Cluster.Status.Phase {
	case cluster.Initializing:
		err = s.updateClusterInitializing(ctx, cd, newcd, clusterSpec)
	case cluster.Normal:
		err = s.updateClusterNormal(ctx, cd, newcd, clusterSpec, pis)
	default:
		return nil, fmt.Errorf("unknown cluster phase %s", cd.Cluster.Status.Phase)
	}
	if err != nil {
		return nil, err
	}

	// Copy the clusterSpec parameters to the dbSpec
	s.setDBSpecFromClusterSpec(newcd)

	// Update generation on DBs if they have changed
	for dbUID, db := range newcd.DBs {
		prevDB, ok := cd.DBs[dbUID]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(db.Spec, prevDB.Spec) {
			logger.Debug().
				Any("prevDB", spew.Sdump(prevDB.Spec)).
				Any(logDB, spew.Sdump(db.Spec)).
				Msg("db spec changed, updating generation")
			db.Generation++
		}
	}

	// check that we haven't changed the current cd or there's a bug somewhere
	if !reflect.DeepEqual(origcd, cd) {
		return nil, errors.New("cd was changed in updateCluster, this shouldn't happen")
	}
	return newcd, nil
}

// updateClusterInitializing initializes the cluster (phase Initializing) in its init mode
func (s *Sentinel) updateClusterInitializing(ctx context.Context, cd, newcd *cluster.Data,
	clusterSpec *cluster.Spec) error {
	switch *clusterSpec.InitMode {
	case cluster.New:
		return s.initNewCluster(ctx, cd, newcd, clusterSpec)
	case cluster.ExistingCluster:
		return s.initExistingCluster(ctx, cd, newcd, clusterSpec)
	case cluster.PITR:
		return s.initPITRCluster(ctx, cd, newcd, clusterSpec)
	default:
		return fmt.Errorf("unknown init mode %s", *cd.Cluster.DefSpec().InitMode)
	}
}

// initNewCluster initializes a new cluster (init mode new): it chooses a keeper for a new db, and waits until it is
// initialized
func (s *Sentinel) initNewCluster(ctx context.Context, cd, newcd *cluster.Data, clusterSpec *cluster.Spec) error {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	// Is there already a keeper choosed to be the new master?
	if cd.Cluster.Status.Master == "" {
		logger.Info().Msg("trying to find initial master")
		k, err := s.findInitialKeeper(newcd)
		if err != nil {
			return fmt.Errorf("cannot choose initial master: %v", err)
		}
		logger.Info().
			Str(logKeeper, k.UID).
			Msg("initializing cluster")
		db := &cluster.DB{
			UID:        s.UIDFn(),
			Generation: cluster.InitialGeneration,
			Spec: &cluster.DBSpec{
				KeeperUID:     k.UID,
				InitMode:      cluster.NewDB,
				NewConfig:     clusterSpec.NewConfig,
				Role:          common.RolePrimary,
				Followers:     []string{},
				IncludeConfig: *clusterSpec.MergePgParameters,
			},
		}
		newcd.DBs[db.UID] = db
		newcd.Cluster.Status.Master = db.UID
		logger.Debug().Any(logKeyNewCD, spew.Sdump(newcd)).Msg("newcd dump")
	} else {
		db, ok := newcd.DBs[cd.Cluster.Status.Master]
		if !ok {
			panic(fmt.Errorf("db %q object doesn't exists. This shouldn't happen", cd.Cluster.Status.Master))
		}
		// Check that the choosed db for being the master has correctly initialized
		switch s.dbConvergenceState(db, clusterSpec.InitTimeout.Duration) {
		case Converged:
			if db.Status.Healthy {
				logger.Info().
					Str(logDB, db.UID).
					Str(logKeeper, db.Spec.KeeperUID).
					Msg("db initialized")
				// Set db initMode to none, not needed but just a security measure
				db.Spec.InitMode = cluster.NoDB
				// Don't include previous config anymore
				db.Spec.IncludeConfig = false

				// Replace reported pg parameters in cluster spec
				if *clusterSpec.MergePgParameters {
					newcd.Cluster.Spec.PGParameters =
						mergedPGParameters(clusterSpec.PGParameters, db.Status.PGParameters)
				}
				// Cluster initialized, switch to Normal state
				newcd.Cluster.Status.Phase = cluster.Normal
			}
		case Converging:
			logger.Info().
				Str(logDB, db.UID).
				Str(logKeeper, db.Spec.KeeperUID).
				Msg("waiting for db")
		case ConvergenceFailed:
			logger.Info().
				Str(logDB, db.UID).
				Str(logKeeper, db.Spec.KeeperUID).
				Msg("db failed to initialize")
			// Empty DBs
			newcd.DBs = cluster.DBs{}
			// Unset master so another keeper can be chosen
			newcd.Cluster.Status.Master = ""
		}
	}
	return nil
}

// initExistingCluster initializes a cluster from the db of an existing keeper (init mode existing), and waits until it
// is converged
func (s *Sentinel) initExistingCluster(ctx context.Context, cd, newcd *cluster.Data, clusterSpec *cluster.Spec) error {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	if cd.Cluster.Status.Master == "" {
		wantedKeeper := clusterSpec.ExistingConfig.KeeperUID
		logger.Info().
			Str(logKeeper, wantedKeeper).
			Msg("trying to use keeper as initial master")

		k, ok := newcd.Keepers[wantedKeeper]
		if !ok {
			return fmt.Errorf("keeper %q state not available", wantedKeeper)
		}

		logger.Info().
			Str(logKeeper, k.UID).
			Msg("initializing cluster using selected keeper as master db owner")

		db := &cluster.DB{
			UID:        s.UIDFn(),
			Generation: cluster.InitialGeneration,
			Spec: &cluster.DBSpec{
				KeeperUID:     k.UID,
				InitMode:      cluster.ExistingDB,
				Role:          common.RolePrimary,
				Followers:     []string{},
				IncludeConfig: *clusterSpec.MergePgParameters,
			},
		}
		newcd.DBs[db.UID] = db
		newcd.Cluster.Status.Master = db.UID
		logger.Debug().Any(logKeyNewCD, spew.Sdump(newcd)).Msg("newcd dump")
	} else {
		db, ok := newcd.DBs[cd.Cluster.Status.Master]
		if !ok {
			panic(fmt.Errorf("db %q object doesn't exists. This shouldn't happen", cd.Cluster.Status.Master))
		}
		// Check that the choosed db for being the master has correctly initialized
		if db.Status.Healthy && s.dbConvergenceState(db, clusterSpec.ConvergenceTimeout.Duration) == Converged {
			logger.Info().
				Str(logDB, db.UID).
				Str(logKeeper, db.Spec.KeeperUID).
				Msg("db initialized") // Set db initMode to none, not needed but just a security measure
			db.Spec.InitMode = cluster.NoDB
			// Don't include previous config anymore
			db.Spec.IncludeConfig = false
			// Replace reported pg parameters in cluster spec
			if *clusterSpec.MergePgParameters {
				newcd.Cluster.Spec.PGParameters = mergedPGParameters(clusterSpec.PGParameters, db.Status.PGParameters)
			}
			// Cluster initialized, switch to Normal state
			newcd.Cluster.Status.Phase = cluster.Normal
		}
	}
	return nil
}

// initPITRCluster initializes a cluster with a point in time recovery (init mode pitr) on a keeper it chooses, and
// waits until it is converged
func (s *Sentinel) initPITRCluster(ctx context.Context, cd, newcd *cluster.Data, clusterSpec *cluster.Spec) error {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	// Is there already a keeper choosed to be the new master?
	if cd.Cluster.Status.Master == "" {
		logger.Info().Msg("trying to find initial master")
		k, err := s.findInitialKeeper(cd)
		if err != nil {
			return fmt.Errorf("cannot choose initial master: %v", err)
		}
		logger.Info().
			Str(logKeeper, k.UID).
			Msg("initializing cluster using selected keeper as master db owner")
		role := common.RolePrimary
		var followConfig *cluster.FollowConfig
		if *clusterSpec.Role == cluster.Replica {
			role = common.RoleReplica
			followConfig = &cluster.FollowConfig{
				Type:                    cluster.FollowTypeExternal,
				StandbySettings:         clusterSpec.StandbyConfig.StandbySettings,
				ArchiveRecoverySettings: clusterSpec.StandbyConfig.ArchiveRecoverySettings,
			}
		}
		db := &cluster.DB{
			UID:        s.UIDFn(),
			Generation: cluster.InitialGeneration,
			Spec: &cluster.DBSpec{
				KeeperUID:     k.UID,
				InitMode:      cluster.PITRDB,
				PITRConfig:    clusterSpec.PITRConfig,
				Role:          role,
				FollowConfig:  followConfig,
				Followers:     []string{},
				IncludeConfig: *clusterSpec.MergePgParameters,
			},
		}
		newcd.DBs[db.UID] = db
		newcd.Cluster.Status.Master = db.UID
		logger.Debug().Any(logKeyNewCD, spew.Sdump(newcd)).Msg("newcd dump")
	} else {
		db, ok := newcd.DBs[cd.Cluster.Status.Master]
		if !ok {
			panic(fmt.Errorf("db %q object doesn't exists. This shouldn't happen", cd.Cluster.Status.Master))
		}
		// Check that the choosed db for being the master has correctly initialized
		// TODO(sgotti) set a timeout (the max time for a restore operation)
		switch s.dbConvergenceState(db, 0) {
		case Converged:
			if db.Status.Healthy {
				logger.Info().
					Str(logDB, db.UID).
					Str(logKeeper, db.Spec.KeeperUID).
					Msg("db initialized")
				// Set db initMode to none, not needed but just a security measure
				db.Spec.InitMode = cluster.NoDB
				// Don't include previous config anymore
				db.Spec.IncludeConfig = false

				// Replace reported pg parameters in cluster spec
				if *clusterSpec.MergePgParameters {
					newcd.Cluster.Spec.PGParameters =
						mergedPGParameters(clusterSpec.PGParameters, db.Status.PGParameters)
				}
				// Cluster initialized, switch to Normal state
				newcd.Cluster.Status.Phase = cluster.Normal
			}
		case Converging:
			logger.Info().
				Str(logDB, db.UID).
				Str(logKeeper, db.Spec.KeeperUID).
				Msg("waiting for db to converge")
		case ConvergenceFailed:
			logger.Info().
				Str(logDB, db.UID).
				Str(logKeeper, db.Spec.KeeperUID).
				Msg("db failed to initialize")
			// Empty DBs
			newcd.DBs = cluster.DBs{}
			// Unset master so another keeper can be chosen
			newcd.Cluster.Status.Master = ""
		}
	}
	return nil
}

// updateClusterNormal manages an initialized cluster (phase Normal): it removes dead keepers, elects a new master when
// the current one failed, and manages the standbys of the master
func (s *Sentinel) updateClusterNormal(ctx context.Context, cd, newcd *cluster.Data, clusterSpec *cluster.Spec,
	pis cluster.ProxiesInfo) error {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	s.removeDeadKeepers(ctx, cd, newcd)

	// Calculate current master status
	curMasterDBUID := cd.Cluster.Status.Master
	curMasterDB := cd.DBs[curMasterDBUID]
	if curMasterDB == nil {
		return fmt.Errorf("db for keeper %q not available, this shouldn't happen", curMasterDBUID)
	}
	logger.Debug().Any("db", spew.Sdump(curMasterDB)).Msg("db dump")

	wantedMasterDBUID := s.electMaster(ctx, newcd, curMasterDB, curMasterDBUID, clusterSpec)

	// New master elected
	if curMasterDBUID != wantedMasterDBUID {
		s.switchMaster(newcd, clusterSpec, curMasterDBUID, wantedMasterDBUID)
	}

	if curMasterDBUID == wantedMasterDBUID {
		if err := s.updateMaster(ctx, cd, newcd, clusterSpec, pis, curMasterDB, curMasterDBUID); err != nil {
			return err
		}
	}

	s.updateMasterFollowers(newcd, curMasterDBUID, wantedMasterDBUID)
	return nil
}

// removeDeadKeepers removes the keepers without a db that are dead longer than the DeadKeeperRemovalInterval
func (s *Sentinel) removeDeadKeepers(ctx context.Context, cd, newcd *cluster.Data) {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	keepersToRemove := []*cluster.Keeper{}
	for _, k := range newcd.Keepers {
		// get db associated to the keeper
		db := cd.FindDB(k)
		if db != nil {
			// skip keepers with an assigned db
			continue
		}
		if time.Now().After(k.Status.LastHealthyTime.Add(cd.Cluster.DefSpec().DeadKeeperRemovalInterval.Duration)) {
			logger.Info().Str(logKeeper, k.UID).Msg("removing old dead keeper")
			keepersToRemove = append(keepersToRemove, k)
		}
	}
	for _, k := range keepersToRemove {
		delete(newcd.Keepers, k.UID)
	}
}

// electMaster returns the db that should be the master: curMasterDBUID, or the best new master when the current master
// (curMasterDB) failed or didn't converge
func (s *Sentinel) electMaster(ctx context.Context, newcd *cluster.Data, curMasterDB *cluster.DB, curMasterDBUID string,
	clusterSpec *cluster.Spec) string {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	wantedMasterDBUID := curMasterDBUID

	masterOK := true
	if !curMasterDB.Status.Healthy {
		logger.Warn().
			Str(logDB, curMasterDB.UID).
			Str(logKeeper, curMasterDB.Spec.KeeperUID).
			Msg("master db is failed")
		masterOK = false
	}

	// Check that the wanted master is in master state (i.e. check that promotion from standby to master happened)
	if s.dbConvergenceState(curMasterDB, clusterSpec.ConvergenceTimeout.Duration) == ConvergenceFailed {
		logger.Warn().
			Str(logDB, curMasterDB.UID).
			Str(logKeeper, curMasterDB.Spec.KeeperUID).
			Msg("db not converged")
		masterOK = false
	}

	if !masterOK {
		logger.Info().Msg("trying to find a new master to replace failed master")
		bestNewMasters := s.findBestNewMasters(ctx, newcd, curMasterDB)
		if len(bestNewMasters) == 0 {
			logger.Error().Msg("no eligible masters")
		} else {
			// if synchronous replication is enabled,
			// only choose new master in the synchronous replication standbys.
			var bestNewMasterDB *cluster.DB
			if curMasterDB.Spec.SynchronousReplication {
				commonSyncStandbys := util.CommonElements(
					curMasterDB.Status.SynchronousStandbys,
					curMasterDB.Spec.SynchronousStandbys)
				if len(commonSyncStandbys) == 0 {
					// revive:disable-next-line
					logger.Warn().
						Any("reported", curMasterDB.Status.SynchronousStandbys).
						Any("spec", curMasterDB.Spec.SynchronousStandbys).
						Msg("cannot choose synchronous standby since there are no common elements " +
							"between the latest master reported synchronous standbys and the db spec ones")
				} else {
					for _, nm := range bestNewMasters {
						if util.StringInSlice(commonSyncStandbys, nm.UID) {
							bestNewMasterDB = nm
							break
						}
					}
					if bestNewMasterDB == nil {
						// revive:disable-next-line
						logger.Warn().
							Any("reported", curMasterDB.Status.SynchronousStandbys).
							Any("spec", curMasterDB.Spec.SynchronousStandbys).
							Any("common", commonSyncStandbys).
							Any("possibleMasters", bestNewMasters).
							Msg("cannot choose synchronous standby since there's not match between the possible " +
								"masters and the usable synchronousStandbys")
					}
				}
			} else {
				bestNewMasterDB = bestNewMasters[0]
			}
			if bestNewMasterDB != nil {
				logger.Info().
					Str(logDB, bestNewMasterDB.UID).
					Str(logKeeper, bestNewMasterDB.Spec.KeeperUID).
					Msg("electing db as the new master")
				wantedMasterDBUID = bestNewMasterDB.UID
			} else {
				logger.Error().Msg("no eligible masters")
			}
		}
	}
	return wantedMasterDBUID
}

// switchMaster makes wantedMasterDBUID the master instead of curMasterDBUID: it tells the proxies there is no master
// until they converged, and gives the new master the synchronous standbys of the old one
func (s *Sentinel) switchMaster(newcd *cluster.Data, clusterSpec *cluster.Spec,
	curMasterDBUID, wantedMasterDBUID string) {
	// maintain the current role of the old master and just remove followers
	oldMasterdb := newcd.DBs[curMasterDBUID]
	oldMasterdb.Spec.Followers = []string{}

	masterDBRole := common.RolePrimary
	var followConfig *cluster.FollowConfig
	if *clusterSpec.Role == cluster.Replica {
		masterDBRole = common.RoleReplica
		followConfig = &cluster.FollowConfig{
			Type:                    cluster.FollowTypeExternal,
			StandbySettings:         clusterSpec.StandbyConfig.StandbySettings,
			ArchiveRecoverySettings: clusterSpec.StandbyConfig.ArchiveRecoverySettings,
		}
	}

	newcd.Cluster.Status.Master = wantedMasterDBUID
	newMasterDB := newcd.DBs[wantedMasterDBUID]
	newMasterDB.Spec.Role = masterDBRole
	newMasterDB.Spec.FollowConfig = followConfig

	// Tell proxy that there's currently no active master
	if newcd.Proxy.Spec.MasterDBUID != "" {
		// Tell proxies that there's currently no active master
		newcd.Proxy.Spec.MasterDBUID = ""
		newcd.Proxy.Generation++
	}

	// Setup synchronous standbys to the one of the previous master (replacing ourself with the previous master)
	if s.syncRepl(clusterSpec) {
		newMasterDB.Spec.SynchronousReplication = true
		newMasterDB.Spec.SynchronousStandbys = []string{}
		newMasterDB.Spec.ExternalSynchronousStandbys = []string{}
		for _, dbUID := range oldMasterdb.Spec.SynchronousStandbys {
			if dbUID != newMasterDB.UID {
				newMasterDB.Spec.SynchronousStandbys = append(
					newMasterDB.Spec.SynchronousStandbys,
					dbUID)
			} else {
				newMasterDB.Spec.SynchronousStandbys = append(
					newMasterDB.Spec.SynchronousStandbys,
					oldMasterdb.UID)
			}
		}
		if len(newMasterDB.Spec.SynchronousStandbys) == 0 && *clusterSpec.MinSynchronousStandbys > 0 {
			newMasterDB.Spec.ExternalSynchronousStandbys = []string{fakeStandbyName}
		}

		// Just sort to always have them in the same order and avoid
		// unneeded updates to synchronous_standby_names by the keeper.
		sort.Strings(newMasterDB.Spec.SynchronousStandbys)
		sort.Strings(newMasterDB.Spec.ExternalSynchronousStandbys)
	} else {
		newMasterDB.Spec.SynchronousReplication = false
		newMasterDB.Spec.SynchronousStandbys = nil
		newMasterDB.Spec.ExternalSynchronousStandbys = nil
	}
}

// updateMaster manages the current master (curMasterDBUID, which stays the master): the proxies, its role, and once it
// is healthy and converged, its standbys
func (s *Sentinel) updateMaster(ctx context.Context, cd, newcd *cluster.Data, clusterSpec *cluster.Spec,
	pis cluster.ProxiesInfo, curMasterDB *cluster.DB, curMasterDBUID string) error {
	masterDB := newcd.DBs[curMasterDBUID]
	masterDBKeeper := newcd.Keepers[masterDB.Spec.KeeperUID]

	s.updateProxySpec(ctx, newcd, pis, curMasterDBUID)

	// change master db role to "master" if the cluster role has been changed in the spec
	if *clusterSpec.Role == cluster.Primary {
		masterDB.Spec.Role = common.RolePrimary
		masterDB.Spec.FollowConfig = nil
	}

	// Set standbys to follow master only if it's healthy and converged
	if masterDB.Status.Healthy && s.dbConvergenceState(
		masterDB,
		clusterSpec.ConvergenceTimeout.Duration) == Converged {
		return s.updateStandbys(ctx, cd, newcd, clusterSpec, curMasterDB, masterDB, masterDBKeeper, curMasterDBUID)
	}
	return nil
}

// updateProxySpec points the proxies to masterDBUID once they all converged, and enables the active proxies pis
func (s *Sentinel) updateProxySpec(ctx context.Context, newcd *cluster.Data, pis cluster.ProxiesInfo,
	masterDBUID string) {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	if newcd.Proxy.Spec.MasterDBUID == "" {
		// if the Proxy.Spec.MasterDBUID is empty we have to wait for all
		// the proxies to have converged to be sure they closed connections
		// to previous master or disappear (in this case we assume that they
		// have closed connections to previous master)
		unconvergedProxiesUIDs := []string{}
		for _, pi := range pis {
			if pi.Generation != newcd.Proxy.Generation {
				unconvergedProxiesUIDs = append(unconvergedProxiesUIDs, pi.UID)
			}
		}
		if len(unconvergedProxiesUIDs) > 0 {
			logger.Info().
				Any("proxies", unconvergedProxiesUIDs).
				Msg("waiting for proxies to be converged to the current generation")
		} else {
			// Tell proxy that there's a new active master
			newcd.Proxy.Spec.MasterDBUID = masterDBUID
			newcd.Proxy.Generation++
		}
	} else {
		// if we have Proxy.Spec.MasterDBUID != "" then we have waited for
		// proxies to have converged and we can set enabled proxies to
		// the currently available proxies in proxyInfo.
		enabledProxies := []string{}
		for _, pi := range pis {
			enabledProxies = append(enabledProxies, pi.UID)
		}
		sort.Strings(enabledProxies)
		// slices.Equal: no enabled proxies (nil) and an empty list are the same, so that's no change
		if !slices.Equal(newcd.Proxy.Spec.EnabledProxies, enabledProxies) {
			newcd.Proxy.Spec.EnabledProxies = enabledProxies
			newcd.Proxy.Generation++
		}
	}
}

// updateStandbys manages the standbys of the healthy and converged master masterDB (wantedMasterDBUID): it removes old
// masters and dbs that are invalid or can't sync, sets up the synchronous standbys, keeps the number of standbys at
// MaxStandbysPerSender, and makes all standbys follow the master
func (s *Sentinel) updateStandbys(ctx context.Context, cd, newcd *cluster.Data, clusterSpec *cluster.Spec,
	curMasterDB, masterDB *cluster.DB, masterDBKeeper *cluster.Keeper, wantedMasterDBUID string) error {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	s.removeOldMasters(ctx, newcd, wantedMasterDBUID)
	s.removeInvalidDBs(ctx, newcd, wantedMasterDBUID)
	s.removeUnsyncableDBs(ctx, cd, newcd)

	goodStandbys, failedStandbys, convergingStandbys := s.validDBsByStatus(ctx, newcd, dbTypeStandby)
	goodStandbysCount := len(goodStandbys)
	failedStandbysCount := len(failedStandbys)
	convergingStandbysCount := len(convergingStandbys)
	logger.Debug().
		Int("good", goodStandbysCount).
		Int("failed", failedStandbysCount).
		Int("converging", convergingStandbysCount).
		Msg("standbys states")

	// Clean InitMode for goodStandbys
	for _, db := range goodStandbys {
		db.Spec.InitMode = cluster.NoDB
	}

	// Setup synchronous standbys
	if s.syncRepl(clusterSpec) {
		if err := s.updateSynchronousStandbys(ctx, newcd, clusterSpec, curMasterDB, masterDB, masterDBKeeper,
			goodStandbys); err != nil {
			return err
		}
	} else {
		masterDB.Spec.SynchronousReplication = false
		masterDB.Spec.SynchronousStandbys = nil
		masterDB.Spec.ExternalSynchronousStandbys = nil

		masterDB.Status.SynchronousStandbys = nil
	}

	s.updateStandbysCount(ctx, newcd, clusterSpec, masterDB, goodStandbys, goodStandbysCount,
		convergingStandbysCount, wantedMasterDBUID)
	s.reconfigureStandbys(newcd, wantedMasterDBUID)
	return nil
}

// removeOldMasters removes the dbs that are masters, other than wantedMasterDBUID
func (s *Sentinel) removeOldMasters(ctx context.Context, newcd *cluster.Data, wantedMasterDBUID string) {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	toRemove := []*cluster.DB{}
	for _, db := range newcd.DBs {
		if db.UID == wantedMasterDBUID {
			continue
		}
		if s.dbType(newcd, db.UID) != dbTypeMaster {
			continue
		}
		logger.Info().
			Str(logDB, db.UID).
			Str(logKeeper, db.Spec.KeeperUID).
			Msg("removing old master db")
		toRemove = append(toRemove, db)
	}
	for _, db := range toRemove {
		delete(newcd.DBs, db.UID)
	}
}

// removeInvalidDBs removes the invalid dbs, other than wantedMasterDBUID
func (s *Sentinel) removeInvalidDBs(ctx context.Context, newcd *cluster.Data, wantedMasterDBUID string) {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	toRemove := []*cluster.DB{}
	for _, db := range newcd.DBs {
		if db.UID == wantedMasterDBUID {
			continue
		}
		if s.dbValidity(ctx, newcd, db.UID) != dbValidityInvalid {
			continue
		}
		logger.Info().
			Str(logDB, db.UID).
			Str(logKeeper, db.Spec.KeeperUID).
			Msg("removing invalid db")
		toRemove = append(toRemove, db)
	}
	for _, db := range toRemove {
		delete(newcd.DBs, db.UID)
	}
}

// removeUnsyncableDBs removes the dbs that won't sync because the current master (in cd) misses their WAL
func (s *Sentinel) removeUnsyncableDBs(ctx context.Context, cd, newcd *cluster.Data) {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	toRemove := []*cluster.DB{}
	// judge the dbs of the current cluster data (dbCanSync looks them up there) that the new one still has
	for _, db := range cd.DBs {
		if _, ok := newcd.DBs[db.UID]; !ok || s.dbCanSync(ctx, cd, db.UID) {
			continue
		}
		logger.Info().
			Str(logDB, db.UID).
			Str(logKeeper, db.Spec.KeeperUID).
			Msg("removing db that won't be able to sync due to missing wals on current master")
		toRemove = append(toRemove, db)
	}
	for _, db := range toRemove {
		delete(newcd.DBs, db.UID)
	}
}

// updateSynchronousStandbys sets up the synchronous standbys of masterDB, from the good standbys, once the standbys it
// requires are in sync
func (s *Sentinel) updateSynchronousStandbys(ctx context.Context, newcd *cluster.Data, clusterSpec *cluster.Spec,
	curMasterDB, masterDB *cluster.DB, masterDBKeeper *cluster.Keeper, goodStandbys map[string]*cluster.DB) error {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	minSynchronousStandbys, maxSynchronousStandbys, merge, err := syncStandbyLimits(clusterSpec, masterDBKeeper)
	if err != nil {
		return err
	}

	s.refreshInSyncStandbys(ctx, newcd, masterDB, goodStandbys)

	// update synchronousStandbys only if the reported
	// SynchronousStandbys are the same as the required ones. In
	// this way, when we have to choose a new master we are sure
	// that there're no intermediate changes between the
	// reported standbys and the required ones.
	if !util.CompareStringSliceNoOrder(
		masterDB.Status.SynchronousStandbys,
		masterDB.Spec.SynchronousStandbys) {
		logger.Info().
			Any("inSyncStandbys", curMasterDB.Status.SynchronousStandbys).
			Any(logSyncStdbys, curMasterDB.Spec.SynchronousStandbys).
			Msg("waiting for new defined synchronous standbys to be in sync")
	} else {
		s.chooseSynchronousStandbys(ctx, newcd, curMasterDB, masterDB, goodStandbys, minSynchronousStandbys,
			maxSynchronousStandbys, merge)
	}
	return nil
}

// syncStandbyLimits returns the minimum and maximum number of synchronous standbys, and whether to merge the previous
// synchronous standbys on a change: PostgreSQL <= 9.5 (of the keeper of the master) supports only one
func syncStandbyLimits(clusterSpec *cluster.Spec, masterDBKeeper *cluster.Keeper) (minSynchronousStandbys,
	maxSynchronousStandbys int, merge bool, err error) {
	minSynchronousStandbys = int(*clusterSpec.MinSynchronousStandbys)
	maxSynchronousStandbys = int(*clusterSpec.MaxSynchronousStandbys)
	merge = true
	// PostgresSQL <= 9.5 only supports one sync standby at a
	// time (defining multiple sync standbys is like doing "1
	// (standby1, standby2)" on postgres >= 9.6 and so we won't
	// be able to know which is the real in sync standby.
	//
	// So we always have to define 1 standby in
	// masterDB.Spec.SynchronousStandbys with the downside that
	// can there be a time window where we cannot elect the
	// synchronous standby as a new primary if it's not yet in
	// sync
	if masterDBKeeper.Status.PostgresBinaryVersion.Maj != 0 {
		pgVersionString := fmt.Sprintf("%d.%d",
			masterDBKeeper.Status.PostgresBinaryVersion.Maj,
			masterDBKeeper.Status.PostgresBinaryVersion.Min,
		)
		pgVersion, err := semver.NewVersion(pgVersionString)
		if err != nil {
			return 0, 0, false, fmt.Errorf("unable to convert to semver: %s", pgVersionString)
		}

		if pgVersion.LessThanEqual(postgresql.V95) {
			minSynchronousStandbys = 1
			maxSynchronousStandbys = 1
			merge = false
		}
	}
	return minSynchronousStandbys, maxSynchronousStandbys, merge, nil
}

// refreshInSyncStandbys updates the in sync synchronous standbys in the status of masterDB when they differ from the
// required ones, and requires only the in sync ones when one of the others was removed or isn't good
func (s *Sentinel) refreshInSyncStandbys(ctx context.Context, newcd *cluster.Data, masterDB *cluster.DB,
	goodStandbys map[string]*cluster.DB) {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	// if the current known in sync syncstandbys are different than the required ones,
	// wait for them and remove non good ones
	if !util.CompareStringSliceNoOrder(
		masterDB.Status.SynchronousStandbys,
		masterDB.Spec.SynchronousStandbys) {
		// remove old syncstandbys from current status
		masterDB.Status.SynchronousStandbys = util.CommonElements(
			masterDB.Status.SynchronousStandbys,
			masterDB.Spec.SynchronousStandbys)

		// add reported in sync syncstandbys to the current status
		curSyncStandbys := util.CommonElements(
			masterDB.Status.CurSynchronousStandbys,
			masterDB.Spec.SynchronousStandbys)
		toAddSyncStandbys := util.Difference(curSyncStandbys, masterDB.Status.SynchronousStandbys)
		masterDB.Status.SynchronousStandbys = append(
			masterDB.Status.SynchronousStandbys,
			toAddSyncStandbys...)

		// if some of the non yet in sync syncstandbys are failed,
		// set Spec.SynchronousStandbys to the current in sync ones, so other could be added.
		notInSyncSyncStandbys := util.Difference(
			masterDB.Spec.SynchronousStandbys,
			masterDB.Status.SynchronousStandbys)
		update := false
		for _, dbUID := range notInSyncSyncStandbys {
			if _, ok := newcd.DBs[dbUID]; !ok {
				logger.Info().
					Str(logDB, dbUID).
					Any("inSyncStandbys", masterDB.Status.SynchronousStandbys).
					Any(logSyncStdbys, masterDB.Spec.SynchronousStandbys).
					Msg("one of the new synchronousStandbys has been removed")
				update = true
				continue
			}
			if _, ok := goodStandbys[dbUID]; !ok {
				logger.Info().
					Str(logDB, dbUID).
					Any("inSyncStandbys", masterDB.Status.SynchronousStandbys).
					Any(logSyncStdbys, masterDB.Spec.SynchronousStandbys).
					Msg("one of the new synchronousStandbys is not in good state")
				update = true
				continue
			}
		}
		if update {
			// Use the current known in sync syncStandbys as Spec.SynchronousStandbys
			logger.Info().
				Any("inSyncStandbys", masterDB.Status.SynchronousStandbys).
				Any(logSyncStdbys, masterDB.Spec.SynchronousStandbys).
				Msg("setting the expected sync-standbys to the current known in sync sync-standbys")
			masterDB.Spec.SynchronousStandbys = masterDB.Status.SynchronousStandbys

			// Just sort to always have them in the same order and avoid
			// unneeded updates to synchronous_standby_names by the keeper.
			sort.Strings(masterDB.Spec.SynchronousStandbys)
		}
	}
}

// chooseSynchronousStandbys chooses the synchronous standbys of masterDB: it keeps the good ones, adds the best
// standbys up to MaxSynchronousStandbys and previous ones up to MinSynchronousStandbys, merges the previous ones on a
// change (with merge), and adds a fake standby when there are fewer than MinSynchronousStandbys
func (s *Sentinel) chooseSynchronousStandbys(ctx context.Context, newcd *cluster.Data,
	curMasterDB, masterDB *cluster.DB, goodStandbys map[string]*cluster.DB,
	minSynchronousStandbys, maxSynchronousStandbys int, merge bool) {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	addFakeStandby := false
	externalSynchronousStandbys := map[string]struct{}{}

	// make a map of synchronous standbys starting from the current ones
	prevSynchronousStandbys := map[string]struct{}{}
	synchronousStandbys := map[string]struct{}{}

	for _, dbUID := range masterDB.Spec.SynchronousStandbys {
		prevSynchronousStandbys[dbUID] = struct{}{}
		synchronousStandbys[dbUID] = struct{}{}
	}

	s.pruneSynchronousStandbys(ctx, newcd, masterDB, goodStandbys, synchronousStandbys, maxSynchronousStandbys)
	s.addSynchronousStandbys(ctx, newcd, curMasterDB, masterDB, synchronousStandbys, prevSynchronousStandbys,
		minSynchronousStandbys, maxSynchronousStandbys)

	if merge {
		s.mergePrevSynchronousStandbys(ctx, newcd, masterDB, synchronousStandbys, prevSynchronousStandbys)
	}

	if !reflect.DeepEqual(synchronousStandbys, prevSynchronousStandbys) {
		logger.Info().
			Str(logMasterDB, masterDB.UID).
			Any(logPrevSyncStdbys, prevSynchronousStandbys).
			Any(logSyncStdbys, synchronousStandbys).
			Msg("synchronousStandbys changed")
	} else {
		logger.Debug().
			Str(logMasterDB, masterDB.UID).
			Any(logPrevSyncStdbys, prevSynchronousStandbys).
			Any(logSyncStdbys, synchronousStandbys).
			Msg("synchronousStandbys not changed")
	}

	// If there're not enough real synchronous standbys add a fake synchronous standby
	// because we have to be strict and make the master block transactions
	// until MinSynchronousStandbys real standbys are available
	if len(synchronousStandbys)+len(externalSynchronousStandbys) < minSynchronousStandbys {
		logger.Warn().
			Str(logMasterDB, masterDB.UID).
			Int("required", minSynchronousStandbys).
			Msg("using a fake synchronous standby since there are not enough real standbys " +
				"available")
		addFakeStandby = true
	}

	masterDB.Spec.SynchronousReplication = true
	masterDB.Spec.SynchronousStandbys = []string{}
	masterDB.Spec.ExternalSynchronousStandbys = []string{}
	for dbUID := range synchronousStandbys {
		masterDB.Spec.SynchronousStandbys = append(
			masterDB.Spec.SynchronousStandbys,
			dbUID)
	}

	for dbUID := range externalSynchronousStandbys {
		masterDB.Spec.ExternalSynchronousStandbys = append(
			masterDB.Spec.ExternalSynchronousStandbys,
			dbUID)
	}

	if addFakeStandby {
		masterDB.Spec.ExternalSynchronousStandbys = append(
			masterDB.Spec.ExternalSynchronousStandbys,
			fakeStandbyName)
	}

	// remove old syncstandbys from current status
	masterDB.Status.SynchronousStandbys = util.CommonElements(
		masterDB.Status.SynchronousStandbys,
		masterDB.Spec.SynchronousStandbys)

	// Just sort to always have them in the same order and avoid
	// unneeded updates to synchronous_standby_names by the keeper.
	sort.Strings(masterDB.Spec.SynchronousStandbys)
	sort.Strings(masterDB.Spec.ExternalSynchronousStandbys)
}

// pruneSynchronousStandbys removes the synchronous standbys that no longer exist or aren't good, and those in excess of
// maxSynchronousStandbys
func (s *Sentinel) pruneSynchronousStandbys(ctx context.Context, newcd *cluster.Data, masterDB *cluster.DB,
	goodStandbys map[string]*cluster.DB, synchronousStandbys map[string]struct{}, maxSynchronousStandbys int) {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	// Remove not existing dbs (removed above)
	toRemove := map[string]struct{}{}
	for dbUID := range synchronousStandbys {
		if _, ok := newcd.DBs[dbUID]; !ok {
			logger.Info().
				Str(logMasterDB, masterDB.UID).
				Str(logDB, dbUID).
				Msg("removing non existent db from synchronousStandbys")
			toRemove[dbUID] = struct{}{}
		}
	}
	for dbUID := range toRemove {
		delete(synchronousStandbys, dbUID)
	}

	// Check if the current synchronous standbys are healthy or remove them
	toRemove = map[string]struct{}{}
	for dbUID := range synchronousStandbys {
		if _, ok := goodStandbys[dbUID]; !ok {
			logger.Warn().
				Str(logMasterDB, masterDB.UID).
				Str(logDB, dbUID).
				Msg("removing failed synchronous standby")
			toRemove[dbUID] = struct{}{}
		}
	}
	for dbUID := range toRemove {
		delete(synchronousStandbys, dbUID)
	}

	// Remove synchronous standbys in excess: keep the best ones
	if len(synchronousStandbys) > maxSynchronousStandbys {
		ranked := rankSynchronousStandbys(masterDB, goodStandbys, synchronousStandbys)
		for _, dbUID := range ranked[maxSynchronousStandbys:] {
			logger.Info().
				Str(logMasterDB, masterDB.UID).
				Str(logDB, dbUID).
				Msg("removing synchronous standby in excess")
			delete(synchronousStandbys, dbUID)
		}
	}
}

// rankSynchronousStandbys returns the uids of the synchronous standbys, the ones to keep first: those the master
// reports in sync, then the furthest along in the WAL, then by uid, so the order doesn't change from one check to the
// next. All synchronous standbys must be in goodStandbys.
func rankSynchronousStandbys(masterDB *cluster.DB, goodStandbys map[string]*cluster.DB,
	synchronousStandbys map[string]struct{}) []string {
	inSync := map[string]bool{}
	for _, dbUID := range masterDB.Status.SynchronousStandbys {
		inSync[dbUID] = true
	}
	ranked := make([]string, 0, len(synchronousStandbys))
	for dbUID := range synchronousStandbys {
		ranked = append(ranked, dbUID)
	}
	sort.Slice(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if inSync[a] != inSync[b] {
			return inSync[a]
		}
		if pa, pb := goodStandbys[a].Status.XLogPos, goodStandbys[b].Status.XLogPos; pa != pb {
			return pa > pb
		}
		return a < b
	})
	return ranked
}

// addSynchronousStandbys adds the best standbys up to maxSynchronousStandbys, and previous synchronous standbys up to
// minSynchronousStandbys
func (s *Sentinel) addSynchronousStandbys(ctx context.Context, newcd *cluster.Data, curMasterDB, masterDB *cluster.DB,
	synchronousStandbys, prevSynchronousStandbys map[string]struct{},
	minSynchronousStandbys, maxSynchronousStandbys int) {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	// try to add missing standbys up to MaxSynchronousStandbys
	bestStandbys := s.findBestStandbys(ctx, newcd, curMasterDB)

	ac := maxSynchronousStandbys - len(synchronousStandbys)
	addedCount := 0
	for _, bestStandby := range bestStandbys {
		if addedCount >= ac {
			break
		}
		if _, ok := synchronousStandbys[bestStandby.UID]; ok {
			continue
		}

		// ignore standbys that cannot be synchronous standbys
		if db, ok := newcd.DBs[bestStandby.UID]; ok {
			if keeper, ok := newcd.Keepers[db.Spec.KeeperUID]; ok &&
				(keeper.Status.CanBeSynchronousReplica != nil &&
					!*keeper.Status.CanBeSynchronousReplica) {
				logger.Debug().
					Str(logDB, db.UID).
					Str(logKeeper, keeper.UID).
					Msg("cannot choose standby as synchronous (--can-be-synchronous-replica=false)")
				continue
			}
		}

		logger.Info().
			Str(logMasterDB, masterDB.UID).
			Str(logSyncStdbyDb, bestStandby.UID).
			Str(logKeeper, bestStandby.Spec.KeeperUID).
			Msg("adding new synchronous standby in good state trying to reach " +
				"MaxSynchronousStandbys")
		synchronousStandbys[bestStandby.UID] = struct{}{}
		addedCount++
	}

	// If there're some missing standbys to reach
	// MinSynchronousStandbys, keep previous sync standbys,
	// also if not in a good state. In this way we have more
	// possibilities to choose a sync standby to replace a
	// failed master if they becoe healthy again
	ac = minSynchronousStandbys - len(synchronousStandbys)
	addedCount = 0
	for _, db := range newcd.DBs {
		if addedCount >= ac {
			break
		}
		if _, ok := synchronousStandbys[db.UID]; ok {
			continue
		}
		if _, ok := prevSynchronousStandbys[db.UID]; ok {
			logger.Info().
				Str(logMasterDB, masterDB.UID).
				Str(logSyncStdbyDb, db.UID).
				Str(logKeeper, db.Spec.KeeperUID).
				Msg("adding previous synchronous standby to reach MinSynchronousStandbys")
			synchronousStandbys[db.UID] = struct{}{}
			addedCount++
		}
	}
}

// mergePrevSynchronousStandbys adds the previous synchronous standbys when some of the new ones weren't among them
func (s *Sentinel) mergePrevSynchronousStandbys(ctx context.Context, newcd *cluster.Data, masterDB *cluster.DB,
	synchronousStandbys, prevSynchronousStandbys map[string]struct{}) {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	// if some of the new synchronousStandbys are not inside
	// the prevSynchronousStandbys then also add all
	// the prevSynchronousStandbys. In this way when there's
	// a synchronousStandbys change we'll have, in a first
	// step, both the old and the new standbys, then in the
	// second step the old will be removed (since the new
	// standbys are all inside prevSynchronousStandbys), so
	// we'll always be able to choose a sync standby that we
	// know was defined in the primary and in sync if the
	// primary fails.
	allInPrev := true
	for k := range synchronousStandbys {
		if _, ok := prevSynchronousStandbys[k]; !ok {
			allInPrev = false
		}
	}
	if !allInPrev {
		logger.Info().
			Str(logMasterDB, masterDB.UID).
			Any(logPrevSyncStdbys, prevSynchronousStandbys).
			Any(logSyncStdbys, synchronousStandbys).
			Msg("merging current and previous synchronous standbys")
		// use only existing dbs
		for _, db := range newcd.DBs {
			if _, ok := prevSynchronousStandbys[db.UID]; ok {
				logger.Info().
					Str(logMasterDB, masterDB.UID).
					Str(logSyncStdbyDb, db.UID).
					Str(logKeeper, db.Spec.KeeperUID).
					Msg("adding previous synchronous standby")
				synchronousStandbys[db.UID] = struct{}{}
			}
		}
	}
}

// updateStandbysCount keeps the number of standbys at MaxStandbysPerSender: with enough good standbys it removes the
// others and those in excess (not the synchronous standbys), else it adds standbys on free keepers
func (s *Sentinel) updateStandbysCount(ctx context.Context, newcd *cluster.Data, clusterSpec *cluster.Spec,
	masterDB *cluster.DB, goodStandbys map[string]*cluster.DB, goodStandbysCount, convergingStandbysCount int,
	wantedMasterDBUID string) {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	// NotFailed != Good since there can be some dbs that are converging
	// it's the total number of standbys - the failed standbys
	// or the sum of good + converging standbys
	notFailedStandbysCount := goodStandbysCount + convergingStandbysCount
	// Remove dbs in excess if we have a good number >= MaxStandbysPerSender
	// We don't remove failed db until the number of good db is >= MaxStandbysPerSender
	// since they can come back
	if uint16(goodStandbysCount) >= *clusterSpec.MaxStandbysPerSender {
		toRemove := []*cluster.DB{}
		// Remove all non good standbys
		for _, db := range newcd.DBs {
			if s.dbType(newcd, db.UID) != dbTypeStandby {
				continue
			}
			// Don't remove standbys marked as synchronous standbys
			if util.StringInSlice(masterDB.Spec.SynchronousStandbys, db.UID) {
				continue
			}
			if _, ok := goodStandbys[db.UID]; !ok {
				logger.Warn().Str(logDB, db.UID).Msg("removing non good standby")
				toRemove = append(toRemove, db)
			}
		}
		// Remove good standbys in excess
		nr := int(uint16(goodStandbysCount) - *clusterSpec.MaxStandbysPerSender)
		i := 0
		for _, db := range goodStandbys {
			if i >= nr {
				break
			}
			// Don't remove standbys marked as synchronous standbys
			if util.StringInSlice(masterDB.Spec.SynchronousStandbys, db.UID) {
				continue
			}
			logger.Info().Str(logDB, db.UID).Msg("removing good standby in excess")
			toRemove = append(toRemove, db)
			i++
		}
		for _, db := range toRemove {
			delete(newcd.DBs, db.UID)
		}
	} else {
		// Add new dbs to substitute failed dbs, if there're available keepers.

		// nc can be negative if MaxStandbysPerSender has been lowered
		nc := int(*clusterSpec.MaxStandbysPerSender - uint16(notFailedStandbysCount))
		// Add missing DBs until MaxStandbysPerSender
		freeKeepers := s.freeKeepers(newcd)
		nf := len(freeKeepers)
		for i := 0; i < nc && i < nf; i++ {
			freeKeeper := freeKeepers[i]
			db := &cluster.DB{
				UID:        s.UIDFn(),
				Generation: cluster.InitialGeneration,
				Spec: &cluster.DBSpec{
					KeeperUID: freeKeeper.UID,
					InitMode:  cluster.ResyncDB,
					Role:      common.RoleReplica,
					Followers: []string{},
					FollowConfig: &cluster.FollowConfig{
						Type:  cluster.FollowTypeInternal,
						DBUID: wantedMasterDBUID},
				},
			}
			newcd.DBs[db.UID] = db
			logger.Info().
				Str(logDB, db.UID).
				Str(logKeeper, db.Spec.KeeperUID).
				Msg("added new standby db")
		}
	}
}

// reconfigureStandbys makes all standbys follow wantedMasterDBUID
func (s *Sentinel) reconfigureStandbys(newcd *cluster.Data, wantedMasterDBUID string) {
	// Reconfigure all standbys as followers of the current master
	for _, db := range newcd.DBs {
		if s.dbType(newcd, db.UID) != dbTypeStandby {
			continue
		}

		db.Spec.Role = common.RoleReplica
		// Remove followers
		db.Spec.Followers = []string{}
		db.Spec.FollowConfig = &cluster.FollowConfig{
			Type:  cluster.FollowTypeInternal,
			DBUID: wantedMasterDBUID}

		db.Spec.SynchronousReplication = false
		db.Spec.SynchronousStandbys = nil
		db.Spec.ExternalSynchronousStandbys = nil
	}
}

// updateMasterFollowers sets the followers of the master db (curMasterDBUID): the dbs following wantedMasterDBUID
func (s *Sentinel) updateMasterFollowers(newcd *cluster.Data, curMasterDBUID, wantedMasterDBUID string) {
	// Update followers for master DB
	// Always do this since, in future, keepers and related db could be
	// removed (currently only dead keepers without an assigned db are
	// removed)
	masterDB := newcd.DBs[curMasterDBUID]
	masterDB.Spec.Followers = []string{}
	for _, db := range newcd.DBs {
		if masterDB.UID == db.UID {
			continue
		}
		fc := db.Spec.FollowConfig
		if fc != nil {
			if fc.Type == cluster.FollowTypeInternal && fc.DBUID == wantedMasterDBUID {
				masterDB.Spec.Followers = append(masterDB.Spec.Followers, db.UID)
			}
		}
	}
	// Sort followers so the slice won't be considered changed due to different order of the same entries.
	sort.Strings(masterDB.Spec.Followers)
}

// mergedPGParameters returns the parameters an initialized db reports (from its configuration, without the ones the
// keeper manages) with the ones of the spec on top, as the keeper applies them: a parameter of the spec that the
// keeper manages (e.g. wal_keep_size) isn't in the reported ones, and would otherwise be lost
func mergedPGParameters(spec, reported cluster.PGParameters) cluster.PGParameters {
	merged := cluster.PGParameters{}
	for k, v := range reported {
		merged[k] = v
	}
	for k, v := range spec {
		merged[k] = v
	}
	return merged
}

// setDBSpecFromClusterSpec updates dbSpec values with the related clusterSpec ones
func (s *Sentinel) setDBSpecFromClusterSpec(cd *cluster.Data) {
	clusterSpec := cd.Cluster.DefSpec()
	for _, db := range cd.DBs {
		db.Spec.RequestTimeout = *clusterSpec.RequestTimeout
		db.Spec.MaxStandbys = *clusterSpec.MaxStandbys
		db.Spec.UsePgrewind = *clusterSpec.UsePgrewind
		db.Spec.ResyncFastCheckpoint = *clusterSpec.ResyncFastCheckpoint
		db.Spec.PGParameters = clusterSpec.PGParameters
		db.Spec.PGHBA = clusterSpec.PGHBA
		if db.Spec.FollowConfig != nil && db.Spec.FollowConfig.Type == cluster.FollowTypeExternal {
			db.Spec.FollowConfig.StandbySettings = clusterSpec.StandbyConfig.StandbySettings
			db.Spec.FollowConfig.ArchiveRecoverySettings = clusterSpec.StandbyConfig.ArchiveRecoverySettings
		}
		db.Spec.AdditionalWalSenders = *clusterSpec.AdditionalWalSenders
		switch s.dbType(cd, db.UID) {
		case dbTypeMaster:
			db.Spec.AdditionalReplicationSlots = clusterSpec.AdditionalMasterReplicationSlots
		case dbTypeStandby:
			db.Spec.AdditionalReplicationSlots = nil
			// TODO(sgotti). Update when there'll be an option to define
			// additional replication slots on standbys
		}
	}
}
