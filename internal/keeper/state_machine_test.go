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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

var _ = Describe("the keeper state machine", func() {
	// converged tells if a pass of the state machine completed: the db local state has the db uid and generation
	converged := func(p *PostgresKeeper, db *cluster.DB) bool {
		dbls := p.dbLocalStateCopy()
		return dbls.UID == db.UID && dbls.Generation == db.Generation && !dbls.Initializing
	}

	pitrDB := func(external bool) *cluster.DB {
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

	// resyncKeeper returns a keeper with a replica db1 of "primary" to resync; initialized tells if there is an
	// instance of the same system (sys1) already
	resyncKeeper := func(t testing.TB, initialized bool) (*PostgresKeeper, *fakePG, *cluster.DB) {
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

	// primaryKeeper returns a keeper with an initialized, converged instance of db1, with the given local role, that
	// should be primary
	primaryKeeper := func(t testing.TB, localRole common.Role) (*PostgresKeeper, *fakePG, *cluster.DB) {
		p, f, s := smKeeper(t)
		db := smDB("db1", common.RolePrimary, cluster.NewDB)
		s.cd = smClusterData(db)
		delete(s.cd.DBs, "primary")
		s.cd.Cluster.Status.Master = db.UID
		p.dbLocalState = &DBLocalState{UID: db.UID, Generation: 1}
		f.initialized, f.role = true, localRole
		return p, f, db
	}

	// replicaKeeper returns a keeper with an initialized, converged instance of db1, with the given local role, that
	// should follow db "primary"
	replicaKeeper := func(t testing.TB, localRole common.Role) (*PostgresKeeper, *fakePG, *cluster.DB) {
		p, f, s := smKeeper(t)
		db := smDB("db1", common.RoleReplica, cluster.ResyncDB)
		s.cd = smClusterData(db)
		p.dbLocalState = &DBLocalState{UID: db.UID, Generation: 1}
		f.initialized, f.role = true, localRole
		return p, f, db
	}

	var ctx context.Context

	BeforeEach(func() {
		ctx = quietCtx()
	})

	DescribeTable("waits",
		// setup changes the cluster data and store of a converged primary; stops tells if the instance is stopped
		func(setup func(s *fakeStore), stops bool) {
			p, f, s := smKeeper(GinkgoTB())
			db := smDB("db1", common.RolePrimary, cluster.NewDB)
			s.cd = smClusterData(db)
			setup(s)
			f.started = true
			p.postgresKeeperSM(ctx)
			Expect(f.called("SetHba") || f.called("IsInitialized")).To(BeFalse(),
				"the state machine went on: %v", f.calls)
			Expect(f.called("StopIfStarted")).To(Equal(stops), "calls %v", f.calls)
			Expect(f.started).To(Equal(!stops), "calls %v", f.calls)
			Expect(p.dbLocalState.UID).To(BeEmpty(), "db local state changed")
		},
		Entry("store error", func(s *fakeStore) { s.err = errFake }, false),
		Entry("no cluster data", func(s *fakeStore) { s.cd = nil }, false),
		Entry("other format version", func(s *fakeStore) { s.cd.FormatVersion = 99 }, false),
		Entry("invalid spec", func(s *fakeStore) { s.cd.Cluster.Spec.InitMode = nil }, false),
		Entry("keeper not in cluster data", func(s *fakeStore) { delete(s.cd.Keepers, testKeeperUID) }, false),
		Entry("no db assigned", func(s *fakeStore) { delete(s.cd.DBs, "db1") }, true),
		Entry("other boot uuid", func(s *fakeStore) { s.cd.Keepers[testKeeperUID].Status.BootUUID = "old" }, true),
	)

	Describe("the cluster uid", func() {
		It("is saved in the keeper local state", func() {
			p, _, s := smKeeper(GinkgoTB())
			s.cd = smClusterData(nil)
			p.postgresKeeperSM(ctx)
			Expect(p.keeperLocalState.ClusterUID).To(Equal("cluster1"))
			p.keeperLocalState = nil
			Expect(p.loadKeeperLocalState()).To(Succeed())
			Expect(p.keeperLocalState.ClusterUID).To(Equal("cluster1"))
		})

		// without a data dir the cluster uid can't be saved, and the keeper waits
		It("makes the keeper wait when it can't be saved", func() {
			p, f, s := smKeeper(GinkgoTB())
			p.cfg.DataDir = filepath.Join(p.cfg.DataDir, "missing")
			s.cd = smClusterData(smDB("db1", common.RolePrimary, cluster.NewDB))
			p.postgresKeeperSM(ctx)
			Expect(f.calls).To(BeEmpty(), "the state machine went on")
		})
	})

	Describe("a new db", func() {
		It("is initialized, started and converged, and a second pass changes nothing", func() {
			p, f, s := smKeeper(GinkgoTB())
			db := smDB("db1", common.RolePrimary, cluster.NewDB)
			db.Spec.NewConfig = &cluster.NewConfig{Locale: "C", Encoding: "UTF8", DataChecksums: true}
			db.Spec.IncludeConfig = true
			db.Spec.Followers = []string{"db2"}
			db.Spec.AdditionalReplicationSlots = []string{"extra"}
			s.cd = smClusterData(db)
			f.configParams = common.Parameters{"work_mem": "8MB"}
			f.slots = []string{common.OrionName("gone"), "user_slot"}

			p.postgresKeeperSM(ctx)

			const init = "StopIfStarted,RemoveAllIfInitialized,Init,StartTmpMerged,WaitReady,SetupRoles," +
				"StopIfStarted,Start,WaitReady"
			Expect(f.sequence("StopIfStarted", "RemoveAllIfInitialized", "Init", "StartTmpMerged", "WaitReady",
				"SetupRoles", "Start", "Promote")).To(Equal(init))
			Expect(converged(p, db)).To(BeTrue(), "not converged: %+v", p.dbLocalState)
			Expect(p.dbLocalState.InitPGParameters).To(HaveKeyWithValue("work_mem", "8MB"))
			Expect(f.started).To(BeTrue(), "instance not started")
			Expect(f.slots).To(ConsistOf("user_slot", common.OrionName("db2"), common.OrionName("extra")))
			Expect(f.called("Reload") || f.called("Restart")).To(BeFalse(),
				"reloaded or restarted after the first pass: %v", f.calls)

			// a second pass changes nothing
			f.calls = nil
			p.postgresKeeperSM(ctx)
			Expect(f.called("Init") || f.called("Start") || f.called("Reload")).To(BeFalse(),
				"second pass: %v", f.calls)
			Expect(converged(p, db)).To(BeTrue(), "second pass: %v", f.calls)
		})

		DescribeTable("stays initializing when a step fails",
			func(method string) {
				p, f, s := smKeeper(GinkgoTB())
				db := smDB("db1", common.RolePrimary, cluster.NewDB)
				db.Spec.IncludeConfig = true
				s.cd = smClusterData(db)
				f.errs[method] = errFake
				p.postgresKeeperSM(ctx)
				Expect(p.dbLocalState.Initializing).To(BeTrue(), "db local state = %+v", p.dbLocalState)
				Expect(p.dbLocalState.UID).To(Equal(db.UID))
				Expect(f.called("Start")).To(BeFalse(), "started after a failed init: %v", f.calls)
			},
			Entry(nil, "StopIfStarted"),
			Entry(nil, "RemoveAllIfInitialized"),
			Entry(nil, "Init"),
			Entry(nil, "StartTmpMerged"),
			Entry(nil, "WaitReady"),
			Entry(nil, "GetConfigFilePGParameters"),
			Entry(nil, "SetupRoles"),
		)

		It("is not initialized when the db local state can't be saved", func() {
			p, f, s := smKeeper(GinkgoTB())
			p.cfg.DataDir = filepath.Join(p.cfg.DataDir, "missing")
			p.keeperLocalState.ClusterUID = "cluster1"
			s.cd = smClusterData(smDB("db1", common.RolePrimary, cluster.NewDB))
			p.postgresKeeperSM(ctx)
			Expect(f.called("Init")).To(BeFalse(), "initialized without a db local state: %v", f.calls)
		})
	})

	// a pass that stopped while initializing (or resyncing) leaves Initializing set: the next pass removes the data
	// and starts over
	Describe("a failed initialization", func() {
		var db *cluster.DB

		BeforeEach(func() {
			db = smDB("db1", common.RolePrimary, cluster.NewDB)
		})

		It("removes the data and starts over", func() {
			p, f, s := smKeeper(GinkgoTB())
			s.cd = smClusterData(db)
			p.dbLocalState = &DBLocalState{UID: db.UID, Initializing: true}
			f.initialized = true
			p.postgresKeeperSM(ctx)
			Expect(f.sequence("StopIfStarted", "RemoveAllIfInitialized", "Init")).To(Equal(
				"StopIfStarted,RemoveAllIfInitialized,StopIfStarted,RemoveAllIfInitialized,Init,StopIfStarted"))
			Expect(converged(p, db)).To(BeTrue(), "not converged: %+v", p.dbLocalState)
		})

		DescribeTable("doesn't go on after a failed cleanup",
			func(method string) {
				p, f, s := smKeeper(GinkgoTB())
				s.cd = smClusterData(db)
				p.dbLocalState = &DBLocalState{UID: db.UID, Initializing: true}
				f.errs[method] = errFake
				p.postgresKeeperSM(ctx)
				Expect(f.called("Init")).To(BeFalse(), "went on after a failed cleanup: %v", f.calls)
				Expect(p.dbLocalState.Initializing).To(BeTrue(), "db local state = %+v", p.dbLocalState)
			},
			Entry(nil, "StopIfStarted"),
			Entry(nil, "RemoveAllIfInitialized"),
		)
	})

	Describe("a point in time recovery", func() {
		DescribeTable("restores and converges",
			func(external bool, want string) {
				p, f, s := smKeeper(GinkgoTB())
				db := pitrDB(external)
				s.cd = smClusterData(db)
				p.postgresKeeperSM(ctx)
				got := f.sequence("StopIfStarted", "RemoveAllIfInitialized", "Restore", "StartTmpMerged",
					"WaitRecoveryDone", "WaitReady")
				Expect(got).To(HavePrefix(want))
				Expect(converged(p, db)).To(BeTrue(), "not converged: %+v", p.dbLocalState)
			},
			Entry("internal", false,
				"StopIfStarted,RemoveAllIfInitialized,Restore,StartTmpMerged,WaitRecoveryDone,WaitReady,StopIfStarted"),
			// a standby of an external primary doesn't finish its recovery
			Entry("standby of an external primary", true,
				"StopIfStarted,RemoveAllIfInitialized,Restore,StartTmpMerged,WaitReady,StopIfStarted"),
		)

		DescribeTable("doesn't go on after a failed restore",
			func(method string) {
				p, f, s := smKeeper(GinkgoTB())
				db := pitrDB(false)
				s.cd = smClusterData(db)
				f.errs[method] = errFake
				p.postgresKeeperSM(ctx)
				Expect(p.dbLocalState.Initializing).To(BeTrue(), "db local state = %+v", p.dbLocalState)
				Expect(f.called("Start")).To(BeFalse(), "went on after a failed restore: %v", f.calls)
			},
			Entry(nil, "StopIfStarted"),
			Entry(nil, "RemoveAllIfInitialized"),
			Entry(nil, "Restore"),
			Entry(nil, "StartTmpMerged"),
			Entry(nil, "WaitRecoveryDone"),
			Entry(nil, "WaitReady"),
			Entry(nil, "GetConfigFilePGParameters"),
		)
	})

	Describe("an existing db", func() {
		var db *cluster.DB

		BeforeEach(func() {
			db = smDB("db1", common.RolePrimary, cluster.ExistingDB)
			db.Spec.IncludeConfig = true
		})

		It("is taken over with its parameters", func() {
			p, f, s := smKeeper(GinkgoTB())
			s.cd = smClusterData(db)
			f.initialized, f.role = true, common.RolePrimary
			f.configParams = common.Parameters{"shared_buffers": "1GB"}
			p.postgresKeeperSM(ctx)
			Expect(f.sequence("StopIfStarted", "StartTmpMerged", "WaitReady", "RemoveAllIfInitialized", "Init")).
				To(Equal("StopIfStarted,StartTmpMerged,WaitReady,StopIfStarted,WaitReady"))
			Expect(converged(p, db)).To(BeTrue(), "db local state = %+v", p.dbLocalState)
			Expect(p.dbLocalState.InitPGParameters).To(HaveKeyWithValue("shared_buffers", "1GB"))
		})

		DescribeTable("doesn't go on when a step fails",
			func(method string) {
				p, f, s := smKeeper(GinkgoTB())
				s.cd = smClusterData(db)
				f.initialized, f.role = true, common.RolePrimary
				f.errs[method] = errFake
				p.postgresKeeperSM(ctx)
				Expect(converged(p, db)).To(BeFalse(), "went on: %v, %+v", f.calls, p.dbLocalState)
				Expect(f.called("Start")).To(BeFalse(), "went on: %v", f.calls)
			},
			Entry(nil, "StopIfStarted"),
			Entry(nil, "StartTmpMerged"),
			Entry(nil, "WaitReady"),
			Entry(nil, "GetConfigFilePGParameters"),
		)
	})

	DescribeTable("doesn't go on with an unusable init mode",
		func(mode cluster.DBInitMode) {
			p, f, s := smKeeper(GinkgoTB())
			s.cd = smClusterData(smDB("db1", common.RolePrimary, mode))
			p.postgresKeeperSM(ctx)
			Expect(p.dbLocalState.UID).To(BeEmpty(), "went on: %v, %+v", f.calls, p.dbLocalState)
			Expect(f.called("Start")).To(BeFalse(), "went on: %v", f.calls)
		},
		Entry("no db", cluster.NoDB),
		Entry("unknown", cluster.DBInitMode("unknown")),
	)

	Describe("a resync", func() {
		syncs := func(f *fakePG) string { return f.sequence("SyncFromFollowedPGRewind", "SyncFromFollowed") }

		It("uses pg_basebackup when not initialized", func() {
			p, f, db := resyncKeeper(GinkgoTB(), false)
			p.postgresKeeperSM(ctx)
			Expect(syncs(f)).To(Equal("SyncFromFollowed"))
			Expect(converged(p, db)).To(BeTrue(), "not converged: %+v", p.dbLocalState)
			Expect(f.started).To(BeTrue(), "not started")
			Expect(f.recovery.RecoveryMode).To(Equal(pg.RecoveryModeStandby), "recovery options = %+v", f.recovery)
			Expect(f.called("Restart")).To(BeFalse(), "restarted with the recovery options of the resync: %v",
				f.calls)
		})
		It("uses pg_rewind for the same system", func() {
			p, f, db := resyncKeeper(GinkgoTB(), true)
			p.postgresKeeperSM(ctx)
			Expect(syncs(f)).To(Equal("SyncFromFollowedPGRewind"))
			Expect(converged(p, db)).To(BeTrue(), "not converged: %+v", p.dbLocalState)
		})
		It("uses pg_basebackup when pg_rewind fails", func() {
			p, f, db := resyncKeeper(GinkgoTB(), true)
			f.errs["SyncFromFollowedPGRewind"] = errFake
			p.postgresKeeperSM(ctx)
			Expect(syncs(f)).To(Equal("SyncFromFollowedPGRewind,SyncFromFollowed"))
			Expect(converged(p, db)).To(BeTrue(), "not converged: %+v", p.dbLocalState)
		})
		It("uses pg_basebackup when the rewound standby is not ready", func() {
			p, f, db := resyncKeeper(GinkgoTB(), true)
			f.waitReady = func() error { return errFake }
			p.postgresKeeperSM(ctx)
			Expect(syncs(f)).To(Equal("SyncFromFollowedPGRewind,SyncFromFollowed"))
			Expect(converged(p, db)).To(BeTrue(), "not converged: %+v", p.dbLocalState)
		})
		It("uses pg_basebackup when the rewound standby is on another timeline", func() {
			p, f, _ := resyncKeeper(GinkgoTB(), true)
			f.systemData.TimelineID = 2
			p.postgresKeeperSM(ctx)
			Expect(syncs(f)).To(Equal("SyncFromFollowedPGRewind,SyncFromFollowed"))
		})
		It("uses pg_basebackup for another system", func() {
			p, f, _ := resyncKeeper(GinkgoTB(), true)
			f.systemID = "sys2"
			p.postgresKeeperSM(ctx)
			Expect(syncs(f)).To(Equal("SyncFromFollowed"))
		})
		It("uses pg_basebackup without a master", func() {
			p, f, _ := resyncKeeper(GinkgoTB(), true)
			p.e.(*fakeStore).cd.Cluster.Status.Master = ""
			p.postgresKeeperSM(ctx)
			Expect(syncs(f)).To(Equal("SyncFromFollowed"))
		})

		DescribeTable("doesn't go on when it fails",
			func(setup func(p *PostgresKeeper, f *fakePG)) {
				p, f, db := resyncKeeper(GinkgoTB(), true)
				setup(p, f)
				p.postgresKeeperSM(ctx)
				Expect(converged(p, db)).To(BeFalse(), "went on: %v, %+v", f.calls, p.dbLocalState)
				Expect(p.dbLocalState.Initializing).To(BeTrue(), "went on: %v, %+v", f.calls, p.dbLocalState)
			},
			Entry("stop", func(_ *PostgresKeeper, f *fakePG) { f.errs["StopIfStarted"] = errFake }),
			Entry("system id", func(_ *PostgresKeeper, f *fakePG) { f.errs["GetSystemID"] = errFake }),
			Entry("followed db missing", func(p *PostgresKeeper, _ *fakePG) {
				delete(p.e.(*fakeStore).cd.DBs, "primary")
			}),
			Entry("sync", func(_ *PostgresKeeper, f *fakePG) {
				f.errs["SyncFromFollowedPGRewind"] = errFake
				f.errs["SyncFromFollowed"] = errFake
			}),
			Entry("remove data", func(_ *PostgresKeeper, f *fakePG) {
				f.errs["SyncFromFollowedPGRewind"] = errFake
				f.errs["RemoveAllIfInitialized"] = errFake
			}),
			Entry("start", func(_ *PostgresKeeper, f *fakePG) { f.errs["Start"] = errFake }),
			Entry("full resync after pg_rewind", func(_ *PostgresKeeper, f *fakePG) {
				f.waitReady = func() error { return errFake }
				f.errs["SyncFromFollowed"] = errFake
			}),
			Entry("stop for a full resync", func(_ *PostgresKeeper, f *fakePG) {
				f.waitReady = func() error {
					f.errs["StopIfStarted"] = errFake
					return errFake
				}
			}),
		)
	})

	Describe("a primary", func() {
		It("promotes a stopped standby", func() {
			p, f, db := primaryKeeper(GinkgoTB(), common.RoleReplica)
			p.postgresKeeperSM(ctx)
			Expect(f.sequence("Start", "WaitReady", "Promote")).To(Equal("Start,WaitReady,Promote"))
			Expect(converged(p, db)).To(BeTrue(), "not converged: %+v", p.dbLocalState)
			Expect(f.role).To(Equal(common.RolePrimary))
		})
		It("can't be an uninitialized instance", func() {
			p, f, db := primaryKeeper(GinkgoTB(), common.RolePrimary)
			f.initialized = false
			p.postgresKeeperSM(ctx)
			Expect(f.called("Start")).To(BeFalse(), "went on: %v", f.calls)
			Expect(converged(p, db)).To(BeFalse(), "went on: %v", f.calls)
		})
		It("with synchronous replication allows no normal users until the standbys are in sync", func() {
			p, f, db := primaryKeeper(GinkgoTB(), common.RolePrimary)
			db.Spec.SynchronousReplication = true
			db.Spec.SynchronousStandbys = []string{"db2"}
			p.postgresKeeperSM(ctx)
			Expect(p.waitSyncStandbysSynced).To(BeTrue(), "not waiting for the sync standbys: %v", f.calls)
			Expect(converged(p, db)).To(BeTrue(), "not converged: %v", f.calls)
			internalHBA := f.curHba

			f.syncStandbys = []string{common.OrionName("db2"), "other"}
			f.calls = nil
			p.postgresKeeperSM(ctx)
			Expect(p.waitSyncStandbysSynced).To(BeFalse(), "normal users not allowed once in sync: %v", f.calls)
			Expect(f.called("Reload")).To(BeTrue(), "normal users not allowed once in sync: %v", f.calls)
			Expect(len(f.curHba)).To(BeNumerically(">", len(internalHBA)), "hba %v", f.curHba)
		})
		It("doesn't converge when the sync standbys can't be read", func() {
			p, f, db := primaryKeeper(GinkgoTB(), common.RolePrimary)
			db.Spec.SynchronousReplication = true
			f.errs["GetSyncStandbys"] = errFake
			p.postgresKeeperSM(ctx)
			Expect(converged(p, db)).To(BeFalse(), "converged: %v", f.calls)
		})

		DescribeTable("doesn't converge when a step fails",
			func(method string) {
				p, f, db := primaryKeeper(GinkgoTB(), common.RoleReplica)
				db.Spec.Followers = []string{"db2"}
				f.errs[method] = errFake
				p.postgresKeeperSM(ctx)
				Expect(converged(p, db)).To(BeFalse(), "converged: %v", f.calls)
			},
			Entry(nil, "IsInitialized"),
			Entry(nil, "GetRole"),
			Entry(nil, "IsStarted"),
			Entry(nil, "Start"),
			Entry(nil, "WaitReady"),
			Entry(nil, "Promote"),
			Entry(nil, "GetReplicationSlots"),
			Entry(nil, "CreateReplicationSlot"),
		)
	})

	Describe("a replica", func() {
		It("starts a stopped standby, and restarts it for new recovery options", func() {
			p, f, db := replicaKeeper(GinkgoTB(), common.RoleReplica)
			p.postgresKeeperSM(ctx)
			// a stopped standby starts with the recovery options of its primary: no restart
			Expect(f.sequence("Start", "Restart")).To(Equal("Start"))
			Expect(converged(p, db)).To(BeTrue(), "not converged: %+v", p.dbLocalState)
			f.calls = nil
			p.postgresKeeperSM(ctx)
			Expect(f.called("Start") || f.called("Restart")).To(BeFalse(), "second pass: %v", f.calls)

			// the primary moves
			p.e.(*fakeStore).cd.DBs["primary"].Status.ListenAddress = "10.0.0.3"
			f.calls = nil
			p.postgresKeeperSM(ctx)
			Expect(f.called("Restart")).To(BeTrue(), "not restarted for a new primary_conninfo: %v", f.calls)
		})
		It("follows an external primary", func() {
			p, f, db := replicaKeeper(GinkgoTB(), common.RoleReplica)
			db.Spec.FollowConfig = &cluster.FollowConfig{
				Type:            cluster.FollowTypeExternal,
				StandbySettings: &cluster.StandbySettings{PrimaryConninfo: "host=elsewhere"},
			}
			f.started = true
			p.postgresKeeperSM(ctx)
			Expect(f.called("Restart")).To(BeTrue(), "not restarted for the external primary: %v", f.calls)
			Expect(f.curRecovery.RecoveryParameters).To(HaveKeyWithValue("primary_conninfo", "host=elsewhere"))
			Expect(converged(p, db)).To(BeTrue(), "not converged: %+v", p.dbLocalState)
		})

		DescribeTable("doesn't converge",
			func(setup func(p *PostgresKeeper, f *fakePG, db *cluster.DB)) {
				p, f, db := replicaKeeper(GinkgoTB(), common.RoleReplica)
				setup(p, f, db)
				p.postgresKeeperSM(ctx)
				Expect(converged(p, db)).To(BeFalse(), "converged: %v", f.calls)
			},
			Entry("unknown follow type", func(_ *PostgresKeeper, _ *fakePG, db *cluster.DB) {
				db.Spec.FollowConfig.Type = "unknown"
			}),
			Entry("followed db missing", func(p *PostgresKeeper, _ *fakePG, _ *cluster.DB) {
				delete(p.e.(*fakeStore).cd.DBs, "primary")
			}),
			Entry("a primary can't become a standby", func(_ *PostgresKeeper, f *fakePG, _ *cluster.DB) {
				f.role = common.RolePrimary
			}),
			Entry("not initialized", func(_ *PostgresKeeper, f *fakePG, _ *cluster.DB) { f.initialized = false }),
			Entry("IsStarted fails", func(_ *PostgresKeeper, f *fakePG, _ *cluster.DB) {
				f.errs["IsStarted"] = errFake
			}),
			Entry("Start fails", func(_ *PostgresKeeper, f *fakePG, _ *cluster.DB) { f.errs["Start"] = errFake }),
			Entry("Restart fails", func(_ *PostgresKeeper, f *fakePG, _ *cluster.DB) {
				f.started = true // with other recovery options
				f.errs["Restart"] = errFake
			}),
			Entry("Restart fails for an external primary", func(_ *PostgresKeeper, f *fakePG, db *cluster.DB) {
				db.Spec.FollowConfig = &cluster.FollowConfig{Type: cluster.FollowTypeExternal}
				f.started = true
				f.curRecovery.RecoveryParameters["primary_conninfo"] = "old"
				f.errs["Restart"] = errFake
			}),
		)

		// the replication slots are best effort on a standby
		DescribeTable("converges when the replication slots fail",
			func(external bool) {
				p, f, db := replicaKeeper(GinkgoTB(), common.RoleReplica)
				if external {
					db.Spec.FollowConfig = &cluster.FollowConfig{Type: cluster.FollowTypeExternal}
				}
				f.errs["GetReplicationSlots"] = errFake
				p.postgresKeeperSM(ctx)
				Expect(converged(p, db)).To(BeTrue(), "not converged after failing replication slots: %v", f.calls)
			},
			Entry("internal primary", false),
			Entry("external primary", true),
		)
	})

	It("doesn't go on without a role", func() {
		p, f, db := primaryKeeper(GinkgoTB(), common.RolePrimary)
		db.Spec.Role = common.RoleUndefined
		p.postgresKeeperSM(ctx)
		Expect(converged(p, db)).To(BeFalse(), "went on: %v", f.calls)
		Expect(f.called("Start")).To(BeFalse(), "went on: %v", f.calls)
	})

	Describe("parameter changes", func() {
		// converge first, then change the spec
		setup := func(autoRestart bool) (*PostgresKeeper, *fakePG, *cluster.DB) {
			p, f, db := primaryKeeper(GinkgoTB(), common.RolePrimary)
			p.e.(*fakeStore).cd.Cluster.Spec.AutomaticPgRestart = util.ToPtr(autoRestart)
			p.postgresKeeperSM(ctx)
			Expect(converged(p, db)).To(BeTrue(), "not converged: %v", f.calls)
			f.calls = nil
			db.Spec.PGParameters = cluster.PGParameters{"max_connections": "200"}
			db.Generation++
			return p, f, db
		}

		It("reload", func() {
			p, f, db := setup(false)
			p.postgresKeeperSM(ctx)
			Expect(f.called("Reload")).To(BeTrue(), "not reloaded: %v", f.calls)
			Expect(f.curParams).To(HaveKeyWithValue("max_connections", "200"))
			Expect(converged(p, db)).To(BeTrue(), "not converged: %v", f.calls)
		})
		It("don't stop the pass when the reload fails", func() {
			p, f, db := setup(false)
			f.errs["Reload"] = errFake
			p.postgresKeeperSM(ctx)
			Expect(converged(p, db)).To(BeTrue(), "a failed reload stops the pass: %v", f.calls)
		})
		DescribeTable("that require a restart restart only with automatic restarts",
			func(auto bool) {
				p, f, db := setup(auto)
				f.restartRequired = true
				p.postgresKeeperSM(ctx)
				Expect(f.called("Restart")).To(Equal(auto), "calls %v", f.calls)
				Expect(converged(p, db)).To(BeTrue(), "not converged: %v", f.calls)
			},
			Entry("no automatic restart", false),
			Entry("automatic restart", true),
		)
		DescribeTable("don't stop the pass when the restart fails, or can't tell",
			func(method string) {
				p, f, db := setup(true)
				f.restartRequired = true
				f.errs[method] = errFake
				p.postgresKeeperSM(ctx)
				Expect(converged(p, db)).To(BeTrue(), "a failure stops the pass: %v", f.calls)
			},
			Entry(nil, "Restart"),
			Entry(nil, "IsRestartRequired"),
		)
		It("turn synchronous replication on and off", func() {
			p, f, db := setup(false)
			db.Spec.SynchronousReplication = true
			db.Spec.SynchronousStandbys = []string{"db2"}
			f.syncStandbys = []string{common.OrionName("db2")}
			p.postgresKeeperSM(ctx)
			Expect(f.params["synchronous_standby_names"]).NotTo(BeEmpty(), "synchronous_standby_names not set")
			Expect(converged(p, db)).To(BeTrue(), "not converged: %v", f.calls)
			db.Spec.SynchronousReplication = false
			db.Generation++
			p.postgresKeeperSM(ctx)
			Expect(f.params["synchronous_standby_names"]).To(BeEmpty(), "synchronous_standby_names not cleared")
			Expect(converged(p, db)).To(BeTrue(), "not converged: %v", f.calls)
		})
		It("don't converge when the generation can't be saved", func() {
			p, f, db := setup(false)
			p.cfg.DataDir = filepath.Join(p.cfg.DataDir, "missing")
			p.postgresKeeperSM(ctx)
			Expect(converged(p, db)).To(BeFalse(), "converged without saving: %v", f.calls)
		})
	})
})
