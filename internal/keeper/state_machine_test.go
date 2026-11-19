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

package keeper

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/logging"
	pg "github.com/pgvillage-tools/orion/internal/postgresql"
	"github.com/pgvillage-tools/orion/internal/util"
	"github.com/rs/zerolog"
)

var errFake = errors.New("fake error")

// quietCtx returns a context whose keeper logger discards everything, so the many state machine passes don't flood
// the test output (the logger keeps the keeper log level, or logging.GetLogComponent replaces it)
func quietCtx() context.Context {
	_, def := logging.GetLogComponent(context.Background(), logging.KeeperComponent)
	return zerolog.New(io.Discard).Level(def.GetLevel()).WithContext(context.Background())
}

// converged tells if a pass of the state machine completed: the db local state has the db uid and generation
func converged(p *PostgresKeeper, db *cluster.DB) bool {
	dbls := p.dbLocalStateCopy()
	return dbls.UID == db.UID && dbls.Generation == db.Generation && !dbls.Initializing
}

func TestStateMachineWaits(t *testing.T) {
	ctx := quietCtx()
	tests := []struct {
		name string
		// setup changes the cluster data and store of a converged primary
		setup func(s *fakeStore)
		// stops tells if the instance is stopped
		stops bool
	}{
		{name: "store error", setup: func(s *fakeStore) { s.err = errFake }},
		{name: "no cluster data", setup: func(s *fakeStore) { s.cd = nil }},
		{name: "other format version", setup: func(s *fakeStore) { s.cd.FormatVersion = 99 }},
		{name: "invalid spec", setup: func(s *fakeStore) { s.cd.Cluster.Spec.InitMode = nil }},
		{name: "keeper not in cluster data", setup: func(s *fakeStore) { delete(s.cd.Keepers, testKeeperUID) }},
		{
			name:  "no db assigned",
			setup: func(s *fakeStore) { delete(s.cd.DBs, "db1") },
			stops: true,
		},
		{
			name:  "other boot uuid",
			setup: func(s *fakeStore) { s.cd.Keepers[testKeeperUID].Status.BootUUID = "old" },
			stops: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, f, s := smKeeper(t)
			db := smDB("db1", common.RolePrimary, cluster.NewDB)
			s.cd = smClusterData(db)
			tt.setup(s)
			f.started = true
			p.postgresKeeperSM(ctx)
			if f.called("SetHba") || f.called("IsInitialized") {
				t.Errorf("the state machine went on: %v", f.calls)
			}
			if f.called("StopIfStarted") != tt.stops || f.started == tt.stops {
				t.Errorf("stops = %v, want %v (calls %v)", f.called("StopIfStarted"), tt.stops, f.calls)
			}
			if p.dbLocalState.UID != "" {
				t.Errorf("db local state changed: %+v", p.dbLocalState)
			}
		})
	}
}

func TestStateMachineSavesClusterUID(t *testing.T) {
	ctx := quietCtx()
	p, _, s := smKeeper(t)
	s.cd = smClusterData(nil)
	p.postgresKeeperSM(ctx)
	if p.keeperLocalState.ClusterUID != "cluster1" {
		t.Fatalf("cluster uid = %q", p.keeperLocalState.ClusterUID)
	}
	p.keeperLocalState = nil
	if err := p.loadKeeperLocalState(); err != nil || p.keeperLocalState.ClusterUID != "cluster1" {
		t.Errorf("saved keeper local state = %+v, %v", p.keeperLocalState, err)
	}

	// without a data dir the cluster uid can't be saved, and the keeper waits
	p, f, s := smKeeper(t)
	p.cfg.DataDir = filepath.Join(p.cfg.DataDir, "missing")
	s.cd = smClusterData(smDB("db1", common.RolePrimary, cluster.NewDB))
	p.postgresKeeperSM(ctx)
	if len(f.calls) != 0 {
		t.Errorf("the state machine went on: %v", f.calls)
	}
}

func TestStateMachineNewDB(t *testing.T) {
	ctx := quietCtx()
	p, f, s := smKeeper(t)
	db := smDB("db1", common.RolePrimary, cluster.NewDB)
	db.Spec.NewConfig = &cluster.NewConfig{Locale: "C", Encoding: "UTF8", DataChecksums: true}
	db.Spec.IncludeConfig = true
	db.Spec.Followers = []string{"db2"}
	db.Spec.AdditionalReplicationSlots = []string{"extra"}
	s.cd = smClusterData(db)
	f.configParams = common.Parameters{"work_mem": "8MB"}
	f.slots = []string{common.OrionName("gone"), "user_slot"}

	p.postgresKeeperSM(ctx)

	const init = "StopIfStarted,RemoveAllIfInitialized,Init,StartTmpMerged,WaitReady,SetupRoles,StopIfStarted,Start," +
		"WaitReady"
	if got := f.sequence("StopIfStarted", "RemoveAllIfInitialized", "Init", "StartTmpMerged", "WaitReady", "SetupRoles",
		"Start", "Promote"); got != init {
		t.Errorf("calls = %s, want %s", got, init)
	}
	if !converged(p, db) {
		t.Errorf("not converged: %+v", p.dbLocalState)
	}
	if p.dbLocalState.InitPGParameters["work_mem"] != "8MB" {
		t.Errorf("init parameters = %v", p.dbLocalState.InitPGParameters)
	}
	if !f.started {
		t.Error("instance not started")
	}
	want := []string{"user_slot", common.OrionName("db2"), common.OrionName("extra")}
	if !util.CompareStringSliceNoOrder(f.slots, want) {
		t.Errorf("slots = %v, want %v", f.slots, want)
	}
	if f.called("Reload") || f.called("Restart") {
		t.Errorf("reloaded or restarted after the first pass: %v", f.calls)
	}

	// a second pass changes nothing
	f.calls = nil
	p.postgresKeeperSM(ctx)
	if f.called("Init") || f.called("Start") || f.called("Reload") || !converged(p, db) {
		t.Errorf("second pass: %v", f.calls)
	}
}

func TestStateMachineNewDBErrors(t *testing.T) {
	ctx := quietCtx()
	for _, method := range []string{"StopIfStarted", "RemoveAllIfInitialized", "Init", "StartTmpMerged", "WaitReady",
		"GetConfigFilePGParameters", "SetupRoles"} {
		t.Run(method, func(t *testing.T) {
			p, f, s := smKeeper(t)
			db := smDB("db1", common.RolePrimary, cluster.NewDB)
			db.Spec.IncludeConfig = true
			s.cd = smClusterData(db)
			f.errs[method] = errFake
			p.postgresKeeperSM(ctx)
			if !p.dbLocalState.Initializing || p.dbLocalState.UID != db.UID {
				t.Errorf("db local state = %+v, want initializing %s", p.dbLocalState, db.UID)
			}
			if f.called("Start") {
				t.Errorf("started after a failed init: %v", f.calls)
			}
		})
	}

	// the db local state can't be saved
	p, f, s := smKeeper(t)
	p.cfg.DataDir = filepath.Join(p.cfg.DataDir, "missing")
	p.keeperLocalState.ClusterUID = "cluster1"
	s.cd = smClusterData(smDB("db1", common.RolePrimary, cluster.NewDB))
	p.postgresKeeperSM(ctx)
	if f.called("Init") {
		t.Errorf("initialized without a db local state: %v", f.calls)
	}
}

// a pass that stopped while initializing (or resyncing) leaves Initializing set: the next pass removes the data and
// starts over
func TestStateMachineFailedInitialization(t *testing.T) {
	ctx := quietCtx()
	p, f, s := smKeeper(t)
	db := smDB("db1", common.RolePrimary, cluster.NewDB)
	s.cd = smClusterData(db)
	p.dbLocalState = &DBLocalState{UID: db.UID, Initializing: true}
	f.initialized = true
	p.postgresKeeperSM(ctx)
	if got := f.sequence("StopIfStarted", "RemoveAllIfInitialized", "Init"); got !=
		"StopIfStarted,RemoveAllIfInitialized,StopIfStarted,RemoveAllIfInitialized,Init,StopIfStarted" {
		t.Errorf("calls = %s", got)
	}
	if !converged(p, db) {
		t.Errorf("not converged: %+v", p.dbLocalState)
	}

	for _, method := range []string{"StopIfStarted", "RemoveAllIfInitialized"} {
		t.Run(method, func(t *testing.T) {
			p, f, s := smKeeper(t)
			s.cd = smClusterData(db)
			p.dbLocalState = &DBLocalState{UID: db.UID, Initializing: true}
			f.errs[method] = errFake
			p.postgresKeeperSM(ctx)
			if f.called("Init") || !p.dbLocalState.Initializing {
				t.Errorf("went on after a failed cleanup: %v, %+v", f.calls, p.dbLocalState)
			}
		})
	}
}

func pitrDB(external bool) *cluster.DB {
	db := smDB("db1", common.RolePrimary, cluster.PITRDB)
	db.Spec.IncludeConfig = true
	db.Spec.PITRConfig = &cluster.PITRConfig{
		DataRestoreCommand:      "restore %d",
		ArchiveRecoverySettings: &cluster.ArchiveRecoverySettings{RestoreCommand: "cp /archive/%f %p"},
	}
	if external {
		db.Spec.FollowConfig = &cluster.FollowConfig{
			Type:            cluster.FollowTypeExternal,
			StandbySettings: &cluster.StandbySettings{PrimaryConninfo: "host=elsewhere"},
		}
	}
	return db
}

func TestStateMachinePITR(t *testing.T) {
	ctx := quietCtx()
	for _, external := range []bool{false, true} {
		p, f, s := smKeeper(t)
		db := pitrDB(external)
		s.cd = smClusterData(db)
		p.postgresKeeperSM(ctx)
		want := "StopIfStarted,RemoveAllIfInitialized,Restore,StartTmpMerged,WaitRecoveryDone,WaitReady,StopIfStarted"
		if external {
			// a standby of an external primary doesn't finish its recovery
			want = "StopIfStarted,RemoveAllIfInitialized,Restore,StartTmpMerged,WaitReady,StopIfStarted"
		}
		got := f.sequence("StopIfStarted", "RemoveAllIfInitialized", "Restore", "StartTmpMerged", "WaitRecoveryDone",
			"WaitReady")
		if got[:len(want)] != want {
			t.Errorf("external %v: calls = %s, want %s...", external, got, want)
		}
		if !converged(p, db) {
			t.Errorf("external %v: not converged: %+v", external, p.dbLocalState)
		}
	}
}

func TestStateMachinePITRErrors(t *testing.T) {
	ctx := quietCtx()
	for _, method := range []string{"StopIfStarted", "RemoveAllIfInitialized", "Restore", "StartTmpMerged",
		"WaitRecoveryDone", "WaitReady", "GetConfigFilePGParameters"} {
		t.Run(method, func(t *testing.T) {
			p, f, s := smKeeper(t)
			db := pitrDB(false)
			s.cd = smClusterData(db)
			f.errs[method] = errFake
			p.postgresKeeperSM(ctx)
			if !p.dbLocalState.Initializing || f.called("Start") {
				t.Errorf("went on after a failed restore: %v, %+v", f.calls, p.dbLocalState)
			}
		})
	}
}

func TestStateMachineExistingDB(t *testing.T) {
	ctx := quietCtx()
	p, f, s := smKeeper(t)
	db := smDB("db1", common.RolePrimary, cluster.ExistingDB)
	db.Spec.IncludeConfig = true
	s.cd = smClusterData(db)
	f.initialized, f.role = true, common.RolePrimary
	f.configParams = common.Parameters{"shared_buffers": "1GB"}
	p.postgresKeeperSM(ctx)
	if got := f.sequence("StopIfStarted", "StartTmpMerged", "WaitReady", "RemoveAllIfInitialized", "Init"); got !=
		"StopIfStarted,StartTmpMerged,WaitReady,StopIfStarted,WaitReady" {
		t.Errorf("calls = %s", got)
	}
	if !converged(p, db) || p.dbLocalState.InitPGParameters["shared_buffers"] != "1GB" {
		t.Errorf("db local state = %+v", p.dbLocalState)
	}

	for _, method := range []string{"StopIfStarted", "StartTmpMerged", "WaitReady", "GetConfigFilePGParameters"} {
		t.Run(method, func(t *testing.T) {
			p, f, s := smKeeper(t)
			s.cd = smClusterData(db)
			f.initialized, f.role = true, common.RolePrimary
			f.errs[method] = errFake
			p.postgresKeeperSM(ctx)
			if converged(p, db) || f.called("Start") {
				t.Errorf("went on: %v, %+v", f.calls, p.dbLocalState)
			}
		})
	}
}

func TestStateMachineUnusableInitModes(t *testing.T) {
	ctx := quietCtx()
	for _, mode := range []cluster.DBInitMode{cluster.NoDB, "unknown"} {
		p, f, s := smKeeper(t)
		s.cd = smClusterData(smDB("db1", common.RolePrimary, mode))
		p.postgresKeeperSM(ctx)
		if p.dbLocalState.UID != "" || f.called("Start") {
			t.Errorf("%s: went on: %v, %+v", mode, f.calls, p.dbLocalState)
		}
	}
}

// resyncKeeper returns a keeper with a replica db1 of "primary" to resync; initialized tells if there is an instance
// of the same system (sys1) already
func resyncKeeper(t *testing.T, initialized bool) (*PostgresKeeper, *fakePG, *cluster.DB) {
	p, f, s := smKeeper(t)
	db := smDB("db1", common.RoleReplica, cluster.ResyncDB)
	db.Spec.UsePgrewind = true
	s.cd = smClusterData(db)
	if initialized {
		f.initialized, f.role, f.systemID = true, common.RoleReplica, "sys1"
	}
	f.systemData = &pg.SystemData{SystemID: "sys1", TimelineID: 1, XLogPos: 100}
	return p, f, db
}

func TestStateMachineResync(t *testing.T) {
	ctx := quietCtx()
	syncs := func(f *fakePG) string { return f.sequence("SyncFromFollowedPGRewind", "SyncFromFollowed") }

	t.Run("not initialized: pg_basebackup", func(t *testing.T) {
		p, f, db := resyncKeeper(t, false)
		p.postgresKeeperSM(ctx)
		if got := syncs(f); got != "SyncFromFollowed" {
			t.Errorf("syncs = %s", got)
		}
		if !converged(p, db) || !f.started || f.recovery.RecoveryMode != pg.RecoveryModeStandby {
			t.Errorf("not a converged standby: %+v, %+v", p.dbLocalState, f.recovery)
		}
		if f.called("Restart") {
			t.Errorf("restarted with the recovery options of the resync: %v", f.calls)
		}
	})
	t.Run("same system: pg_rewind", func(t *testing.T) {
		p, f, db := resyncKeeper(t, true)
		p.postgresKeeperSM(ctx)
		if got := syncs(f); got != "SyncFromFollowedPGRewind" {
			t.Errorf("syncs = %s", got)
		}
		if !converged(p, db) {
			t.Errorf("not converged: %+v", p.dbLocalState)
		}
	})
	t.Run("pg_rewind fails: pg_basebackup", func(t *testing.T) {
		p, f, db := resyncKeeper(t, true)
		f.errs["SyncFromFollowedPGRewind"] = errFake
		p.postgresKeeperSM(ctx)
		if got := syncs(f); got != "SyncFromFollowedPGRewind,SyncFromFollowed" {
			t.Errorf("syncs = %s", got)
		}
		if !converged(p, db) {
			t.Errorf("not converged: %+v", p.dbLocalState)
		}
	})
	t.Run("rewound standby not ready: pg_basebackup", func(t *testing.T) {
		p, f, db := resyncKeeper(t, true)
		f.waitReady = func() error { return errFake }
		p.postgresKeeperSM(ctx)
		if got := syncs(f); got != "SyncFromFollowedPGRewind,SyncFromFollowed" {
			t.Errorf("syncs = %s", got)
		}
		if !converged(p, db) {
			t.Errorf("not converged: %+v", p.dbLocalState)
		}
	})
	t.Run("rewound standby on another timeline: pg_basebackup", func(t *testing.T) {
		p, f, _ := resyncKeeper(t, true)
		f.systemData.TimelineID = 2
		p.postgresKeeperSM(ctx)
		if got := syncs(f); got != "SyncFromFollowedPGRewind,SyncFromFollowed" {
			t.Errorf("syncs = %s", got)
		}
	})
	t.Run("other system: pg_basebackup", func(t *testing.T) {
		p, f, _ := resyncKeeper(t, true)
		f.systemID = "sys2"
		p.postgresKeeperSM(ctx)
		if got := syncs(f); got != "SyncFromFollowed" {
			t.Errorf("syncs = %s", got)
		}
	})
	t.Run("no master: pg_basebackup", func(t *testing.T) {
		p, f, _ := resyncKeeper(t, true)
		p.e.(*fakeStore).cd.Cluster.Status.Master = ""
		p.postgresKeeperSM(ctx)
		if got := syncs(f); got != "SyncFromFollowed" {
			t.Errorf("syncs = %s", got)
		}
	})
}

func TestStateMachineResyncErrors(t *testing.T) {
	ctx := quietCtx()
	tests := []struct {
		name  string
		setup func(p *PostgresKeeper, f *fakePG)
	}{
		{name: "stop", setup: func(_ *PostgresKeeper, f *fakePG) { f.errs["StopIfStarted"] = errFake }},
		{name: "system id", setup: func(_ *PostgresKeeper, f *fakePG) { f.errs["GetSystemID"] = errFake }},
		{name: "followed db missing", setup: func(p *PostgresKeeper, _ *fakePG) {
			delete(p.e.(*fakeStore).cd.DBs, "primary")
		}},
		{name: "sync", setup: func(_ *PostgresKeeper, f *fakePG) {
			f.errs["SyncFromFollowedPGRewind"] = errFake
			f.errs["SyncFromFollowed"] = errFake
		}},
		{name: "remove data", setup: func(_ *PostgresKeeper, f *fakePG) {
			f.errs["SyncFromFollowedPGRewind"] = errFake
			f.errs["RemoveAllIfInitialized"] = errFake
		}},
		{name: "start", setup: func(_ *PostgresKeeper, f *fakePG) { f.errs["Start"] = errFake }},
		{name: "full resync after pg_rewind", setup: func(_ *PostgresKeeper, f *fakePG) {
			f.waitReady = func() error { return errFake }
			f.errs["SyncFromFollowed"] = errFake
		}},
		{name: "stop for a full resync", setup: func(_ *PostgresKeeper, f *fakePG) {
			f.waitReady = func() error {
				f.errs["StopIfStarted"] = errFake
				return errFake
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, f, db := resyncKeeper(t, true)
			tt.setup(p, f)
			p.postgresKeeperSM(ctx)
			if converged(p, db) || !p.dbLocalState.Initializing {
				t.Errorf("went on: %v, %+v", f.calls, p.dbLocalState)
			}
		})
	}
}

// primaryKeeper returns a keeper with an initialized, converged instance of db1, with the given local role, that
// should be primary
func primaryKeeper(t *testing.T, localRole common.Role) (*PostgresKeeper, *fakePG, *cluster.DB) {
	p, f, s := smKeeper(t)
	db := smDB("db1", common.RolePrimary, cluster.NewDB)
	s.cd = smClusterData(db)
	delete(s.cd.DBs, "primary")
	s.cd.Cluster.Status.Master = db.UID
	p.dbLocalState = &DBLocalState{UID: db.UID, Generation: 1}
	f.initialized, f.role = true, localRole
	return p, f, db
}

func TestStateMachinePrimary(t *testing.T) {
	ctx := quietCtx()

	t.Run("promotes a stopped standby", func(t *testing.T) {
		p, f, db := primaryKeeper(t, common.RoleReplica)
		p.postgresKeeperSM(ctx)
		if got := f.sequence("Start", "WaitReady", "Promote"); got != "Start,WaitReady,Promote" {
			t.Errorf("calls = %s", got)
		}
		if !converged(p, db) || f.role != common.RolePrimary {
			t.Errorf("not a converged primary: %+v", p.dbLocalState)
		}
	})
	t.Run("an uninitialized instance can't be primary", func(t *testing.T) {
		p, f, db := primaryKeeper(t, common.RolePrimary)
		f.initialized = false
		p.postgresKeeperSM(ctx)
		if f.called("Start") || converged(p, db) {
			t.Errorf("went on: %v", f.calls)
		}
	})
	t.Run("synchronous: no normal users until the standbys are in sync", func(t *testing.T) {
		p, f, db := primaryKeeper(t, common.RolePrimary)
		db.Spec.SynchronousReplication = true
		db.Spec.SynchronousStandbys = []string{"db2"}
		p.postgresKeeperSM(ctx)
		if !p.waitSyncStandbysSynced || !converged(p, db) {
			t.Fatalf("not waiting for the sync standbys: %v", f.calls)
		}
		internalHBA := f.curHba

		f.syncStandbys = []string{common.OrionName("db2"), "other"}
		f.calls = nil
		p.postgresKeeperSM(ctx)
		if p.waitSyncStandbysSynced || !f.called("Reload") || len(f.curHba) <= len(internalHBA) {
			t.Errorf("normal users not allowed once in sync: %v, hba %v", f.calls, f.curHba)
		}
	})
	t.Run("sync standbys can't be read", func(t *testing.T) {
		p, f, db := primaryKeeper(t, common.RolePrimary)
		db.Spec.SynchronousReplication = true
		f.errs["GetSyncStandbys"] = errFake
		p.postgresKeeperSM(ctx)
		if converged(p, db) {
			t.Errorf("converged: %v", f.calls)
		}
	})

	for _, method := range []string{"IsInitialized", "GetRole", "IsStarted", "Start", "WaitReady", "Promote",
		"GetReplicationSlots", "CreateReplicationSlot"} {
		t.Run(method+" fails", func(t *testing.T) {
			p, f, db := primaryKeeper(t, common.RoleReplica)
			db.Spec.Followers = []string{"db2"}
			f.errs[method] = errFake
			p.postgresKeeperSM(ctx)
			if converged(p, db) {
				t.Errorf("converged: %v", f.calls)
			}
		})
	}
}

// replicaKeeper returns a keeper with an initialized, converged instance of db1, with the given local role, that
// should follow db "primary"
func replicaKeeper(t *testing.T, localRole common.Role) (*PostgresKeeper, *fakePG, *cluster.DB) {
	p, f, s := smKeeper(t)
	db := smDB("db1", common.RoleReplica, cluster.ResyncDB)
	s.cd = smClusterData(db)
	p.dbLocalState = &DBLocalState{UID: db.UID, Generation: 1}
	f.initialized, f.role = true, localRole
	return p, f, db
}

func TestStateMachineReplica(t *testing.T) {
	ctx := quietCtx()

	t.Run("starts a stopped standby, and restarts it for new recovery options", func(t *testing.T) {
		p, f, db := replicaKeeper(t, common.RoleReplica)
		p.postgresKeeperSM(ctx)
		// a stopped standby starts with the recovery options of its primary: no restart
		if got := f.sequence("Start", "Restart"); got != "Start" {
			t.Errorf("calls = %s", got)
		}
		if !converged(p, db) {
			t.Errorf("not converged: %+v", p.dbLocalState)
		}
		f.calls = nil
		p.postgresKeeperSM(ctx)
		if f.called("Start") || f.called("Restart") {
			t.Errorf("second pass: %v", f.calls)
		}

		// the primary moves
		p.e.(*fakeStore).cd.DBs["primary"].Status.ListenAddress = "10.0.0.3"
		f.calls = nil
		p.postgresKeeperSM(ctx)
		if !f.called("Restart") {
			t.Errorf("not restarted for a new primary_conninfo: %v", f.calls)
		}
	})
	t.Run("external primary", func(t *testing.T) {
		p, f, db := replicaKeeper(t, common.RoleReplica)
		db.Spec.FollowConfig = &cluster.FollowConfig{
			Type:            cluster.FollowTypeExternal,
			StandbySettings: &cluster.StandbySettings{PrimaryConninfo: "host=elsewhere"},
		}
		f.started = true
		p.postgresKeeperSM(ctx)
		if !f.called("Restart") || f.curRecovery.RecoveryParameters["primary_conninfo"] != "host=elsewhere" {
			t.Errorf("not restarted for the external primary: %v, %+v", f.calls, f.recovery)
		}
		if !converged(p, db) {
			t.Errorf("not converged: %+v", p.dbLocalState)
		}
	})

	tests := []struct {
		name  string
		setup func(p *PostgresKeeper, f *fakePG, db *cluster.DB)
	}{
		{name: "unknown follow type", setup: func(_ *PostgresKeeper, _ *fakePG, db *cluster.DB) {
			db.Spec.FollowConfig.Type = "unknown"
		}},
		{name: "followed db missing", setup: func(p *PostgresKeeper, _ *fakePG, _ *cluster.DB) {
			delete(p.e.(*fakeStore).cd.DBs, "primary")
		}},
		{name: "a primary can't become a standby", setup: func(_ *PostgresKeeper, f *fakePG, _ *cluster.DB) {
			f.role = common.RolePrimary
		}},
		{name: "not initialized", setup: func(_ *PostgresKeeper, f *fakePG, _ *cluster.DB) { f.initialized = false }},
		{name: "IsStarted fails", setup: func(_ *PostgresKeeper, f *fakePG, _ *cluster.DB) {
			f.errs["IsStarted"] = errFake
		}},
		{name: "Start fails", setup: func(_ *PostgresKeeper, f *fakePG, _ *cluster.DB) { f.errs["Start"] = errFake }},
		{name: "Restart fails", setup: func(_ *PostgresKeeper, f *fakePG, _ *cluster.DB) {
			f.started = true // with other recovery options
			f.errs["Restart"] = errFake
		}},
		{name: "Restart fails for an external primary", setup: func(_ *PostgresKeeper, f *fakePG, db *cluster.DB) {
			db.Spec.FollowConfig = &cluster.FollowConfig{Type: cluster.FollowTypeExternal}
			f.started = true
			f.curRecovery.RecoveryParameters["primary_conninfo"] = "old"
			f.errs["Restart"] = errFake
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, f, db := replicaKeeper(t, common.RoleReplica)
			tt.setup(p, f, db)
			p.postgresKeeperSM(ctx)
			if converged(p, db) {
				t.Errorf("converged: %v", f.calls)
			}
		})
	}

	// the replication slots are best effort on a standby
	for _, external := range []bool{false, true} {
		p, f, db := replicaKeeper(t, common.RoleReplica)
		if external {
			db.Spec.FollowConfig = &cluster.FollowConfig{Type: cluster.FollowTypeExternal}
		}
		f.errs["GetReplicationSlots"] = errFake
		p.postgresKeeperSM(ctx)
		if !converged(p, db) {
			t.Errorf("external %v: not converged after failing replication slots: %v", external, f.calls)
		}
	}
}

func TestStateMachineNoRole(t *testing.T) {
	ctx := quietCtx()
	p, f, db := primaryKeeper(t, common.RolePrimary)
	db.Spec.Role = common.RoleUndefined
	p.postgresKeeperSM(ctx)
	if converged(p, db) || f.called("Start") {
		t.Errorf("went on: %v", f.calls)
	}
}

func TestStateMachineParameterChanges(t *testing.T) {
	ctx := quietCtx()

	// converge first, then change the spec
	setup := func(t *testing.T, autoRestart bool) (*PostgresKeeper, *fakePG, *cluster.DB) {
		p, f, db := primaryKeeper(t, common.RolePrimary)
		p.e.(*fakeStore).cd.Cluster.Spec.AutomaticPgRestart = util.ToPtr(autoRestart)
		p.postgresKeeperSM(ctx)
		if !converged(p, db) {
			t.Fatalf("not converged: %v", f.calls)
		}
		f.calls = nil
		db.Spec.PGParameters = cluster.PGParameters{"max_connections": "200"}
		db.Generation++
		return p, f, db
	}

	t.Run("reload", func(t *testing.T) {
		p, f, db := setup(t, false)
		p.postgresKeeperSM(ctx)
		if !f.called("Reload") || f.curParams["max_connections"] != "200" || !converged(p, db) {
			t.Errorf("not reloaded: %v, %v", f.calls, f.curParams)
		}
	})
	t.Run("reload fails", func(t *testing.T) {
		p, f, db := setup(t, false)
		f.errs["Reload"] = errFake
		p.postgresKeeperSM(ctx)
		if !converged(p, db) {
			t.Errorf("a failed reload stops the pass: %v", f.calls)
		}
	})
	t.Run("restart required", func(t *testing.T) {
		for _, auto := range []bool{false, true} {
			p, f, db := setup(t, auto)
			f.restartRequired = true
			p.postgresKeeperSM(ctx)
			if f.called("Restart") != auto || !converged(p, db) {
				t.Errorf("automatic restart %v: %v", auto, f.calls)
			}
		}
	})
	t.Run("restart fails, or can't tell", func(t *testing.T) {
		for _, method := range []string{"Restart", "IsRestartRequired"} {
			p, f, db := setup(t, true)
			f.restartRequired = true
			f.errs[method] = errFake
			p.postgresKeeperSM(ctx)
			if !converged(p, db) {
				t.Errorf("%s: a failure stops the pass: %v", method, f.calls)
			}
		}
	})
	t.Run("synchronous replication turned on and off", func(t *testing.T) {
		p, f, db := setup(t, false)
		db.Spec.SynchronousReplication = true
		db.Spec.SynchronousStandbys = []string{"db2"}
		f.syncStandbys = []string{common.OrionName("db2")}
		p.postgresKeeperSM(ctx)
		if f.params["synchronous_standby_names"] == "" || !converged(p, db) {
			t.Errorf("synchronous_standby_names not set: %v", f.params)
		}
		db.Spec.SynchronousReplication = false
		db.Generation++
		p.postgresKeeperSM(ctx)
		if f.params["synchronous_standby_names"] != "" || !converged(p, db) {
			t.Errorf("synchronous_standby_names not cleared: %v", f.params)
		}
	})
	t.Run("the generation can't be saved", func(t *testing.T) {
		p, f, db := setup(t, false)
		p.cfg.DataDir = filepath.Join(p.cfg.DataDir, "missing")
		p.postgresKeeperSM(ctx)
		if converged(p, db) {
			t.Errorf("converged without saving: %v", f.calls)
		}
	})
}
