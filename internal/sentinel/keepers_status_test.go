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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/timer"
)

var _ = Describe("the status of keepers and dbs", func() {
	// etTimerCD returns a cluster data used by the health helpers to read
	// FailInterval.
	etTimerCD := func(failInterval time.Duration) *cluster.Data {
		spec := etAsyncSpec()
		spec.FailInterval = &cluster.Duration{Duration: failInterval}
		return etClusterData(spec, etD1, nil)
	}

	etKeeperInfo := func(keeperUID, infoUID string, state *cluster.PostgresState) *cluster.KeeperInfo {
		return &cluster.KeeperInfo{
			InfoUID:       infoUID,
			UID:           keeperUID,
			ClusterUID:    etCluster,
			BootUUID:      "boot-" + keeperUID,
			PostgresState: state,
		}
	}

	etHealthyState := func(dbUID string, xlogPos uint64) *cluster.PostgresState {
		return &cluster.PostgresState{
			UID:                 dbUID,
			Generation:          etGen1,
			Healthy:             true,
			TimelineID:          etTL1,
			XLogPos:             xlogPos,
			SynchronousStandbys: []string{},
		}
	}

	// The negative of updateCluster case #13: a keeper that has been dead for
	// longer than DeadKeeperRemovalInterval is not removed while it still has a
	// db assigned. A dead keeper without a db in the same cluster is removed.
	Describe("a keeper dead for longer than DeadKeeperRemovalInterval", func() {
		const (
			deadFor = 100 * time.Hour
			xlogPos = 1000
		)

		It("is not removed with a master db, while a dead keeper without a db is", func() {
			master := etMaster(etD1, etK1, xlogPos, etD2)
			master.Status.Healthy = false
			cd := etClusterData(etAsyncSpec(), etD1,
				[]*cluster.DB{master, etStandby(etD2, etK2, etD1, xlogPos)}, etK3)
			cd.Keepers[etK1].Status.Healthy = false
			cd.Keepers[etK1].Status.LastHealthyTime = now.Add(-deadFor)
			cd.Keepers[etK3].Status.Healthy = false
			cd.Keepers[etK3].Status.LastHealthyTime = now.Add(-deadFor)
			outcd := etUpdateCluster(cd)
			Ω(outcd.Keepers).To(HaveKey(etK1), "dead keeper %s with db %s was removed", etK1, etD1)
			Ω(outcd.Keepers).NotTo(HaveKey(etK3), "dead keeper %s without db was not removed", etK3)
			Ω(outcd.Cluster.Status.Master).To(Equal(etD2))
		})

		It("is not removed with a standby db, and neither is its db", func() {
			standby := etStandby(etD2, etK2, etD1, xlogPos)
			standby.Status.Healthy = false
			cd := etClusterData(etAsyncSpec(), etD1, []*cluster.DB{etMaster(etD1, etK1, xlogPos, etD2), standby})
			cd.Keepers[etK2].Status.Healthy = false
			cd.Keepers[etK2].Status.LastHealthyTime = now.Add(-deadFor)
			outcd := etUpdateCluster(cd)
			Ω(outcd.Keepers).To(HaveKey(etK2), "dead keeper %s with db %s was removed", etK2, etD2)
			Ω(outcd.DBs).To(HaveKey(etD2), "db %s of the dead keeper was removed", etD2)
		})
	})

	// isKeeperHealthy with SetKeeperError and CleanKeeperError. Time is
	// controlled by writing the timer map directly.
	It("counts a keeper as unhealthy once it is in error for longer than FailInterval", func() {
		const failInterval = time.Minute
		cd := etTimerCD(failInterval)
		s := etSentinel(cd)
		k := etKeeper(etK1)

		Ω(s.isKeeperHealthy(cd, k)).To(BeTrue(), "keeper without error timer: want healthy")
		s.SetKeeperError(etK1)
		Ω(s.isKeeperHealthy(cd, k)).To(BeTrue(), "keeper just in error: want healthy until failInterval passed")
		// SetKeeperError does not restart a running timer
		started := timer.Now() - int64(2*failInterval)
		s.keeperErrorTimers[etK1] = started
		s.SetKeeperError(etK1)
		Ω(s.keeperErrorTimers[etK1]).To(Equal(started), "SetKeeperError restarted a running timer")
		Ω(s.isKeeperHealthy(cd, k)).To(BeFalse(), "keeper in error for longer than failInterval: want unhealthy")
		s.CleanKeeperError(etK1)
		Ω(s.keeperErrorTimers).NotTo(HaveKey(etK1), "CleanKeeperError did not remove the timer")
		Ω(s.isKeeperHealthy(cd, k)).To(BeTrue(), "keeper after CleanKeeperError: want healthy")
	})

	// isDBHealthy with SetDBError and CleanDBError.
	It("counts a db as unhealthy once it is in error for longer than FailInterval", func() {
		const failInterval = time.Minute
		cd := etTimerCD(failInterval)
		s := etSentinel(cd)
		db := &cluster.DB{UID: etD1}

		Ω(s.isDBHealthy(cd, db)).To(BeTrue(), "db without error timer: want healthy")
		s.SetDBError(etD1)
		Ω(s.isDBHealthy(cd, db)).To(BeTrue(), "db just in error: want healthy until failInterval passed")
		started := timer.Now() - int64(2*failInterval)
		s.dbErrorTimers[etD1] = started
		s.SetDBError(etD1)
		Ω(s.dbErrorTimers[etD1]).To(Equal(started), "SetDBError restarted a running timer")
		Ω(s.isDBHealthy(cd, db)).To(BeFalse(), "db in error for longer than failInterval: want unhealthy")
		s.CleanDBError(etD1)
		Ω(s.isDBHealthy(cd, db)).To(BeTrue(), "db after CleanDBError: want healthy")
	})

	// isDBIncreasingXLogPos: the db counts as increasing until
	// SetDBNotIncreasingXLogPos was called more than
	// DefaultDBNotIncreasingXLogPosTimes times (it is a counter of checks, not a
	// timer), and Clean resets it.
	It("counts a db as not increasing its xlogpos after more than DefaultDBNotIncreasingXLogPosTimes checks", func() {
		s := etSentinel(etTimerCD(time.Minute))
		db := &cluster.DB{UID: etD1}
		Ω(s.isDBIncreasingXLogPos(db)).To(BeTrue(), "db without counter: want increasing")
		for range cluster.DefaultDBNotIncreasingXLogPosTimes {
			s.SetDBNotIncreasingXLogPos(etD1)
		}
		Ω(s.isDBIncreasingXLogPos(db)).To(BeTrue(), "db not increasing %d times: want still increasing",
			cluster.DefaultDBNotIncreasingXLogPosTimes)
		s.SetDBNotIncreasingXLogPos(etD1)
		Ω(s.isDBIncreasingXLogPos(db)).To(BeFalse(), "db not increasing %d times: want not increasing",
			cluster.DefaultDBNotIncreasingXLogPosTimes+1)
		s.CleanDBNotIncreasingXLogPos(etD1)
		Ω(s.isDBIncreasingXLogPos(db)).To(BeTrue(), "db after Clean: want increasing")
	})

	// updateKeepersStatus over a few rounds: the first run only records
	// history, an unchanged keeper info puts the keeper in error, ForceFail
	// makes a keeper unhealthy for one check and is reset, keepers of another
	// cluster are ignored, a new keeper is added, db states are copied, and a
	// keeper in error for longer than FailInterval is unhealthy together with
	// its db.
	It("updates the status of the keepers and their dbs from the keepers info over several checks", func() {
		const (
			xlogPos  = 5000
			staleFor = time.Hour
			k4       = "keeper4"
		)
		ctx := context.Background()
		spec := etAsyncSpec()
		spec.FailInterval = &cluster.Duration{Duration: time.Minute}
		cd := etClusterData(spec, etD1, []*cluster.DB{
			etMaster(etD1, etK1, xlogPos, etD2),
			etStandby(etD2, etK2, etD1, xlogPos),
		})
		cd.Keepers[etK1].Status.LastHealthyTime = time.Time{}
		cd.Keepers[etK2].Status.ForceFail = true
		s := etSentinel(cd)

		kis := cluster.KeepersInfo{
			etK1: etKeeperInfo(etK1, "info1", etHealthyState(etD1, xlogPos+1)),
			etK2: etKeeperInfo(etK2, "info1", etHealthyState(etD2, xlogPos)),
			etK3: etKeeperInfo(etK3, "info1", nil),
		}
		kis[etK3].ClusterUID = "othercluster"

		By("round 1, first run: history only, cd unchanged")
		firstcd, kihs := s.updateKeepersStatus(ctx, cd, kis, true)
		Ω(firstcd).To(Equal(cd), "first run changed cd")
		Ω(kihs).To(HaveLen(2), "first run: want 2 unseen histories")
		Ω(kihs[etK1].Seen).To(BeFalse(), "first run: want 2 unseen histories")
		Ω(kihs[etK2].Seen).To(BeFalse(), "first run: want 2 unseen histories")
		s.keeperInfoHistories = kihs

		By("round 2, same infos: not updated, so in error (but within failInterval)")
		newcd, kihs := s.updateKeepersStatus(ctx, cd, kis, false)
		Ω(s.keeperErrorTimers).To(HaveKey(etK1), "keeper1 with unchanged info: want error timer")
		Ω(s.dbErrorTimers).To(HaveKey(etD1), "db1 of keeper1 with unchanged info: want error timer")
		Ω(newcd.Keepers[etK1].Status.Healthy).To(BeTrue(), "keeper1 within failInterval: want healthy")
		Ω(newcd.Keepers[etK1].Status.LastHealthyTime.IsZero()).To(BeFalse(), "zero LastHealthyTime: want set")
		// ForceFail: unhealthy for this check, flag reset, its db unhealthy too
		Ω(newcd.Keepers[etK2].Status.Healthy).To(BeFalse(), "keeper2 with ForceFail: want unhealthy")
		Ω(newcd.Keepers[etK2].Status.ForceFail).To(BeFalse(), "keeper2 with ForceFail: want the flag reset")
		Ω(newcd.DBs[etD2].Status.Healthy).To(BeFalse(), "db2 on a ForceFail keeper: want unhealthy")
		Ω(newcd.Keepers).NotTo(HaveKey(etK3), "keeper3 of another cluster was added")
		s.keeperInfoHistories = kihs

		By("round 3, new infos: keepers updated and healthy again (ForceFail was reset), db status copied, " +
			"new keeper added")
		kis = cluster.KeepersInfo{
			etK1: etKeeperInfo(etK1, "info2", etHealthyState(etD1, xlogPos+1)),
			etK2: etKeeperInfo(etK2, "info2", etHealthyState(etD2, xlogPos)),
			k4:   etKeeperInfo(k4, "info1", nil),
		}
		kis[etK2].PostgresState.Healthy = false
		before := time.Now()
		newcd2, kihs := s.updateKeepersStatus(ctx, newcd, kis, false)
		for _, uid := range []string{etK1, etK2} {
			Ω(s.keeperErrorTimers).NotTo(HaveKey(uid), "%s with new info: want error timer cleaned", uid)
			k := newcd2.Keepers[uid]
			Ω(k.Status.Healthy).To(BeTrue(), "%s with new info: want healthy", uid)
			Ω(k.Status.LastHealthyTime).NotTo(BeTemporally("<", before),
				"%s with new info: want an updated LastHealthyTime", uid)
			Ω(k.Status.BootUUID).To(Equal("boot-"+uid), "%s: BootUUID not copied", uid)
		}
		Ω(newcd2.Keepers).To(HaveKey(k4), "new keeper4 not added")
		Ω(s.dbErrorTimers).NotTo(HaveKey(etD1), "db1 healthy: want error timer cleaned")
		Ω(newcd2.DBs[etD1].Status.XLogPos).To(BeEquivalentTo(xlogPos+1), "db1 xlogpos")
		Ω(s.dbNotIncreasingXLogPos).NotTo(HaveKey(etD1), "db1 xlogpos increased: want counter cleaned")
		Ω(s.dbNotIncreasingXLogPos[etD2]).To(BeEquivalentTo(1), "db2 xlogpos unchanged: want counter 1")
		Ω(s.dbErrorTimers).To(HaveKey(etD2), "db2 reported unhealthy: want error timer")
		s.keeperInfoHistories = kihs

		By("round 4: keeper1 info seen but not updated for longer than sleepInterval, in error for longer than " +
			"failInterval: unhealthy, and its db unhealthy too")
		s.sleepInterval = time.Second
		s.keeperInfoHistories[etK1].Timer = timer.Now() - int64(staleFor)
		s.keeperErrorTimers[etK1] = timer.Now() - int64(staleFor)
		newcd3, _ := s.updateKeepersStatus(ctx, newcd2, kis, false)
		Ω(newcd3.Keepers[etK1].Status.Healthy).To(BeFalse(),
			"keeper1 stale for longer than failInterval: want unhealthy")
		Ω(newcd3.DBs[etD1].Status.Healthy).To(BeFalse(), "db1 on an unhealthy keeper: want unhealthy")

		By("round 5: keeper2 reports the state of an unexpected db uid")
		kis[etK2] = etKeeperInfo(etK2, "info3", etHealthyState(etD3, xlogPos))
		delete(s.dbErrorTimers, etD2)
		_, _ = s.updateKeepersStatus(ctx, newcd2, kis, false)
		Ω(s.dbErrorTimers).To(HaveKey(etD2), "keeper2 reporting another db uid: want error timer for db2")
	})

	// A convergence timer is started for a db that has not converged, kept
	// while the generation stays the same, restarted on a new generation and
	// removed once converged.
	It("keeps a convergence timer for each db that hasn't converged to its generation", func() {
		db := etMaster(etD1, etK1, 0)
		cd := etClusterData(etAsyncSpec(), etD1, []*cluster.DB{db})
		s := etSentinel(cd)
		s.dbConvergenceInfos = map[string]*DBConvergenceInfo{}

		s.updateDBConvergenceInfos(cd)
		Ω(s.dbConvergenceInfos).NotTo(HaveKey(etD1), "converged db: want no convergence info")

		db.Generation = etGen2
		s.updateDBConvergenceInfos(cd)
		Ω(s.dbConvergenceInfos).To(HaveKey(etD1), "not converged db: want convergence info")
		ci := s.dbConvergenceInfos[etD1]
		Ω(ci.Generation).To(BeEquivalentTo(etGen2), "not converged db: want convergence info for generation %d",
			etGen2)
		const old = int64(-1)
		ci.Timer = old
		s.updateDBConvergenceInfos(cd)
		Ω(s.dbConvergenceInfos[etD1].Timer).To(Equal(old), "same generation: timer was restarted")
		db.Generation = etGen2 + 1
		s.updateDBConvergenceInfos(cd)
		ci = s.dbConvergenceInfos[etD1]
		Ω(ci.Timer).NotTo(Equal(old), "new generation: want restarted timer")
		Ω(ci.Generation).To(BeEquivalentTo(etGen2+1), "new generation: want the new generation")

		db.Status.CurrentGeneration = db.Generation
		s.updateDBConvergenceInfos(cd)
		Ω(s.dbConvergenceInfos).NotTo(HaveKey(etD1), "db converged: want convergence info removed")
	})

	// Only changed or new objects get a new ChangeTime.
	It("sets a new ChangeTime on changed or new objects only", func() {
		cd := etClusterData(etAsyncSpec(), etD1, []*cluster.DB{
			etMaster(etD1, etK1, 0, etD2),
			etStandby(etD2, etK2, etD1, 0),
		})
		newcd := cd.DeepCopy()
		newcd.DBs[etD1].Spec.Followers = []string{}
		newcd.DBs[etD3] = etStandby(etD3, etK3, etD1, 0)
		newcd.Keepers[etK3] = etKeeper(etK3)
		newcd.Keepers[etK2].Status.Healthy = false

		s := etSentinel(cd)
		s.updateChangeTimes(cd, newcd)

		Ω(newcd.ChangeTime.IsZero()).To(BeFalse(), "cluster data ChangeTime not set")
		Ω(newcd.Cluster.ChangeTime.IsZero()).To(BeTrue(), "unchanged cluster got a ChangeTime")
		Ω(newcd.Proxy.ChangeTime.IsZero()).To(BeTrue(), "unchanged proxy got a ChangeTime")
		for uid, changed := range map[string]bool{etD1: true, etD2: false, etD3: true} {
			Ω(!newcd.DBs[uid].ChangeTime.IsZero()).To(Equal(changed), "db %s: ChangeTime set", uid)
		}
		for uid, changed := range map[string]bool{etK1: false, etK2: true, etK3: true} {
			Ω(!newcd.Keepers[uid].ChangeTime.IsZero()).To(Equal(changed), "keeper %s: ChangeTime set", uid)
		}

		newcd2 := newcd.DeepCopy()
		newcd2.Cluster.Status.Master = etD2
		newcd2.Proxy.Spec.MasterDBUID = ""
		s.updateChangeTimes(newcd, newcd2)
		Ω(newcd2.Cluster.ChangeTime.IsZero()).To(BeFalse(), "changed cluster: want ChangeTime set")
		Ω(newcd2.Proxy.ChangeTime.IsZero()).To(BeFalse(), "changed proxy: want ChangeTime set")
	})
})
