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

	cluster "github.com/pgvillage-tools/orion/api/v1"

	"github.com/pgvillage-tools/orion/internal/util"
)

// TestElectionMaxStandbyLag checks that, with asynchronous replication, a
// standby that is further behind the failed master than MaxStandbyLag is never
// elected, and that one within the limit is.
func TestElectionMaxStandbyLag(t *testing.T) {
	const (
		maxLag        = 1000
		masterXLogPos = 100000
	)
	tests := []struct {
		name       string
		standbyLag map[string]uint64
		wantMaster string
	}{
		{
			name:       "lag over max: no new master, failed master stays",
			standbyLag: map[string]uint64{etD2: maxLag + 1},
			wantMaster: etD1,
		},
		{
			name:       "lag exactly max: elected",
			standbyLag: map[string]uint64{etD2: maxLag},
			wantMaster: etD2,
		},
		{
			name:       "lag under max: elected",
			standbyLag: map[string]uint64{etD2: 1},
			wantMaster: etD2,
		},
		{
			name:       "one standby over max, one under: only the one under max is elected",
			standbyLag: map[string]uint64{etD2: 10 * maxLag, etD3: maxLag / 2},
			wantMaster: etD3,
		},
	}
	keepers := map[string]string{etD2: etK2, etD3: etK3}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := etAsyncSpec()
			spec.MaxStandbyLag = util.ToPtr(uint32(maxLag))
			master := etMaster(etD1, etK1, masterXLogPos)
			master.Status.Healthy = false
			dbs := []*cluster.DB{master}
			for dbUID, lag := range tt.standbyLag {
				master.Spec.Followers = append(master.Spec.Followers, dbUID)
				dbs = append(dbs, etStandby(dbUID, keepers[dbUID], etD1, masterXLogPos-lag))
			}
			outcd := etUpdateCluster(t, etClusterData(spec, etD1, dbs))
			if got := outcd.Cluster.Status.Master; got != tt.wantMaster {
				t.Errorf("got master %q, want %q", got, tt.wantMaster)
			}
		})
	}
}

// TestIsLagBelowMax tests the lag check directly, including a standby that
// reports a higher xlogpos than the master (the uint64 difference wraps but is
// read as a negative int64, so it counts as below max).
func TestIsLagBelowMax(t *testing.T) {
	const (
		maxLag        = 1000
		masterXLogPos = 5000
	)
	spec := etAsyncSpec()
	spec.MaxStandbyLag = util.ToPtr(uint32(maxLag))
	cd := etClusterData(spec, etD1, nil)
	s := etSentinel(cd)
	master := &cluster.DB{UID: etD1, Status: cluster.DBStatus{XLogPos: masterXLogPos}}
	for _, tt := range []struct {
		xlogPos uint64
		want    bool
	}{
		{masterXLogPos, true},
		{masterXLogPos - maxLag, true},
		{masterXLogPos - maxLag - 1, false},
		{0, false},
		{masterXLogPos + maxLag, true},
	} {
		db := &cluster.DB{UID: etD2, Status: cluster.DBStatus{XLogPos: tt.xlogPos}}
		if got := s.isLagBelowMax(context.Background(), cd, master, db); got != tt.want {
			t.Errorf("xlogpos %d: got %t, want %t", tt.xlogPos, got, tt.want)
		}
	}
}

// TestFindBestNewMastersOrder documents the order in which candidates are
// returned. With async replication updateCluster elects bestNewMasters[0].
// The candidate with the highest XLogPos comes first, so the standby that is
// least behind is elected and the least data is lost (stolon elected the one
// furthest behind).
func TestFindBestNewMastersOrder(t *testing.T) {
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
	if got, want := etUIDs(best), []string{etD2, etD3}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	outcd := etUpdateCluster(t, cd)
	if got := outcd.Cluster.Status.Master; got != etD2 {
		t.Errorf("got elected master %q, want %q (the standby with the highest xlogpos)", got, etD2)
	}
}

// TestFindBestNewMastersCanBeMaster checks that a keeper started with
// --can-be-master=false is never a candidate.
func TestFindBestNewMastersCanBeMaster(t *testing.T) {
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
	if got, want := etUIDs(best), []string{etD3}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// etTLH returns a timelines history from pairs of timelineID, switchPoint.
func etTLH(entries ...uint64) cluster.PostgresTimelinesHistory {
	tlsh := cluster.PostgresTimelinesHistory{}
	for i := 0; i+1 < len(entries); i += 2 {
		tlsh = append(tlsh, &cluster.PostgresTimelineHistory{TimelineID: entries[i], SwitchPoint: entries[i+1]})
	}
	return tlsh
}

// TestIsDifferentTimelineBranch tests all branches of isDifferentTimelineBranch
// (followed is the master, db the standby).
func TestIsDifferentTimelineBranch(t *testing.T) {
	const switchPoint = 1000
	tests := []struct {
		name     string
		followed cluster.DBStatus
		db       cluster.DBStatus
		want     bool
	}{
		{
			name:     "standby on a higher timeline than the master",
			followed: cluster.DBStatus{TimelineID: etTL1},
			db:       cluster.DBStatus{TimelineID: etTL2},
			want:     true,
		},
		{
			name:     "both on timeline 1 (no history)",
			followed: cluster.DBStatus{TimelineID: etTL1},
			db:       cluster.DBStatus{TimelineID: etTL1},
			want:     false,
		},
		{
			name:     "same timeline, same switchpoint",
			followed: cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
			db:       cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
			want:     false,
		},
		{
			name:     "same timeline, forked at a different switchpoint",
			followed: cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
			db:       cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint+1)},
			want:     true,
		},
		{
			name:     "same timeline, standby has no history",
			followed: cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
			db:       cluster.DBStatus{TimelineID: etTL2},
			want:     false,
		},
		{
			name:     "standby on an older timeline, behind the master's switchpoint",
			followed: cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
			db:       cluster.DBStatus{TimelineID: etTL1, XLogPos: switchPoint - 1},
			want:     false,
		},
		{
			name:     "standby on an older timeline, exactly at the master's switchpoint",
			followed: cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
			db:       cluster.DBStatus{TimelineID: etTL1, XLogPos: switchPoint},
			want:     false,
		},
		{
			name:     "standby on an older timeline, past the master's switchpoint",
			followed: cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
			db:       cluster.DBStatus{TimelineID: etTL1, XLogPos: switchPoint + 1},
			want:     true,
		},
		{
			name:     "standby on an older timeline, master has no history for it",
			followed: cluster.DBStatus{TimelineID: etTL3, TimelinesHistory: etTLH(etTL2, switchPoint)},
			db:       cluster.DBStatus{TimelineID: etTL1, XLogPos: switchPoint + 1},
			want:     false,
		},
	}
	s := &Sentinel{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			followed := &cluster.DB{UID: etD1, Status: tt.followed}
			db := &cluster.DB{UID: etD2, Status: tt.db}
			if got := s.isDifferentTimelineBranch(context.Background(), followed, db); got != tt.want {
				t.Errorf("got %t, want %t", got, tt.want)
			}
		})
	}
}

// etForkedCluster returns a cluster where master db1 is on timeline 2 (forked
// from timeline 1 at switchPoint), db2 is on the same branch and db3 is on a
// timeline 2 that forked at a different point. db3 is further ahead than db2.
func etForkedCluster(switchPoint uint64) *cluster.Data {
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

// TestElectionTimelineFork checks that, when the master fails, a standby on a
// different timeline branch is not a candidate even though it is further
// ahead, and that a standby on the same branch is elected.
func TestElectionTimelineFork(t *testing.T) {
	const switchPoint = 10000
	cd := etForkedCluster(switchPoint)
	cd.DBs[etD1].Status.Healthy = false
	s := etSentinel(cd)
	if v := s.dbValidity(context.Background(), cd, etD3); v != dbValidityInvalid {
		t.Errorf("forked db3 validity: got %d, want invalid", v)
	}
	best := s.findBestNewMasters(context.Background(), cd, cd.DBs[etD1])
	if got, want := etUIDs(best), []string{etD2}; !reflect.DeepEqual(got, want) {
		t.Errorf("got candidates %v, want %v", got, want)
	}
	outcd := etUpdateCluster(t, cd)
	if got := outcd.Cluster.Status.Master; got != etD2 {
		t.Errorf("got master %q, want %q", got, etD2)
	}
}

// TestElectionOlderTimelineNotCandidate checks that a standby still on the
// previous timeline, behind the switchpoint (so on the same branch and able to
// follow), is valid but is not a candidate for master: findBestStandbys only
// takes standbys on the master's timeline.
func TestElectionOlderTimelineNotCandidate(t *testing.T) {
	const switchPoint = 10000
	cd := etForkedCluster(switchPoint)
	cd.DBs[etD1].Status.Healthy = false
	older := cd.DBs[etD3]
	older.Status.TimelineID = etTL1
	older.Status.TimelinesHistory = nil
	older.Status.XLogPos = switchPoint - 1
	s := etSentinel(cd)
	if v := s.dbValidity(context.Background(), cd, etD3); v != dbValidityValid {
		t.Errorf("db3 validity: got %d, want valid", v)
	}
	best := s.findBestNewMasters(context.Background(), cd, cd.DBs[etD1])
	if got, want := etUIDs(best), []string{etD2}; !reflect.DeepEqual(got, want) {
		t.Errorf("got candidates %v, want %v", got, want)
	}
}

// TestTimelineForkedStandbyResynced checks that, with a healthy master, a
// standby on a different timeline branch is removed and a new db is created on
// its keeper with InitMode resync.
func TestTimelineForkedStandbyResynced(t *testing.T) {
	const switchPoint = 10000
	outcd := etUpdateCluster(t, etForkedCluster(switchPoint))
	if got := outcd.Cluster.Status.Master; got != etD1 {
		t.Fatalf("got master %q, want %q", got, etD1)
	}
	if _, ok := outcd.DBs[etD3]; ok {
		t.Errorf("forked db %s was not removed", etD3)
	}
	if _, ok := outcd.DBs[etD2]; !ok {
		t.Errorf("same branch db %s was removed", etD2)
	}
	newDB, ok := outcd.DBs[etD4]
	if !ok {
		t.Fatalf("no new db %s created, dbs: %v", etD4, outcd.DBs)
	}
	if newDB.Spec.KeeperUID != etK3 || newDB.Spec.InitMode != cluster.ResyncDB {
		t.Errorf("new db: got keeper %q initMode %q, want keeper %q initMode %q",
			newDB.Spec.KeeperUID, newDB.Spec.InitMode, etK3, cluster.ResyncDB)
	}
	if newDB.Spec.FollowConfig == nil || newDB.Spec.FollowConfig.DBUID != etD1 {
		t.Errorf("new db does not follow %s: %v", etD1, newDB.Spec.FollowConfig)
	}
	if got, want := outcd.DBs[etD1].Spec.Followers, []string{etD2, etD4}; !reflect.DeepEqual(got, want) {
		t.Errorf("master followers: got %v, want %v", got, want)
	}
}

// etFailedElectionCluster returns the state right after db2 was elected to
// replace the failed master db1 (see TestUpdateCluster #5), where db2 then
// failed before it promoted (not healthy, not converged), and db1 is back
// healthy. db1 still has the master role (updateCluster keeps the old
// master's role on election) and has the most WAL.
func etFailedElectionCluster(spec *cluster.Spec, syncRepl bool) *cluster.Data {
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

// TestElectionElectedStandbyFailsBeforePromotion covers an elected standby
// that fails before it promotes, while the old master comes back healthy.
func TestElectionElectedStandbyFailsBeforePromotion(t *testing.T) {
	t.Run("async: old master elected again", func(t *testing.T) {
		outcd := etUpdateCluster(t, etFailedElectionCluster(etAsyncSpec(), false))
		if got := outcd.Cluster.Status.Master; got != etD1 {
			t.Errorf("got master %q, want %q", got, etD1)
		}
	})
	t.Run("async: old master further behind than MaxStandbyLag not elected", func(t *testing.T) {
		const maxLag = 10
		spec := etAsyncSpec()
		spec.MaxStandbyLag = util.ToPtr(uint32(maxLag))
		cd := etFailedElectionCluster(spec, false)
		cd.DBs[etD1].Status.XLogPos = cd.DBs[etD2].Status.XLogPos - maxLag - 1
		outcd := etUpdateCluster(t, cd)
		if got := outcd.Cluster.Status.Master; got != etD2 {
			t.Errorf("got master %q, want %q", got, etD2)
		}
	})
	t.Run("async: old master on an older timeline not elected", func(t *testing.T) {
		cd := etFailedElectionCluster(etAsyncSpec(), false)
		cd.DBs[etD2].Status.TimelineID = etTL2
		outcd := etUpdateCluster(t, cd)
		if got := outcd.Cluster.Status.Master; got != etD2 {
			t.Errorf("got master %q, want %q", got, etD2)
		}
	})
	t.Run("sync: old master not elected again, elected db stays master", func(t *testing.T) {
		// db2 never was a master, so it has no reported (in sync)
		// synchronous standbys: there is nothing in common with its spec, so
		// nobody can be elected.
		outcd := etUpdateCluster(t, etFailedElectionCluster(etSyncSpec(), true))
		if got := outcd.Cluster.Status.Master; got != etD2 {
			t.Errorf("got master %q, want %q", got, etD2)
		}
	})
	t.Run("sync: elected db with a synchronous standbys status elects the old master", func(t *testing.T) {
		// Not reachable in practice: after an election the old master's db
		// is removed and a new standby db (new uid) is created for its
		// keeper, so a db never carries the synchronous standbys it reported
		// in an earlier term as master. This pins what updateCluster does
		// with such a status.
		cd := etFailedElectionCluster(etSyncSpec(), true)
		cd.DBs[etD2].Status.SynchronousStandbys = []string{etD1}
		outcd := etUpdateCluster(t, cd)
		if got := outcd.Cluster.Status.Master; got != etD1 {
			t.Errorf("got master %q, want %q", got, etD1)
		}
	})
}
