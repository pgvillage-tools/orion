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

var _ = Describe("electing a new master", func() {
	// etTLH returns a timelines history from pairs of timelineID, switchPoint.
	etTLH := func(entries ...uint64) cluster.PostgresTimelinesHistory {
		tlsh := cluster.PostgresTimelinesHistory{}
		for i := 0; i+1 < len(entries); i += 2 {
			tlsh = append(tlsh, &cluster.PostgresTimelineHistory{TimelineID: entries[i], SwitchPoint: entries[i+1]})
		}
		return tlsh
	}

	// etForkedCluster returns a cluster where master db1 is on timeline 2 (forked
	// from timeline 1 at switchPoint), db2 is on the same branch and db3 is on a
	// timeline 2 that forked at a different point. db3 is further ahead than db2.
	etForkedCluster := func(switchPoint uint64) *cluster.Data {
		const (
			masterAhead     = 1000
			sameBranchAhead = 100
			forkedAhead     = 900
			forkOffset      = 500
		)
		master := etMaster(etD1, etK1, switchPoint+masterAhead, etD2, etD3)
		master.Status.TimelineID = etTL2
		master.Status.TimelinesHistory = etTLH(etTL1, switchPoint)
		sameBranch := etStandby(etD2, etK2, etD1, switchPoint+sameBranchAhead)
		sameBranch.Status.TimelineID = etTL2
		sameBranch.Status.TimelinesHistory = etTLH(etTL1, switchPoint)
		forked := etStandby(etD3, etK3, etD1, switchPoint+forkedAhead)
		forked.Status.TimelineID = etTL2
		forked.Status.TimelinesHistory = etTLH(etTL1, switchPoint+forkOffset)
		return etClusterData(etAsyncSpec(), etD1, []*cluster.DB{master, sameBranch, forked})
	}

	// etFailedElectionCluster returns the state right after db2 was elected to
	// replace the failed master db1 (see updateCluster case #5), where db2 then
	// failed before it promoted (not healthy, not converged), and db1 is back
	// healthy. db1 still has the master role (updateCluster keeps the old
	// master's role on election) and has the most WAL.
	etFailedElectionCluster := func(spec *cluster.Spec, syncRepl bool) *cluster.Data {
		const (
			xlogPos = 10000
			behind  = 10
		)
		oldMaster := etMaster(etD1, etK1, xlogPos)
		elected := etMaster(etD2, etK2, xlogPos-behind)
		elected.Generation = etGen2
		elected.Status.Healthy = false
		if syncRepl {
			// on election, the old master replaces the new master in its list
			// of synchronous standbys
			oldMaster.Spec.SynchronousReplication = true
			oldMaster.Spec.SynchronousStandbys = []string{etD2}
			oldMaster.Status.SynchronousStandbys = []string{etD2}
			elected.Spec.SynchronousReplication = true
			elected.Spec.SynchronousStandbys = []string{etD1}
		}
		cd := etClusterData(spec, etD2, []*cluster.DB{oldMaster, elected})
		cd.Proxy.Spec.MasterDBUID = ""
		return cd
	}

	// With asynchronous replication, a standby that is further behind the failed
	// master than MaxStandbyLag is never elected, and one within the limit is.
	Describe("with asynchronous replication and a MaxStandbyLag", func() {
		const (
			maxLag        = 1000
			masterXLogPos = 100000
		)
		keepers := map[string]string{etD2: etK2, etD3: etK3}

		DescribeTable("elects only a standby within the max lag",
			func(standbyLag map[string]uint64, wantMaster string) {
				spec := etAsyncSpec()
				spec.MaxStandbyLag = util.ToPtr(uint32(maxLag))
				master := etMaster(etD1, etK1, masterXLogPos)
				master.Status.Healthy = false
				dbs := []*cluster.DB{master}
				for dbUID, lag := range standbyLag {
					master.Spec.Followers = append(master.Spec.Followers, dbUID)
					dbs = append(dbs, etStandby(dbUID, keepers[dbUID], etD1, masterXLogPos-lag))
				}
				outcd := etUpdateCluster(etClusterData(spec, etD1, dbs))
				Ω(outcd.Cluster.Status.Master).To(Equal(wantMaster))
			},
			Entry("elects no new master when the lag is over max: the failed master stays",
				map[string]uint64{etD2: maxLag + 1}, etD1),
			Entry("elects a standby with a lag of exactly max", map[string]uint64{etD2: maxLag}, etD2),
			Entry("elects a standby with a lag under max", map[string]uint64{etD2: 1}, etD2),
			Entry("elects only the standby under max when another one is over max",
				map[string]uint64{etD2: 10 * maxLag, etD3: maxLag / 2}, etD3),
		)
	})

	// The lag check directly, including a standby that reports a higher xlogpos
	// than the master (the uint64 difference wraps but is read as a negative
	// int64, so it counts as below max).
	Describe("isLagBelowMax", func() {
		const (
			maxLag        = 1000
			masterXLogPos = 5000
		)

		DescribeTable("compares the lag of a standby behind the master with MaxStandbyLag",
			func(xlogPos uint64, want bool) {
				spec := etAsyncSpec()
				spec.MaxStandbyLag = util.ToPtr(uint32(maxLag))
				cd := etClusterData(spec, etD1, nil)
				s := etSentinel(cd)
				master := &cluster.DB{UID: etD1, Status: cluster.DBStatus{XLogPos: masterXLogPos}}
				db := &cluster.DB{UID: etD2, Status: cluster.DBStatus{XLogPos: xlogPos}}
				Ω(s.isLagBelowMax(context.Background(), cd, master, db)).To(Equal(want))
			},
			Entry("is below max for a standby at the master's xlogpos", uint64(masterXLogPos), true),
			Entry("is below max for a standby exactly max behind", uint64(masterXLogPos-maxLag), true),
			Entry("is not below max for a standby more than max behind", uint64(masterXLogPos-maxLag-1), false),
			Entry("is not below max for a standby at xlogpos 0", uint64(0), false),
			Entry("is below max for a standby ahead of the master", uint64(masterXLogPos+maxLag), true),
		)
	})

	Describe("findBestNewMasters", func() {
		// The order in which candidates are returned. With async replication
		// updateCluster elects bestNewMasters[0]. The candidate with the highest
		// XLogPos comes first, so the standby that is least behind is elected and
		// the least data is lost (stolon elected the one furthest behind).
		It("returns the candidate with the highest xlogpos first, which is elected", func() {
			const masterXLogPos = 10000
			master := etMaster(etD1, etK1, masterXLogPos, etD2, etD3)
			master.Status.Healthy = false
			cd := etClusterData(etAsyncSpec(), etD1, []*cluster.DB{
				master,
				etStandby(etD2, etK2, etD1, masterXLogPos-10),
				etStandby(etD3, etK3, etD1, masterXLogPos-500),
			})
			s := etSentinel(cd)
			best := s.findBestNewMasters(context.Background(), cd, cd.DBs[etD1])
			Ω(etUIDs(best)).To(Equal([]string{etD2, etD3}))
			outcd := etUpdateCluster(cd)
			Ω(outcd.Cluster.Status.Master).To(Equal(etD2), "the standby with the highest xlogpos")
		})

		It("never returns a keeper started with --can-be-master=false", func() {
			const xlogPos = 1000
			master := etMaster(etD1, etK1, xlogPos, etD2, etD3)
			master.Status.Healthy = false
			cd := etClusterData(etAsyncSpec(), etD1, []*cluster.DB{
				master,
				etStandby(etD2, etK2, etD1, xlogPos),
				etStandby(etD3, etK3, etD1, xlogPos),
			})
			cd.Keepers[etK2].Status.CanBeMaster = util.ToPtr(false)
			s := etSentinel(cd)
			best := s.findBestNewMasters(context.Background(), cd, cd.DBs[etD1])
			Ω(etUIDs(best)).To(Equal([]string{etD3}))
		})
	})

	// All branches of isDifferentTimelineBranch (followed is the master, db the standby).
	Describe("isDifferentTimelineBranch", func() {
		const switchPoint = 1000

		DescribeTable("tells whether a standby is on another timeline branch than the master",
			func(followedStatus, dbStatus cluster.DBStatus, want bool) {
				s := &Sentinel{}
				followed := &cluster.DB{UID: etD1, Status: followedStatus}
				db := &cluster.DB{UID: etD2, Status: dbStatus}
				Ω(s.isDifferentTimelineBranch(context.Background(), followed, db)).To(Equal(want))
			},
			Entry("is different for a standby on a higher timeline than the master",
				cluster.DBStatus{TimelineID: etTL1},
				cluster.DBStatus{TimelineID: etTL2},
				true),
			Entry("is the same when both are on timeline 1 (no history)",
				cluster.DBStatus{TimelineID: etTL1},
				cluster.DBStatus{TimelineID: etTL1},
				false),
			Entry("is the same on the same timeline with the same switchpoint",
				cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
				cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
				false),
			Entry("is different on the same timeline, forked at a different switchpoint",
				cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
				cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint+1)},
				true),
			Entry("is the same on the same timeline when the standby has no history",
				cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
				cluster.DBStatus{TimelineID: etTL2},
				false),
			Entry("is the same for a standby on an older timeline, behind the master's switchpoint",
				cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
				cluster.DBStatus{TimelineID: etTL1, XLogPos: switchPoint - 1},
				false),
			Entry("is the same for a standby on an older timeline, exactly at the master's switchpoint",
				cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
				cluster.DBStatus{TimelineID: etTL1, XLogPos: switchPoint},
				false),
			Entry("is different for a standby on an older timeline, past the master's switchpoint",
				cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
				cluster.DBStatus{TimelineID: etTL1, XLogPos: switchPoint + 1},
				true),
			Entry("is the same for a standby on an older timeline the master has no history for",
				cluster.DBStatus{TimelineID: etTL3, TimelinesHistory: etTLH(etTL2, switchPoint)},
				cluster.DBStatus{TimelineID: etTL1, XLogPos: switchPoint + 1},
				false),
		)
	})

	Describe("with forked timelines", func() {
		const switchPoint = 10000

		// When the master fails, a standby on a different timeline branch is not a
		// candidate even though it is further ahead, and a standby on the same
		// branch is elected.
		It("elects the standby on the same branch, not the one further ahead on another branch", func() {
			cd := etForkedCluster(switchPoint)
			cd.DBs[etD1].Status.Healthy = false
			s := etSentinel(cd)
			Ω(s.dbValidity(context.Background(), cd, etD3)).To(Equal(dbValidityInvalid), "forked db3 validity")
			best := s.findBestNewMasters(context.Background(), cd, cd.DBs[etD1])
			Ω(etUIDs(best)).To(Equal([]string{etD2}))
			outcd := etUpdateCluster(cd)
			Ω(outcd.Cluster.Status.Master).To(Equal(etD2))
		})

		// A standby still on the previous timeline, behind the switchpoint (so on
		// the same branch and able to follow), is valid but is not a candidate for
		// master: findBestStandbys only takes standbys on the master's timeline.
		It("doesn't elect a valid standby still on the previous timeline", func() {
			cd := etForkedCluster(switchPoint)
			cd.DBs[etD1].Status.Healthy = false
			older := cd.DBs[etD3]
			older.Status.TimelineID = etTL1
			older.Status.TimelinesHistory = nil
			older.Status.XLogPos = switchPoint - 1
			s := etSentinel(cd)
			Ω(s.dbValidity(context.Background(), cd, etD3)).To(Equal(dbValidityValid), "db3 validity")
			best := s.findBestNewMasters(context.Background(), cd, cd.DBs[etD1])
			Ω(etUIDs(best)).To(Equal([]string{etD2}))
		})

		// With a healthy master, a standby on a different timeline branch is
		// removed and a new db is created on its keeper with InitMode resync.
		It("resyncs a standby on another branch in a new db on its keeper", func() {
			outcd := etUpdateCluster(etForkedCluster(switchPoint))
			Ω(outcd.Cluster.Status.Master).To(Equal(etD1))
			Ω(outcd.DBs).NotTo(HaveKey(etD3), "forked db %s was not removed", etD3)
			Ω(outcd.DBs).To(HaveKey(etD2), "same branch db %s was removed", etD2)
			Ω(outcd.DBs).To(HaveKey(etD4), "no new db %s created", etD4)
			newDB := outcd.DBs[etD4]
			Ω(newDB.Spec.KeeperUID).To(Equal(etK3))
			Ω(newDB.Spec.InitMode).To(Equal(cluster.ResyncDB))
			Ω(newDB.Spec.FollowConfig).NotTo(BeNil(), "new db does not follow %s", etD1)
			Ω(newDB.Spec.FollowConfig.DBUID).To(Equal(etD1), "new db does not follow %s", etD1)
			Ω(outcd.DBs[etD1].Spec.Followers).To(Equal([]string{etD2, etD4}), "master followers")
		})
	})

	// An elected standby that fails before it promotes, while the old master
	// comes back healthy.
	Describe("when the elected standby fails before it promotes", func() {
		It("elects the old master again with async replication", func() {
			outcd := etUpdateCluster(etFailedElectionCluster(etAsyncSpec(), false))
			Ω(outcd.Cluster.Status.Master).To(Equal(etD1))
		})

		It("doesn't elect an old master further behind than MaxStandbyLag with async replication", func() {
			const maxLag = 10
			spec := etAsyncSpec()
			spec.MaxStandbyLag = util.ToPtr(uint32(maxLag))
			cd := etFailedElectionCluster(spec, false)
			cd.DBs[etD1].Status.XLogPos = cd.DBs[etD2].Status.XLogPos - maxLag - 1
			outcd := etUpdateCluster(cd)
			Ω(outcd.Cluster.Status.Master).To(Equal(etD2))
		})

		It("doesn't elect an old master on an older timeline with async replication", func() {
			cd := etFailedElectionCluster(etAsyncSpec(), false)
			cd.DBs[etD2].Status.TimelineID = etTL2
			outcd := etUpdateCluster(cd)
			Ω(outcd.Cluster.Status.Master).To(Equal(etD2))
		})

		It("doesn't elect the old master again with sync replication: the elected db stays master", func() {
			// db2 never was a master, so it has no reported (in sync)
			// synchronous standbys: there is nothing in common with its spec, so
			// nobody can be elected.
			outcd := etUpdateCluster(etFailedElectionCluster(etSyncSpec(), true))
			Ω(outcd.Cluster.Status.Master).To(Equal(etD2))
		})

		It("elects the old master with sync replication when the elected db has a synchronous standbys status",
			func() {
				// Not reachable in practice: after an election the old master's db
				// is removed and a new standby db (new uid) is created for its
				// keeper, so a db never carries the synchronous standbys it reported
				// in an earlier term as master. This pins what updateCluster does
				// with such a status.
				cd := etFailedElectionCluster(etSyncSpec(), true)
				cd.DBs[etD2].Status.SynchronousStandbys = []string{etD1}
				outcd := etUpdateCluster(cd)
				Ω(outcd.Cluster.Status.Master).To(Equal(etD1))
			})
	})
})
