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
	"reflect"
	"testing"
	"time"

	cluster "github.com/pgvillage-tools/orion/api/v1"

	"github.com/pgvillage-tools/orion/internal/timer"
)

// TestDeadKeeperWithDBNotRemoved is the negative of TestUpdateCluster #13: a
// keeper that has been dead for longer than DeadKeeperRemovalInterval is not
// removed while it still has a db assigned. A dead keeper without a db in the
// same cluster is removed.
func TestDeadKeeperWithDBNotRemoved(t *testing.T) {
	const (
		deadFor = 100 * time.Hour
		xlogPos = 1000
	)
	t.Run("master keeper dead", func(t *testing.T) {
		master := etMaster(etD1, etK1, xlogPos, etD2)
		master.Status.Healthy = false
		cd := etClusterData(etAsyncSpec(), etD1,
			[]*cluster.DB{master, etStandby(etD2, etK2, etD1, xlogPos)}, etK3)
		cd.Keepers[etK1].Status.Healthy = false
		cd.Keepers[etK1].Status.LastHealthyTime = now.Add(-deadFor)
		cd.Keepers[etK3].Status.Healthy = false
		cd.Keepers[etK3].Status.LastHealthyTime = now.Add(-deadFor)
		outcd := etUpdateCluster(t, cd)
		if _, ok := outcd.Keepers[etK1]; !ok {
			t.Errorf("dead keeper %s with db %s was removed", etK1, etD1)
		}
		if _, ok := outcd.Keepers[etK3]; ok {
			t.Errorf("dead keeper %s without db was not removed", etK3)
		}
		if got := outcd.Cluster.Status.Master; got != etD2 {
			t.Errorf("got master %q, want %q", got, etD2)
		}
	})
	t.Run("standby keeper dead", func(t *testing.T) {
		standby := etStandby(etD2, etK2, etD1, xlogPos)
		standby.Status.Healthy = false
		cd := etClusterData(etAsyncSpec(), etD1, []*cluster.DB{etMaster(etD1, etK1, xlogPos, etD2), standby})
		cd.Keepers[etK2].Status.Healthy = false
		cd.Keepers[etK2].Status.LastHealthyTime = now.Add(-deadFor)
		outcd := etUpdateCluster(t, cd)
		if _, ok := outcd.Keepers[etK2]; !ok {
			t.Errorf("dead keeper %s with db %s was removed", etK2, etD2)
		}
		if _, ok := outcd.DBs[etD2]; !ok {
			t.Errorf("db %s of the dead keeper was removed", etD2)
		}
	})
}

// etTimerCD returns a cluster data used by the health helpers to read
// FailInterval.
func etTimerCD(failInterval time.Duration) *cluster.Data {
	spec := etAsyncSpec()
	spec.FailInterval = &cluster.Duration{Duration: failInterval}
	return etClusterData(spec, etD1, nil)
}

// TestKeeperErrorTimer checks isKeeperHealthy with SetKeeperError and
// CleanKeeperError. Time is controlled by writing the timer map directly.
func TestKeeperErrorTimer(t *testing.T) {
	const failInterval = time.Minute
	cd := etTimerCD(failInterval)
	s := etSentinel(cd)
	k := etKeeper(etK1)

	if !s.isKeeperHealthy(cd, k) {
		t.Error("keeper without error timer: want healthy")
	}
	s.SetKeeperError(etK1)
	if !s.isKeeperHealthy(cd, k) {
		t.Error("keeper just in error: want healthy until failInterval passed")
	}
	// SetKeeperError does not restart a running timer
	started := timer.Now() - int64(2*failInterval)
	s.keeperErrorTimers[etK1] = started
	s.SetKeeperError(etK1)
	if s.keeperErrorTimers[etK1] != started {
		t.Error("SetKeeperError restarted a running timer")
	}
	if s.isKeeperHealthy(cd, k) {
		t.Error("keeper in error for longer than failInterval: want unhealthy")
	}
	s.CleanKeeperError(etK1)
	if _, ok := s.keeperErrorTimers[etK1]; ok {
		t.Error("CleanKeeperError did not remove the timer")
	}
	if !s.isKeeperHealthy(cd, k) {
		t.Error("keeper after CleanKeeperError: want healthy")
	}
}

// TestDBErrorTimer checks isDBHealthy with SetDBError and CleanDBError.
func TestDBErrorTimer(t *testing.T) {
	const failInterval = time.Minute
	cd := etTimerCD(failInterval)
	s := etSentinel(cd)
	db := &cluster.DB{UID: etD1}

	if !s.isDBHealthy(cd, db) {
		t.Error("db without error timer: want healthy")
	}
	s.SetDBError(etD1)
	if !s.isDBHealthy(cd, db) {
		t.Error("db just in error: want healthy until failInterval passed")
	}
	started := timer.Now() - int64(2*failInterval)
	s.dbErrorTimers[etD1] = started
	s.SetDBError(etD1)
	if s.dbErrorTimers[etD1] != started {
		t.Error("SetDBError restarted a running timer")
	}
	if s.isDBHealthy(cd, db) {
		t.Error("db in error for longer than failInterval: want unhealthy")
	}
	s.CleanDBError(etD1)
	if !s.isDBHealthy(cd, db) {
		t.Error("db after CleanDBError: want healthy")
	}
}

// TestDBNotIncreasingXLogPos checks isDBIncreasingXLogPos: the db counts as
// increasing until SetDBNotIncreasingXLogPos was called more than
// DefaultDBNotIncreasingXLogPosTimes times (it is a counter of checks, not a
// timer), and Clean resets it.
func TestDBNotIncreasingXLogPos(t *testing.T) {
	s := etSentinel(etTimerCD(time.Minute))
	db := &cluster.DB{UID: etD1}
	if !s.isDBIncreasingXLogPos(db) {
		t.Error("db without counter: want increasing")
	}
	for range cluster.DefaultDBNotIncreasingXLogPosTimes {
		s.SetDBNotIncreasingXLogPos(etD1)
	}
	if !s.isDBIncreasingXLogPos(db) {
		t.Errorf("db not increasing %d times: want still increasing", cluster.DefaultDBNotIncreasingXLogPosTimes)
	}
	s.SetDBNotIncreasingXLogPos(etD1)
	if s.isDBIncreasingXLogPos(db) {
		t.Errorf("db not increasing %d times: want not increasing", cluster.DefaultDBNotIncreasingXLogPosTimes+1)
	}
	s.CleanDBNotIncreasingXLogPos(etD1)
	if !s.isDBIncreasingXLogPos(db) {
		t.Error("db after Clean: want increasing")
	}
}

func etKeeperInfo(keeperUID, infoUID string, state *cluster.PostgresState) *cluster.KeeperInfo {
	return &cluster.KeeperInfo{
		InfoUID:       infoUID,
		UID:           keeperUID,
		ClusterUID:    etCluster,
		BootUUID:      "boot-" + keeperUID,
		PostgresState: state,
	}
}

func etHealthyState(dbUID string, xlogPos uint64) *cluster.PostgresState {
	return &cluster.PostgresState{
		UID:                 dbUID,
		Generation:          etGen1,
		Healthy:             true,
		TimelineID:          etTL1,
		XLogPos:             xlogPos,
		SynchronousStandbys: []string{},
	}
}

// TestUpdateKeepersStatus covers updateKeepersStatus over a few rounds: the
// first run only records history, an unchanged keeper info puts the keeper
// in error, ForceFail makes a keeper unhealthy for one check and is reset,
// keepers of another cluster are ignored, a new keeper is added, db states
// are copied, and a keeper in error for longer than FailInterval is
// unhealthy together with its db.
func TestUpdateKeepersStatus(t *testing.T) {
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

	// round 1, first run: history only, cd unchanged
	firstcd, kihs := s.updateKeepersStatus(ctx, cd, kis, true)
	if !reflect.DeepEqual(firstcd, cd) {
		t.Errorf("first run changed cd")
	}
	if len(kihs) != 2 || kihs[etK1].Seen || kihs[etK2].Seen {
		t.Errorf("first run: want 2 unseen histories, got %v", kihs)
	}
	s.keeperInfoHistories = kihs

	// round 2, same infos: not updated, so in error (but within failInterval)
	newcd, kihs := s.updateKeepersStatus(ctx, cd, kis, false)
	if _, ok := s.keeperErrorTimers[etK1]; !ok {
		t.Error("keeper1 with unchanged info: want error timer")
	}
	if _, ok := s.dbErrorTimers[etD1]; !ok {
		t.Error("db1 of keeper1 with unchanged info: want error timer")
	}
	if !newcd.Keepers[etK1].Status.Healthy {
		t.Error("keeper1 within failInterval: want healthy")
	}
	if newcd.Keepers[etK1].Status.LastHealthyTime.IsZero() {
		t.Error("zero LastHealthyTime: want set")
	}
	// ForceFail: unhealthy for this check, flag reset, its db unhealthy too
	if k := newcd.Keepers[etK2]; k.Status.Healthy || k.Status.ForceFail {
		t.Errorf("keeper2 with ForceFail: got healthy %t forceFail %t, want false/false",
			k.Status.Healthy, k.Status.ForceFail)
	}
	if newcd.DBs[etD2].Status.Healthy {
		t.Error("db2 on a ForceFail keeper: want unhealthy")
	}
	if _, ok := newcd.Keepers[etK3]; ok {
		t.Error("keeper3 of another cluster was added")
	}
	s.keeperInfoHistories = kihs

	// round 3, new infos: keepers updated and healthy again (ForceFail was
	// reset), db status copied, new keeper added
	kis = cluster.KeepersInfo{
		etK1: etKeeperInfo(etK1, "info2", etHealthyState(etD1, xlogPos+1)),
		etK2: etKeeperInfo(etK2, "info2", etHealthyState(etD2, xlogPos)),
		k4:   etKeeperInfo(k4, "info1", nil),
	}
	kis[etK2].PostgresState.Healthy = false
	before := time.Now()
	newcd2, kihs := s.updateKeepersStatus(ctx, newcd, kis, false)
	for _, uid := range []string{etK1, etK2} {
		if _, ok := s.keeperErrorTimers[uid]; ok {
			t.Errorf("%s with new info: want error timer cleaned", uid)
		}
		k := newcd2.Keepers[uid]
		if !k.Status.Healthy || k.Status.LastHealthyTime.Before(before) {
			t.Errorf("%s with new info: want healthy with updated LastHealthyTime, got %v", uid, k.Status)
		}
		if k.Status.BootUUID != "boot-"+uid {
			t.Errorf("%s: BootUUID not copied: %q", uid, k.Status.BootUUID)
		}
	}
	if _, ok := newcd2.Keepers[k4]; !ok {
		t.Error("new keeper4 not added")
	}
	if _, ok := s.dbErrorTimers[etD1]; ok {
		t.Error("db1 healthy: want error timer cleaned")
	}
	if got := newcd2.DBs[etD1].Status.XLogPos; got != xlogPos+1 {
		t.Errorf("db1 xlogpos: got %d, want %d", got, xlogPos+1)
	}
	if _, ok := s.dbNotIncreasingXLogPos[etD1]; ok {
		t.Error("db1 xlogpos increased: want counter cleaned")
	}
	if got := s.dbNotIncreasingXLogPos[etD2]; got != 1 {
		t.Errorf("db2 xlogpos unchanged: want counter 1, got %d", got)
	}
	if _, ok := s.dbErrorTimers[etD2]; !ok {
		t.Error("db2 reported unhealthy: want error timer")
	}
	s.keeperInfoHistories = kihs

	// round 4: keeper1 info seen but not updated for longer than
	// sleepInterval, in error for longer than failInterval: unhealthy, and
	// its db unhealthy too
	s.sleepInterval = time.Second
	s.keeperInfoHistories[etK1].Timer = timer.Now() - int64(staleFor)
	s.keeperErrorTimers[etK1] = timer.Now() - int64(staleFor)
	newcd3, _ := s.updateKeepersStatus(ctx, newcd2, kis, false)
	if newcd3.Keepers[etK1].Status.Healthy {
		t.Error("keeper1 stale for longer than failInterval: want unhealthy")
	}
	if newcd3.DBs[etD1].Status.Healthy {
		t.Error("db1 on an unhealthy keeper: want unhealthy")
	}

	// round 5: keeper2 reports the state of an unexpected db uid
	kis[etK2] = etKeeperInfo(etK2, "info3", etHealthyState(etD3, xlogPos))
	delete(s.dbErrorTimers, etD2)
	_, _ = s.updateKeepersStatus(ctx, newcd2, kis, false)
	if _, ok := s.dbErrorTimers[etD2]; !ok {
		t.Error("keeper2 reporting another db uid: want error timer for db2")
	}
}

// TestUpdateDBConvergenceInfos checks that a convergence timer is started for
// a db that has not converged, kept while the generation stays the same,
// restarted on a new generation and removed once converged.
func TestUpdateDBConvergenceInfos(t *testing.T) {
	db := etMaster(etD1, etK1, 0)
	cd := etClusterData(etAsyncSpec(), etD1, []*cluster.DB{db})
	s := etSentinel(cd)
	s.dbConvergenceInfos = map[string]*DBConvergenceInfo{}

	s.updateDBConvergenceInfos(cd)
	if _, ok := s.dbConvergenceInfos[etD1]; ok {
		t.Error("converged db: want no convergence info")
	}

	db.Generation = etGen2
	s.updateDBConvergenceInfos(cd)
	ci, ok := s.dbConvergenceInfos[etD1]
	if !ok || ci.Generation != etGen2 {
		t.Fatalf("not converged db: want convergence info for generation %d, got %v", etGen2, ci)
	}
	const old = int64(-1)
	ci.Timer = old
	s.updateDBConvergenceInfos(cd)
	if s.dbConvergenceInfos[etD1].Timer != old {
		t.Error("same generation: timer was restarted")
	}
	db.Generation = etGen2 + 1
	s.updateDBConvergenceInfos(cd)
	if ci = s.dbConvergenceInfos[etD1]; ci.Timer == old || ci.Generation != etGen2+1 {
		t.Errorf("new generation: want restarted timer, got %v", ci)
	}

	db.Status.CurrentGeneration = db.Generation
	s.updateDBConvergenceInfos(cd)
	if _, ok := s.dbConvergenceInfos[etD1]; ok {
		t.Error("db converged: want convergence info removed")
	}
}

// TestUpdateChangeTimes checks that only changed or new objects get a new
// ChangeTime.
func TestUpdateChangeTimes(t *testing.T) {
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

	if newcd.ChangeTime.IsZero() {
		t.Error("cluster data ChangeTime not set")
	}
	if !newcd.Cluster.ChangeTime.IsZero() {
		t.Error("unchanged cluster got a ChangeTime")
	}
	if !newcd.Proxy.ChangeTime.IsZero() {
		t.Error("unchanged proxy got a ChangeTime")
	}
	for uid, changed := range map[string]bool{etD1: true, etD2: false, etD3: true} {
		if got := !newcd.DBs[uid].ChangeTime.IsZero(); got != changed {
			t.Errorf("db %s: ChangeTime set %t, want %t", uid, got, changed)
		}
	}
	for uid, changed := range map[string]bool{etK1: false, etK2: true, etK3: true} {
		if got := !newcd.Keepers[uid].ChangeTime.IsZero(); got != changed {
			t.Errorf("keeper %s: ChangeTime set %t, want %t", uid, got, changed)
		}
	}

	newcd2 := newcd.DeepCopy()
	newcd2.Cluster.Status.Master = etD2
	newcd2.Proxy.Spec.MasterDBUID = ""
	s.updateChangeTimes(newcd, newcd2)
	if newcd2.Cluster.ChangeTime.IsZero() || newcd2.Proxy.ChangeTime.IsZero() {
		t.Error("changed cluster and proxy: want ChangeTime set")
	}
}
