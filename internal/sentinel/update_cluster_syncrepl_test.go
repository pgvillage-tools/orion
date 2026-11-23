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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/util"
)

var _ = Describe("updateCluster with synchronous replication", func() {
	// minSyncZeroSpec returns etTableSyncSpec with a MinSynchronousStandbys of 0
	minSyncZeroSpec := func() *cluster.Spec {
		spec := etTableSyncSpec()
		spec.MinSynchronousStandbys = util.ToPtr(uint16(0))
		return spec
	}

	// syncMaster enables synchronous replication on db1 of cd with the synchronous standbys standbys, which db1
	// reported as reported, and returns db1
	syncMaster := func(cd *cluster.Data, reported []string, standbys ...string) *cluster.DB {
		master := etSyncStandbys(cd.DBs[d1], standbys...)
		master.Status.SynchronousStandbys = reported
		return master
	}

	DescribeTable("(#15-#31)",
		checkUpdateCluster,
		Entry("#15 One master and one standby all healthy. Synchronous replication enabled right now in "+
			"the cluster spec.", func() updateClusterCase {
			cd := func() *cluster.Data {
				cd := etTableCluster(etTableSyncSpec(), d2)
				cd.DBs[d1].Status.Healthy = false
				return cd
			}
			outcd := etElected(cd(), d2)
			etSyncStandbys(outcd.DBs[d2]).Spec.ExternalSynchronousStandbys = []string{fakeStandbyName}
			return updateClusterCase{cd: cd(), outcd: outcd}
		}()),
		Entry("#16 One master and one standby. Synchronous replication enabled right now in the cluster "+
			"spec. master db not healthy: standby elected as new master since "+
			"dbSpec.SynchronousReplication is false yet. The new master will have "+
			"SynchronousReplication true and a fake sync stanby.", func() updateClusterCase {
			cd := func() *cluster.Data {
				cd := etTableCluster(etTableSyncSpec(), d2)
				cd.DBs[d1].Status.Healthy = false
				return cd
			}
			outcd := etElected(cd(), d2)
			etSyncStandbys(outcd.DBs[d2]).Spec.ExternalSynchronousStandbys = []string{fakeStandbyName}
			return updateClusterCase{cd: cd(), outcd: outcd}
		}()),
		Entry("#17 One master and one standby. Synchronous replication already enabled. master db not "+
			"healthy: standby elected as new master since it's in the SynchronousStandbys.",
			func() updateClusterCase {
				cd := func() *cluster.Data {
					cd := etTableCluster(etTableSyncSpec(), d2)
					syncMaster(cd, []string{d2}, d2).Status.Healthy = false
					return cd
				}
				outcd := etElected(cd(), d2)
				etSyncStandbys(outcd.DBs[d2], d1)
				return updateClusterCase{cd: cd(), outcd: outcd}
			}()),
		Entry("#18 One master and one standby. Synchronous replication already enabled. stanby db not "+
			"healthy: standby kept inside synchronousStandbys since there's not better standby to "+
			"choose", func() updateClusterCase {
			cd := func() *cluster.Data {
				cd := etTableCluster(etTableSyncSpec(), d2)
				syncMaster(cd, []string{d2}, d2)
				cd.DBs[d2].Status.Healthy = false
				return cd
			}
			return updateClusterCase{cd: cd(), outcd: cd()}
		}()),
		Entry("#19 One master and two standbys. Synchronous replication already enabled with "+
			"MinSynchronousStandbys and MaxSynchronousStandbys to 1 (default). sync standby db2 not "+
			"healthy: the other standby db3 choosed as new sync standby. Both will appear as "+
			"SynchronousStandbys", func() updateClusterCase {
			cd := func() *cluster.Data {
				cd := etInitModeNone(etTableCluster(etTableSyncSpec(), d2, d3))
				syncMaster(cd, []string{d2}, d2)
				cd.DBs[d2].Status.Healthy = false
				return cd
			}
			outcd := cd()
			outcd.DBs[d1].Generation = g2
			outcd.DBs[d1].Spec.SynchronousStandbys = []string{d2, d3}
			return updateClusterCase{cd: cd(), outcd: outcd}
		}()),
		Entry("#20 From previous. master haven't yet reported the new sync standbys (db2, db3). master "+
			"db is not healty. db2 is the unique in common between the reported (db2) and the required "+
			"in the spec (db2, db3) but it's not healty so no master could be elected.", func() updateClusterCase {
			cd := func() *cluster.Data {
				cd := etInitModeNone(etTableCluster(etTableSyncSpec(), d2, d3))
				master := syncMaster(cd, []string{d2}, d2, d3)
				master.Generation = g2
				master.Status.Healthy = false
				cd.DBs[d2].Status.Healthy = false
				return cd
			}
			return updateClusterCase{cd: cd(), outcd: cd()}
		}()),
		Entry("#21 From #19. master have not yet reported the new sync standbys as in sync (db3). db2 "+
			"will remain the unique real in sync db in db1.Status.SynchronousStandbys", func() updateClusterCase {
			cd := func() *cluster.Data {
				cd := etInitModeNone(etTableCluster(etTableSyncSpec(), d2, d3))
				master := syncMaster(cd, []string{d2}, d2, d3)
				master.Generation, master.Status.CurrentGeneration = g2, g2
				master.Status.CurSynchronousStandbys = []string{}
				cd.DBs[d2].Status.Healthy = false
				return cd
			}
			return updateClusterCase{cd: cd(), outcd: cd()}
		}()),
		Entry("#22 From #21. master have not yet reported the new sync standbys as in sync (db3) and "+
			"db3 is failed. db2 will remain the unique real in sync db in "+
			"db1.Status.SynchronousStandbys and also db1.Spec.SynchronousStandbys will contain only db2 "+
			"(db3 removed)", func() updateClusterCase {
			cd := func() *cluster.Data {
				cd := etInitModeNone(etTableCluster(etTableSyncSpec(), d2, d3))
				master := syncMaster(cd, []string{d2}, d2, d3)
				master.Generation, master.Status.CurrentGeneration = g2, g2
				master.Status.CurSynchronousStandbys = []string{}
				cd.DBs[d2].Status.Healthy = false
				cd.DBs[d3].Status.Healthy = false
				return cd
			}
			outcd := cd()
			outcd.DBs[d1].Generation = g3
			outcd.DBs[d1].Spec.SynchronousStandbys = []string{d2}
			return updateClusterCase{cd: cd(), outcd: outcd}
		}()),
		Entry("#23 From #19. master have reported the new sync standbys as in sync (db2, db3). db2 will "+
			"be removed from synchronousStandbys", func() updateClusterCase {
			cd := func() *cluster.Data {
				cd := etInitModeNone(etTableCluster(etTableSyncSpec(), d2, d3))
				master := syncMaster(cd, []string{d2, d3}, d2, d3)
				master.Generation, master.Status.CurrentGeneration = g2, g2
				master.Status.CurSynchronousStandbys = []string{d3}
				cd.DBs[d2].Status.Healthy = false
				return cd
			}
			outcd := cd()
			outcd.DBs[d1].Generation = g3
			outcd.DBs[d1].Spec.SynchronousStandbys = []string{d3}
			outcd.DBs[d1].Status.SynchronousStandbys = []string{d3}
			return updateClusterCase{cd: cd(), outcd: outcd}
		}()),
		Entry("#24 From previous. master db is not healty. db3 is the unique in common between the "+
			"reported (db2, db3) and the required in the spec (db3) so it'll be elected as new master.",
			func() updateClusterCase {
				cd := func() *cluster.Data {
					cd := etInitModeNone(etTableCluster(etTableSyncSpec(), d2, d3))
					master := syncMaster(cd, []string{d2, d3}, d3)
					master.Generation, master.Status.CurrentGeneration = g3, g2
					master.Status.Healthy = false
					cd.DBs[d2].Status.Healthy = false
					return cd
				}
				outcd := etElected(cd(), d3)
				etSyncStandbys(outcd.DBs[d3], d1)
				return updateClusterCase{cd: cd(), outcd: outcd}
			}()),
		Entry("#25 One master and two standbys. Synchronous replication already enabled with "+
			"MinSynchronousStandbys and MaxSynchronousStandbys to 2. master (db1) and db2 failed, db3 "+
			"elected as master. This test checks that the db3 synchronousStandbys are correctly sorted",
			func() updateClusterCase {
				cd := func() *cluster.Data {
					cd := etInitModeNone(etTableCluster(etTableSyncSpec(), d2, d3))
					syncMaster(cd, []string{d2, d3}, d2, d3).Status.Healthy = false
					cd.DBs[d2].Status.Healthy = false
					return cd
				}
				outcd := etElected(cd(), d3)
				etSyncStandbys(outcd.DBs[d3], d1, d2)
				return updateClusterCase{cd: cd(), outcd: outcd}
			}()),
		Entry("#26 One master (unhealthy) and an async standby. Synchronous replication already enabled "+
			"with MinSynchronousStandbys and MaxSynchronousStandbys to 1 (default) master (db1) and "+
			"async (db2) with --never-synchronous-replica. db2 is never elected as new sync.",
			func() updateClusterCase {
				cd := func() *cluster.Data {
					cd := etInitModeNone(etTableCluster(etTableSyncSpec(), d2))
					cd.Keepers[k2].Status.CanBeSynchronousReplica = util.ToPtr(false)
					master := syncMaster(cd, []string{})
					master.Spec.ExternalSynchronousStandbys = []string{fakeStandbyName}
					master.Status.CurSynchronousStandbys = []string{}
					return cd
				}
				return updateClusterCase{cd: cd(), outcd: cd()}
			}()),
		Entry("#27 One master (unhealthy) and a sync standby. Synchronous replication already enabled "+
			"with MinSynchronousStandbys and MaxSynchronousStandbys to 1 (default) master (db1) and "+
			"sync (db2) with --never-master. db2 is never promoted as new master.", func() updateClusterCase {
			cd := func() *cluster.Data {
				cd := etInitModeNone(etTableCluster(etTableSyncSpec(), d2))
				cd.Keepers[k2].Status.CanBeMaster = util.ToPtr(false)
				master := syncMaster(cd, []string{d2}, d2)
				master.Status.Healthy = false
				master.Status.CurSynchronousStandbys = []string{d2}
				return cd
			}
			return updateClusterCase{cd: cd(), outcd: cd()}
		}()),
		Entry("#28 One master and one standby. Synchronous replication enabled right now in the cluster "+
			"spec, with MinSynchronousStandbys=0. master db not healthy: standby elected as new master "+
			"since dbSpec.SynchronousReplication is false yet. The new master will have "+
			"SynchronousReplication true and NO fake sync standby.", func() updateClusterCase {
			cd := func() *cluster.Data {
				cd := etTableCluster(minSyncZeroSpec(), d2)
				cd.DBs[d1].Status.Healthy = false
				return cd
			}
			outcd := etElected(cd(), d2)
			etSyncStandbys(outcd.DBs[d2])
			return updateClusterCase{cd: cd(), outcd: outcd}
		}()),
		Entry("#29 One master and one standby. Synchronous replication already enabled, with "+
			"MinSynchronousStandbys=0. master db not healthy: standby elected as new master since it's "+
			"in the SynchronousStandbys. No fake replica is added.", func() updateClusterCase {
			cd := func() *cluster.Data {
				cd := etTableCluster(minSyncZeroSpec(), d2)
				syncMaster(cd, []string{d2}, d2).Status.Healthy = false
				return cd
			}
			outcd := etElected(cd(), d2)
			etSyncStandbys(outcd.DBs[d2], d1)
			return updateClusterCase{cd: cd(), outcd: outcd}
		}()),
		Entry("#30 One master and one standby. Synchronous replication already enabled, with "+
			"MinSynchronousStandbys=0. standby db not healthy: standby removed from synchronousStandbys "+
			"even though there's not better standby to choose", func() updateClusterCase {
			cd := func() *cluster.Data {
				cd := etTableCluster(minSyncZeroSpec(), d2)
				syncMaster(cd, []string{d2}, d2)
				cd.DBs[d2].Status.Healthy = false
				return cd
			}
			outcd := cd()
			outcd.DBs[d1].Generation = g2
			outcd.DBs[d1].Spec.SynchronousStandbys = []string{}
			outcd.DBs[d1].Status.SynchronousStandbys = []string{}
			return updateClusterCase{cd: cd(), outcd: outcd}
		}()),
		Entry("#31 One master (unhealthy) and an async standby. Synchronous replication already enabled "+
			"enabled, with MinSynchronousStandbys=0. master (db1) and async (db2) with "+
			"--never-synchronous-replica. StrictSyncRepl is set to false. Db2 is never elected as new "+
			"sync, and fake replica is removed from db1", func() updateClusterCase {
			cd := func() *cluster.Data {
				cd := etInitModeNone(etTableCluster(minSyncZeroSpec(), d2))
				cd.Keepers[k2].Status.CanBeSynchronousReplica = util.ToPtr(false)
				master := syncMaster(cd, []string{})
				master.Spec.ExternalSynchronousStandbys = []string{fakeStandbyName}
				master.Status.CurSynchronousStandbys = []string{}
				return cd
			}
			outcd := cd()
			outcd.DBs[d1].Generation = g2
			outcd.DBs[d1].Spec.ExternalSynchronousStandbys = []string{}
			return updateClusterCase{cd: cd(), outcd: outcd}
		}()),
	)

	It("remembers when the master has fewer synchronous standbys than wanted, until it has enough", func() {
		// one master and a standby that can't be synchronous: no synchronous standby, MaxSynchronousStandbys 1
		cd := etInitModeNone(etTableCluster(minSyncZeroSpec(), d2))
		cd.Keepers[k2].Status.CanBeSynchronousReplica = util.ToPtr(false)
		syncMaster(cd, []string{})
		s := etSentinel(cd)
		_, err := s.updateCluster(context.Background(), cd, cluster.ProxiesInfo{})
		Ω(err).NotTo(HaveOccurred())
		Ω(s.syncStandbysShort).To(BeTrue())

		// the standby may be synchronous now: the sentinel chooses it
		cd.Keepers[k2].Status.CanBeSynchronousReplica = util.ToPtr(true)
		outcd, err := s.updateCluster(context.Background(), cd, cluster.ProxiesInfo{})
		Ω(err).NotTo(HaveOccurred())
		Ω(outcd.DBs[d1].Spec.SynchronousStandbys).To(ContainElement(d2))
		Ω(s.syncStandbysShort).To(BeFalse())
	})
})
