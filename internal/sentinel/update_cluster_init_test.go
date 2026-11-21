// Copyright 2026 PgVillage
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
	"strings"
	"testing"
	"time"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/timer"
	"github.com/pgvillage-tools/orion/internal/util"
)

// The Initializing phase of updateCluster: choosing the keeper of the first db (new, existing and PITR clusters),
// and waiting for that db to converge before the cluster goes to the Normal phase.

// initCD returns an Initializing cluster with the init mode of spec and keepers k1 and k2. With master set, db1 on k1
// is the db chosen to initialize the cluster, at generation 2 and converged to currentGeneration.
func initCD(spec *cluster.Spec, master bool, currentGeneration int64) *cluster.Data {
	cd := etClusterData(spec, "", nil, "keeper1", "keeper2")
	cd.Cluster.Status.Phase = cluster.Initializing
	if master {
		db := etMaster("db1", "keeper1", 0)
		db.Generation = 2
		db.Status.CurrentGeneration = currentGeneration
		db.Status.PGParameters = cluster.PGParameters{"max_connections": "150"}
		cd.DBs[db.UID] = db
		cd.Cluster.Status.Master = db.UID
	}
	return cd
}

// initSpec returns a spec with init mode mode
func initSpec(mode cluster.InitMode) *cluster.Spec {
	spec := etAsyncSpec()
	spec.InitMode = util.ToPtr(mode)
	switch mode {
	case cluster.ExistingCluster:
		spec.ExistingConfig = &cluster.ExistingConfig{KeeperUID: "keeper2"}
	case cluster.PITR:
		spec.PITRConfig = &cluster.PITRConfig{DataRestoreCommand: "restore"}
	}
	return spec
}

// initUpdate runs updateCluster on cd; converging sets the convergence timer of db1 to now (so it is converging, not
// failed)
func initUpdate(t *testing.T, cd *cluster.Data, converging bool) (*cluster.Data, error) {
	t.Helper()
	s := etSentinel(cd)
	if converging {
		s.dbConvergenceInfos["db1"].Timer = timer.Now()
	}
	return s.updateCluster(context.Background(), cd, cluster.ProxiesInfo{})
}

func mustInitUpdate(t *testing.T, cd *cluster.Data, converging bool) *cluster.Data {
	t.Helper()
	outcd, err := initUpdate(t, cd, converging)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return outcd
}

// checkInitialized checks that the cluster went to the Normal phase with db1 as its master, which no longer has an
// init mode or includes its previous config, and the cluster parameters of db1 when merged
func checkInitialized(t *testing.T, outcd *cluster.Data, merged bool) {
	t.Helper()
	if outcd.Cluster.Status.Phase != cluster.Normal {
		t.Fatalf("phase = %s, want %s", outcd.Cluster.Status.Phase, cluster.Normal)
	}
	db := outcd.DBs["db1"]
	if db.Spec.InitMode != cluster.NoDB || db.Spec.IncludeConfig {
		t.Errorf("db1 init mode %q, include config %v; want none and false", db.Spec.InitMode, db.Spec.IncludeConfig)
	}
	if got := outcd.Cluster.Spec.PGParameters["max_connections"]; (got == "150") != merged {
		t.Errorf("cluster max_connections = %q, merged %v", got, merged)
	}
}

// checkStillInitializing checks that the cluster is still Initializing with db1 as the db to initialize it
func checkStillInitializing(t *testing.T, outcd *cluster.Data) {
	t.Helper()
	if outcd.Cluster.Status.Phase != cluster.Initializing || outcd.Cluster.Status.Master != "db1" {
		t.Fatalf("phase %s, master %q; want %s with db1", outcd.Cluster.Status.Phase, outcd.Cluster.Status.Master,
			cluster.Initializing)
	}
	if _, ok := outcd.DBs["db1"]; !ok {
		t.Error("db1 removed")
	}
}

// checkNewMasterDB checks that the cluster has one db, its master, on keeper with init mode and role, and returns it
func checkNewMasterDB(t *testing.T, outcd *cluster.Data, keeper string, mode cluster.DBInitMode,
	role common.Role) *cluster.DB {
	t.Helper()
	if len(outcd.DBs) != 1 {
		t.Fatalf("dbs = %v, want one", etUIDs(dbList(outcd)))
	}
	db := outcd.DBs[outcd.Cluster.Status.Master]
	if db == nil {
		t.Fatalf("master %q has no db", outcd.Cluster.Status.Master)
	}
	if db.Spec.KeeperUID != keeper || db.Spec.InitMode != mode || db.Spec.Role != role {
		t.Errorf("db on keeper %s, init mode %s, role %s; want %s, %s, %s", db.Spec.KeeperUID, db.Spec.InitMode,
			db.Spec.Role, keeper, mode, role)
	}
	if db.Generation != cluster.InitialGeneration || !db.Spec.IncludeConfig {
		t.Errorf("generation %d, include config %v; want %d and the default (merge) true", db.Generation,
			db.Spec.IncludeConfig, cluster.InitialGeneration)
	}
	return db
}

func dbList(cd *cluster.Data) []*cluster.DB {
	var dbs []*cluster.DB
	for _, db := range cd.DBs {
		dbs = append(dbs, db)
	}
	return dbs
}

func TestUpdateClusterInitNew(t *testing.T) {
	t.Run("chooses a keeper for the first db", func(t *testing.T) {
		spec := initSpec(cluster.New)
		spec.NewConfig = &cluster.NewConfig{Locale: "C"}
		outcd := mustInitUpdate(t, initCD(spec, false, 0), false)
		db := checkNewMasterDB(t, outcd, "keeper1", cluster.NewDB, common.RolePrimary)
		if db.Spec.NewConfig == nil || db.Spec.NewConfig.Locale != "C" {
			t.Errorf("new config = %+v", db.Spec.NewConfig)
		}
	})
	t.Run("needs a keeper", func(t *testing.T) {
		cd := initCD(initSpec(cluster.New), false, 0)
		cd.Keepers = cluster.Keepers{}
		if _, err := initUpdate(t, cd, false); err == nil ||
			!strings.Contains(err.Error(), "cannot choose initial master: no keepers registered") {
			t.Errorf("expected no keepers error, got %v", err)
		}
	})
	t.Run("initialized when the db converged healthy", func(t *testing.T) {
		checkInitialized(t, mustInitUpdate(t, initCD(initSpec(cluster.New), true, 2), false), true)
	})
	t.Run("without merging the parameters of initdb", func(t *testing.T) {
		spec := initSpec(cluster.New)
		spec.MergePgParameters = util.ToPtr(false)
		checkInitialized(t, mustInitUpdate(t, initCD(spec, true, 2), false), false)
	})
	t.Run("waits for a converged db to be healthy", func(t *testing.T) {
		cd := initCD(initSpec(cluster.New), true, 2)
		cd.DBs["db1"].Status.Healthy = false
		checkStillInitializing(t, mustInitUpdate(t, cd, false))
	})
	t.Run("waits for a converging db", func(t *testing.T) {
		checkStillInitializing(t, mustInitUpdate(t, initCD(initSpec(cluster.New), true, 1), true))
	})
	t.Run("starts over when the db failed to initialize", func(t *testing.T) {
		outcd := mustInitUpdate(t, initCD(initSpec(cluster.New), true, 1), false)
		if len(outcd.DBs) != 0 || outcd.Cluster.Status.Master != "" {
			t.Errorf("dbs %v, master %q; want none, so another keeper can be chosen", etUIDs(dbList(outcd)),
				outcd.Cluster.Status.Master)
		}
	})
}

func TestUpdateClusterInitExisting(t *testing.T) {
	t.Run("uses the keeper of the existing config", func(t *testing.T) {
		outcd := mustInitUpdate(t, initCD(initSpec(cluster.ExistingCluster), false, 0), false)
		checkNewMasterDB(t, outcd, "keeper2", cluster.ExistingDB, common.RolePrimary)
	})
	t.Run("needs the keeper of the existing config", func(t *testing.T) {
		spec := initSpec(cluster.ExistingCluster)
		spec.ExistingConfig.KeeperUID = "keeper9"
		if _, err := initUpdate(t, initCD(spec, false, 0), false); err == nil ||
			!strings.Contains(err.Error(), `keeper "keeper9" state not available`) {
			t.Errorf("expected a missing keeper error, got %v", err)
		}
	})
	t.Run("initialized when the db converged healthy", func(t *testing.T) {
		checkInitialized(t, mustInitUpdate(t, initCD(initSpec(cluster.ExistingCluster), true, 2), false), true)
	})
	t.Run("without merging the parameters of the existing db", func(t *testing.T) {
		spec := initSpec(cluster.ExistingCluster)
		spec.MergePgParameters = util.ToPtr(false)
		checkInitialized(t, mustInitUpdate(t, initCD(spec, true, 2), false), false)
	})
	t.Run("waits for a db that hasn't converged", func(t *testing.T) {
		// an existing db never fails to initialize: it keeps waiting
		checkStillInitializing(t, mustInitUpdate(t, initCD(initSpec(cluster.ExistingCluster), true, 1), false))
	})
}

func TestUpdateClusterInitPITR(t *testing.T) {
	t.Run("chooses a keeper for the restored primary", func(t *testing.T) {
		outcd := mustInitUpdate(t, initCD(initSpec(cluster.PITR), false, 0), false)
		db := checkNewMasterDB(t, outcd, "keeper1", cluster.PITRDB, common.RolePrimary)
		if db.Spec.PITRConfig == nil || db.Spec.PITRConfig.DataRestoreCommand != "restore" || db.Spec.FollowConfig != nil {
			t.Errorf("pitr config %+v, follow config %+v", db.Spec.PITRConfig, db.Spec.FollowConfig)
		}
	})
	t.Run("a standby cluster restores a standby of the external primary", func(t *testing.T) {
		spec := initSpec(cluster.PITR)
		spec.Role = util.ToPtr(cluster.Replica)
		spec.StandbyConfig = &cluster.StandbyConfig{
			StandbySettings: &cluster.StandbySettings{PrimaryConninfo: "host=elsewhere"},
		}
		outcd := mustInitUpdate(t, initCD(spec, false, 0), false)
		db := checkNewMasterDB(t, outcd, "keeper1", cluster.PITRDB, common.RoleReplica)
		fc := db.Spec.FollowConfig
		if fc == nil || fc.Type != cluster.FollowTypeExternal || fc.StandbySettings == nil ||
			fc.StandbySettings.PrimaryConninfo != "host=elsewhere" {
			t.Errorf("follow config = %+v", fc)
		}
	})
	t.Run("needs a keeper", func(t *testing.T) {
		cd := initCD(initSpec(cluster.PITR), false, 0)
		cd.Keepers = cluster.Keepers{}
		if _, err := initUpdate(t, cd, false); err == nil ||
			!strings.Contains(err.Error(), "cannot choose initial master: no keepers registered") {
			t.Errorf("expected no keepers error, got %v", err)
		}
	})
	t.Run("initialized when the db converged healthy", func(t *testing.T) {
		checkInitialized(t, mustInitUpdate(t, initCD(initSpec(cluster.PITR), true, 2), false), true)
	})
	t.Run("without merging the parameters of the restored db", func(t *testing.T) {
		spec := initSpec(cluster.PITR)
		spec.MergePgParameters = util.ToPtr(false)
		checkInitialized(t, mustInitUpdate(t, initCD(spec, true, 2), false), false)
	})
	t.Run("waits for a converged db to be healthy", func(t *testing.T) {
		cd := initCD(initSpec(cluster.PITR), true, 2)
		cd.DBs["db1"].Status.Healthy = false
		checkStillInitializing(t, mustInitUpdate(t, cd, false))
	})
	t.Run("waits for a restore without a timeout", func(t *testing.T) {
		// a restore has no timeout (TODO(sgotti) in updateCluster), so an old convergence timer doesn't fail it
		checkStillInitializing(t, mustInitUpdate(t, initCD(initSpec(cluster.PITR), true, 1), false))
	})
}

func TestUpdateClusterInitUnknownMode(t *testing.T) {
	cd := initCD(initSpec("unknown"), false, 0)
	if _, err := initUpdate(t, cd, false); err == nil || !strings.Contains(err.Error(), "unknown init mode unknown") {
		t.Errorf("expected an unknown init mode error, got %v", err)
	}
}

func TestUpdateClusterInitMasterWithoutDB(t *testing.T) {
	for _, mode := range []cluster.InitMode{cluster.New, cluster.ExistingCluster, cluster.PITR} {
		t.Run(string(mode), func(t *testing.T) {
			cd := initCD(initSpec(mode), true, 2)
			delete(cd.DBs, "db1")
			defer func() {
				if r := recover(); r == nil || !strings.Contains(r.(error).Error(), `db "db1" object doesn't exists`) {
					t.Errorf("expected a panic for the missing db, got %v", r)
				}
			}()
			_, _ = initUpdate(t, cd, false)
		})
	}
}

// TestUpdateClusterInitCases: the Init phase, from the updateCluster table (#0-#3)
func TestUpdateClusterInitCases(t *testing.T) {
	runUpdateClusterCases(t, []updateClusterCase{
		// Init phase, also test dbSpec parameters copied from clusterSpec.
		// #0 cluster initialization, no keepers
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            util.ToPtr(cluster.DefaultMaxStandbys * 2),
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						AdditionalWalSenders:   util.ToPtr(uint16(cluster.DefaultAdditionalWalSenders) * 2),
						SynchronousReplication: util.ToPtr(true),
						UsePgrewind:            util.ToPtr(true),
						PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
						InitMode:               &newCluster,
						MergePgParameters:      util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
					},
				},
				Keepers: cluster.Keepers{},
				DBs:     cluster.DBs{},
				Proxy:   &cluster.Proxy{},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            util.ToPtr(cluster.DefaultMaxStandbys * 2),
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						AdditionalWalSenders:   util.ToPtr(uint16(cluster.DefaultAdditionalWalSenders) * 2),
						SynchronousReplication: util.ToPtr(true),
						UsePgrewind:            util.ToPtr(true),
						PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
						InitMode:               &newCluster,
						MergePgParameters:      util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
					},
				},
				Keepers: cluster.Keepers{},
				DBs:     cluster.DBs{},
				Proxy:   &cluster.Proxy{},
			},
			err: errors.New("cannot choose initial master: no keepers registered"),
		},
		// #1 cluster initialization, one keeper
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            util.ToPtr(cluster.DefaultMaxStandbys * 2),
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						AdditionalWalSenders:   util.ToPtr(uint16(cluster.DefaultAdditionalWalSenders) * 2),
						SynchronousReplication: util.ToPtr(true),
						UsePgrewind:            util.ToPtr(true),
						PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
						InitMode:               &newCluster,
						MergePgParameters:      util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs:   cluster.DBs{},
				Proxy: &cluster.Proxy{},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            util.ToPtr(cluster.DefaultMaxStandbys * 2),
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						AdditionalWalSenders:   util.ToPtr(uint16(cluster.DefaultAdditionalWalSenders) * 2),
						SynchronousReplication: util.ToPtr(true),
						UsePgrewind:            util.ToPtr(true),
						PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
						InitMode:               &newCluster,
						MergePgParameters:      util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k1,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
							MaxStandbys:            cluster.DefaultMaxStandbys * 2,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders * 2,
							SynchronousReplication: false,
							UsePgrewind:            true,
							// revive:disable-next-line
							PGParameters:                cluster.PGParameters{p1: v1, p2: v2},
							InitMode:                    cluster.NewDB,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							IncludeConfig:               true,
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
					},
				},
				Proxy: &cluster.Proxy{},
			},
		},
		// #2 cluster initialization, more than one keeper, the first will be chosen to be the new master.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            util.ToPtr(cluster.DefaultMaxStandbys * 2),
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
						UsePgrewind:            util.ToPtr(true),
						PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
						InitMode:               &newCluster,
						MergePgParameters:      util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs:   cluster.DBs{},
				Proxy: &cluster.Proxy{},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            util.ToPtr(cluster.DefaultMaxStandbys * 2),
						SynchronousReplication: util.ToPtr(true),
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						UsePgrewind:            util.ToPtr(true),
						PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
						InitMode:               &newCluster,
						MergePgParameters:      util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k1,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
							MaxStandbys:            cluster.DefaultMaxStandbys * 2,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							UsePgrewind:            true,
							PGParameters: cluster.PGParameters{
								p1: v1,
								p2: v2,
							},
							InitMode:                    cluster.NewDB,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							IncludeConfig:               true,
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
					},
				},
				Proxy: &cluster.Proxy{},
			},
		},
		// #3 cluster initialization, keeper initialization failed
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						InitMode:             &newCluster,
						MergePgParameters:    util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							InitMode:                    cluster.NewDB,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							CurrentGeneration: 0,
						},
					},
				},
				Proxy: &cluster.Proxy{},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						InitMode:             &newCluster,
						MergePgParameters:    util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs:   cluster.DBs{},
				Proxy: &cluster.Proxy{},
			},
		},
	})
}
