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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/timer"
	"github.com/pgvillage-tools/orion/internal/util"
)

// The Initializing phase of updateCluster: choosing the keeper of the first db (new, existing and PITR clusters),
// and waiting for that db to converge before the cluster goes to the Normal phase.
var _ = Describe("updateCluster in the Initializing phase", func() {
	const noKeepersErr = "cannot choose initial master: no keepers registered"

	// initCD returns an Initializing cluster with the init mode of spec and keepers k1 and k2. With master set, db1
	// on k1 is the db chosen to initialize the cluster, at generation 2 and converged to currentGeneration.
	initCD := func(spec *cluster.Spec, master bool, currentGeneration int64) *cluster.Data {
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

	// initUpdate runs updateCluster on cd; converging sets the convergence timer of db1 to now (so it is converging,
	// not failed)
	initUpdate := func(cd *cluster.Data, converging bool) (*cluster.Data, error) {
		s := etSentinel(cd)
		if converging {
			s.dbConvergenceInfos["db1"].Timer = timer.Now()
		}
		return s.updateCluster(context.Background(), cd, cluster.ProxiesInfo{})
	}

	mustInitUpdate := func(cd *cluster.Data, converging bool) *cluster.Data {
		GinkgoHelper()
		outcd, err := initUpdate(cd, converging)
		Ω(err).NotTo(HaveOccurred())
		return outcd
	}

	// checkInitialized checks that the cluster went to the Normal phase with db1 as its master, which no longer has
	// an init mode or includes its previous config, and the cluster parameters of db1 when merged
	checkInitialized := func(outcd *cluster.Data, merged bool) {
		GinkgoHelper()
		Ω(outcd.Cluster.Status.Phase).To(Equal(cluster.Normal))
		Ω(outcd.DBs).To(HaveKey("db1"))
		db := outcd.DBs["db1"]
		Ω(db.Spec.InitMode).To(Equal(cluster.NoDB))
		Ω(db.Spec.IncludeConfig).To(BeFalse())
		if merged {
			Ω(outcd.Cluster.Spec.PGParameters).To(HaveKeyWithValue("max_connections", "150"))
		} else {
			Ω(outcd.Cluster.Spec.PGParameters).NotTo(HaveKeyWithValue("max_connections", "150"))
		}
	}

	// checkStillInitializing checks that the cluster is still Initializing with db1 as the db to initialize it
	checkStillInitializing := func(outcd *cluster.Data) {
		GinkgoHelper()
		Ω(outcd.Cluster.Status.Phase).To(Equal(cluster.Initializing))
		Ω(outcd.Cluster.Status.Master).To(Equal("db1"))
		Ω(outcd.DBs).To(HaveKey("db1"))
	}

	// checkNewMasterDB checks that the cluster has one db, its master, on keeper with init mode and role, and
	// returns it
	checkNewMasterDB := func(outcd *cluster.Data, keeper string, mode cluster.DBInitMode,
		role common.Role) *cluster.DB {
		GinkgoHelper()
		Ω(outcd.DBs).To(HaveLen(1))
		db := outcd.DBs[outcd.Cluster.Status.Master]
		Ω(db).NotTo(BeNil(), "master %q has no db", outcd.Cluster.Status.Master)
		Ω(db.Spec.KeeperUID).To(Equal(keeper))
		Ω(db.Spec.InitMode).To(Equal(mode))
		Ω(db.Spec.Role).To(Equal(role))
		Ω(db.Generation).To(BeEquivalentTo(cluster.InitialGeneration))
		Ω(db.Spec.IncludeConfig).To(BeTrue(), "include config: want the default (merge) true")
		return db
	}

	Context("of a new cluster", func() {
		It("chooses a keeper for the first db", func() {
			spec := initSpec(cluster.New)
			spec.NewConfig = &cluster.NewConfig{Locale: "C"}
			outcd := mustInitUpdate(initCD(spec, false, 0), false)
			db := checkNewMasterDB(outcd, "keeper1", cluster.NewDB, common.RolePrimary)
			Ω(db.Spec.NewConfig).NotTo(BeNil())
			Ω(db.Spec.NewConfig.Locale).To(Equal("C"))
		})
		It("needs a keeper", func() {
			cd := initCD(initSpec(cluster.New), false, 0)
			cd.Keepers = cluster.Keepers{}
			_, err := initUpdate(cd, false)
			Ω(err).To(MatchError(ContainSubstring(noKeepersErr)))
		})
		It("is initialized when the db converged healthy", func() {
			checkInitialized(mustInitUpdate(initCD(initSpec(cluster.New), true, 2), false), true)
		})
		It("is initialized without merging the parameters of initdb", func() {
			spec := initSpec(cluster.New)
			spec.MergePgParameters = util.ToPtr(false)
			checkInitialized(mustInitUpdate(initCD(spec, true, 2), false), false)
		})
		It("waits for a converged db to be healthy", func() {
			cd := initCD(initSpec(cluster.New), true, 2)
			cd.DBs["db1"].Status.Healthy = false
			checkStillInitializing(mustInitUpdate(cd, false))
		})
		It("waits for a converging db", func() {
			checkStillInitializing(mustInitUpdate(initCD(initSpec(cluster.New), true, 1), true))
		})
		It("starts over when the db failed to initialize, so another keeper can be chosen", func() {
			outcd := mustInitUpdate(initCD(initSpec(cluster.New), true, 1), false)
			Ω(outcd.DBs).To(BeEmpty())
			Ω(outcd.Cluster.Status.Master).To(BeEmpty())
		})
	})

	Context("of an existing cluster", func() {
		It("uses the keeper of the existing config", func() {
			outcd := mustInitUpdate(initCD(initSpec(cluster.ExistingCluster), false, 0), false)
			checkNewMasterDB(outcd, "keeper2", cluster.ExistingDB, common.RolePrimary)
		})
		It("needs the keeper of the existing config", func() {
			spec := initSpec(cluster.ExistingCluster)
			spec.ExistingConfig.KeeperUID = "keeper9"
			_, err := initUpdate(initCD(spec, false, 0), false)
			Ω(err).To(MatchError(ContainSubstring(`keeper "keeper9" state not available`)))
		})
		It("is initialized when the db converged healthy", func() {
			checkInitialized(mustInitUpdate(initCD(initSpec(cluster.ExistingCluster), true, 2), false), true)
		})
		It("is initialized without merging the parameters of the existing db", func() {
			spec := initSpec(cluster.ExistingCluster)
			spec.MergePgParameters = util.ToPtr(false)
			checkInitialized(mustInitUpdate(initCD(spec, true, 2), false), false)
		})
		It("waits for a db that hasn't converged", func() {
			// an existing db never fails to initialize: it keeps waiting
			checkStillInitializing(mustInitUpdate(initCD(initSpec(cluster.ExistingCluster), true, 1), false))
		})
	})

	Context("of a PITR cluster", func() {
		It("chooses a keeper for the restored primary", func() {
			outcd := mustInitUpdate(initCD(initSpec(cluster.PITR), false, 0), false)
			db := checkNewMasterDB(outcd, "keeper1", cluster.PITRDB, common.RolePrimary)
			Ω(db.Spec.PITRConfig).NotTo(BeNil())
			Ω(db.Spec.PITRConfig.DataRestoreCommand).To(Equal("restore"))
			Ω(db.Spec.FollowConfig).To(BeNil())
		})
		It("restores a standby of the external primary in a standby cluster", func() {
			spec := initSpec(cluster.PITR)
			spec.Role = util.ToPtr(cluster.Replica)
			spec.StandbyConfig = &cluster.StandbyConfig{
				StandbySettings: &cluster.StandbySettings{PrimaryConninfo: "host=elsewhere"},
			}
			outcd := mustInitUpdate(initCD(spec, false, 0), false)
			db := checkNewMasterDB(outcd, "keeper1", cluster.PITRDB, common.RoleReplica)
			fc := db.Spec.FollowConfig
			Ω(fc).NotTo(BeNil())
			Ω(fc.Type).To(Equal(cluster.FollowTypeExternal))
			Ω(fc.StandbySettings).NotTo(BeNil())
			Ω(fc.StandbySettings.PrimaryConninfo).To(Equal("host=elsewhere"))
		})
		It("needs a keeper", func() {
			cd := initCD(initSpec(cluster.PITR), false, 0)
			cd.Keepers = cluster.Keepers{}
			_, err := initUpdate(cd, false)
			Ω(err).To(MatchError(ContainSubstring(noKeepersErr)))
		})
		It("is initialized when the db converged healthy", func() {
			checkInitialized(mustInitUpdate(initCD(initSpec(cluster.PITR), true, 2), false), true)
		})
		It("is initialized without merging the parameters of the restored db", func() {
			spec := initSpec(cluster.PITR)
			spec.MergePgParameters = util.ToPtr(false)
			checkInitialized(mustInitUpdate(initCD(spec, true, 2), false), false)
		})
		It("waits for a converged db to be healthy", func() {
			cd := initCD(initSpec(cluster.PITR), true, 2)
			cd.DBs["db1"].Status.Healthy = false
			checkStillInitializing(mustInitUpdate(cd, false))
		})
		It("waits for a restore without a timeout", func() {
			// a restore has no timeout (TODO(sgotti) in updateCluster), so an old convergence timer doesn't fail it
			checkStillInitializing(mustInitUpdate(initCD(initSpec(cluster.PITR), true, 1), false))
		})
	})

	It("fails on an unknown init mode", func() {
		_, err := initUpdate(initCD(initSpec("unknown"), false, 0), false)
		Ω(err).To(MatchError(ContainSubstring("unknown init mode unknown")))
	})

	DescribeTable("panics when the db chosen to initialize the cluster is missing",
		func(mode cluster.InitMode) {
			cd := initCD(initSpec(mode), true, 2)
			delete(cd.DBs, "db1")
			Ω(func() { _, _ = initUpdate(cd, false) }).To(
				PanicWith(MatchError(ContainSubstring(`db "db1" object doesn't exists`))))
		},
		Entry("of a new cluster", cluster.New),
		Entry("of an existing cluster", cluster.ExistingCluster),
		Entry("of a PITR cluster", cluster.PITR),
	)
})

var _ = DescribeTable("updateCluster in the Initializing phase (#0-#3)",
	checkUpdateCluster,
	// Init phase, also test dbSpec parameters copied from clusterSpec.
	Entry("#0 cluster initialization, no keepers", updateClusterCase{
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
	}),
	Entry("#1 cluster initialization, one keeper", updateClusterCase{
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
	}),
	Entry("#2 cluster initialization, more than one keeper, the first will be chosen to be the new "+
		"master.", updateClusterCase{
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
	}),
	Entry("#3 cluster initialization, keeper initialization failed", updateClusterCase{
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
	}),
)
