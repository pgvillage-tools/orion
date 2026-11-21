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
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/util"
)

// Parameters, keepers, dbs and generations of the updateCluster table cases
const (
	p1 = "param01"
	v1 = "value01"
	p2 = "param02"
	v2 = "value02"

	k1 = "keeper1"
	k2 = "keeper2"
	k3 = "keeper3"

	d1 = "db1"
	d2 = "db2"
	d3 = "db3"

	g2 = 2
	g3 = 3
)

// Fixtures for the focused election tests. They build a cluster in the
// Normal phase with db1 (on keeper1) as master and a configurable set of
// standbys, all healthy and converged, so each test only has to change the
// fields it is about.
const (
	etCluster = "cluster1"
	etK1      = "keeper1"
	etK2      = "keeper2"
	etK3      = "keeper3"
	etD1      = "db1"
	etD2      = "db2"
	etD3      = "db3"
	etD4      = "db4"
	etGen1    = 1
	etGen2    = 2
	etTL1     = 1
	etTL2     = 2
	etTL3     = 3
)

var curUID int

var now = time.Now()

func testUIDFn() string {
	curUID++
	return fmt.Sprintf("%s%d", "db", curUID)
}

func testRandFn(_ int) int {
	return 0
}

func testEqualCD(cd1, cd2 *cluster.Data) bool {
	// ignore times
	for _, cd := range []*cluster.Data{cd1, cd2} {
		cd.Cluster.ChangeTime = time.Time{}
		for _, k := range cd.Keepers {
			k.ChangeTime = time.Time{}
		}
		for _, db := range cd.DBs {
			db.ChangeTime = time.Time{}
		}
		cd.Proxy.ChangeTime = time.Time{}
	}
	return reflect.DeepEqual(cd1, cd2)
}

func etKeeper(uid string) *cluster.Keeper {
	return &cluster.Keeper{
		UID:    uid,
		Spec:   &cluster.KeeperSpec{},
		Status: cluster.KeeperStatus{Healthy: true, LastHealthyTime: now},
	}
}

func etMaster(uid, keeperUID string, xlogPos uint64, followers ...string) *cluster.DB {
	if followers == nil {
		followers = []string{}
	}
	return &cluster.DB{
		UID:        uid,
		Generation: etGen1,
		Spec: &cluster.DBSpec{
			KeeperUID: keeperUID,
			Role:      common.RolePrimary,
			Followers: followers,
		},
		Status: cluster.DBStatus{
			Healthy:           true,
			CurrentGeneration: etGen1,
			TimelineID:        etTL1,
			XLogPos:           xlogPos,
		},
	}
}

// etStandby returns a healthy standby db of masterUID on keeperUID at xlogPos. Its generation (etGen1) is
// cluster.InitialGeneration, which dbCanSync (and so removeUnsyncableDBs) treats as a db that is still initializing:
// set a later Generation to test a standby that can't sync.
func etStandby(uid, keeperUID, masterUID string, xlogPos uint64) *cluster.DB {
	return &cluster.DB{
		UID:        uid,
		Generation: etGen1,
		Spec: &cluster.DBSpec{
			KeeperUID: keeperUID,
			Role:      common.RoleReplica,
			Followers: []string{},
			FollowConfig: &cluster.FollowConfig{
				Type:  cluster.FollowTypeInternal,
				DBUID: masterUID,
			},
		},
		Status: cluster.DBStatus{
			Healthy:           true,
			CurrentGeneration: etGen1,
			TimelineID:        etTL1,
			XLogPos:           xlogPos,
		},
	}
}

// etClusterData returns a Normal phase cluster with the given master and dbs.
// A keeper is created for every db. Extra keepers without a db can be passed in
// extraKeepers.
func etClusterData(spec *cluster.Spec, masterUID string, dbs []*cluster.DB, extraKeepers ...string) *cluster.Data {
	cd := &cluster.Data{
		Cluster: &cluster.Cluster{
			UID:        etCluster,
			Generation: etGen1,
			Spec:       spec,
			Status: cluster.ClusterStatus{
				CurrentGeneration: etGen1,
				Phase:             cluster.Normal,
				Master:            masterUID,
			},
		},
		Keepers: cluster.Keepers{},
		DBs:     cluster.DBs{},
		Proxy: &cluster.Proxy{
			Generation: etGen1,
			Spec:       cluster.ProxySpec{MasterDBUID: masterUID, EnabledProxies: []string{}},
		},
	}
	for _, db := range dbs {
		cd.DBs[db.UID] = db
		cd.Keepers[db.Spec.KeeperUID] = etKeeper(db.Spec.KeeperUID)
	}
	for _, k := range extraKeepers {
		cd.Keepers[k] = etKeeper(k)
	}
	return cd
}

func etAsyncSpec() *cluster.Spec {
	return &cluster.Spec{
		ConvergenceTimeout: &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
		InitTimeout:        &cluster.Duration{Duration: cluster.DefaultInitTimeout},
		SyncTimeout:        &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
	}
}

func etSyncSpec() *cluster.Spec {
	spec := etAsyncSpec()
	spec.SynchronousReplication = util.ToPtr(true)
	return spec
}

// etTableSpec returns the cluster spec of the updateCluster table cases: etAsyncSpec with the default
// MaxStandbysPerSender
func etTableSpec() *cluster.Spec {
	spec := etAsyncSpec()
	spec.MaxStandbysPerSender = util.ToPtr(cluster.DefaultMaxStandbysPerSender)
	return spec
}

// etTableSyncSpec returns etTableSpec with synchronous replication
func etTableSyncSpec() *cluster.Spec {
	spec := etTableSpec()
	spec.SynchronousReplication = util.ToPtr(true)
	return spec
}

// etOverridingSpec returns etTableSpec overriding the db spec defaults: doubled request timeout, max standbys and
// additional wal senders, synchronous replication, pg_rewind and the parameters p1 and p2
func etOverridingSpec() *cluster.Spec {
	spec := etTableSyncSpec()
	spec.RequestTimeout = &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2}
	spec.MaxStandbys = util.ToPtr(cluster.DefaultMaxStandbys * 2)
	spec.AdditionalWalSenders = util.ToPtr(uint16(cluster.DefaultAdditionalWalSenders) * 2)
	spec.UsePgrewind = util.ToPtr(true)
	spec.PGParameters = cluster.PGParameters{p1: v1, p2: v2}
	return spec
}

// etTableDB gives db (of etMaster or etStandby) the db spec defaults of etTableSpec and no timeline, and returns it
func etTableDB(db *cluster.DB) *cluster.DB {
	db.Spec.RequestTimeout = cluster.Duration{Duration: cluster.DefaultRequestTimeout}
	db.Spec.MaxStandbys = cluster.DefaultMaxStandbys
	db.Spec.AdditionalWalSenders = cluster.DefaultAdditionalWalSenders
	db.Status.TimelineID = 0
	return db
}

// etOverridden gives db the db spec values of etOverridingSpec but synchronous replication, and returns it
func etOverridden(db *cluster.DB) *cluster.DB {
	db.Spec.RequestTimeout = cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2}
	db.Spec.MaxStandbys = cluster.DefaultMaxStandbys * 2
	db.Spec.AdditionalWalSenders = cluster.DefaultAdditionalWalSenders * 2
	db.Spec.UsePgrewind = true
	db.Spec.PGParameters = cluster.PGParameters{p1: v1, p2: v2}
	return db
}

// etTableMaster returns a healthy master of the updateCluster table cases, converged to generation 1
func etTableMaster(uid, keeperUID string, followers ...string) *cluster.DB {
	return etTableDB(etMaster(uid, keeperUID, 0, followers...))
}

// etTableStandby returns a healthy standby of the updateCluster table cases, converged to generation 1
func etTableStandby(uid, keeperUID, masterUID string) *cluster.DB {
	return etTableDB(etStandby(uid, keeperUID, masterUID, 0))
}

// etTableCluster returns a Normal phase cluster with spec, master db1 on keeper1 and its standbys (dbN on keeperN),
// of etTableMaster and etTableStandby
func etTableCluster(spec *cluster.Spec, standbys ...string) *cluster.Data {
	dbs := []*cluster.DB{etTableMaster(d1, k1, standbys...)}
	for _, uid := range standbys {
		dbs = append(dbs, etTableStandby(uid, "keeper"+strings.TrimPrefix(uid, "db"), d1))
	}
	return etClusterData(spec, d1, dbs)
}

// etInitModeNone sets the init mode of all dbs of cd to NoDB, and returns cd
func etInitModeNone(cd *cluster.Data) *cluster.Data {
	for _, db := range cd.DBs {
		db.Spec.InitMode = cluster.NoDB
	}
	return cd
}

// etSyncStandbys enables synchronous replication on db, with standbys and no external synchronous standbys, and
// returns db
func etSyncStandbys(db *cluster.DB, standbys ...string) *cluster.DB {
	if standbys == nil {
		standbys = []string{}
	}
	db.Spec.SynchronousReplication = true
	db.Spec.SynchronousStandbys = standbys
	db.Spec.ExternalSynchronousStandbys = []string{}
	return db
}

// etElected changes cd as electing newMaster does: the old master and newMaster go to a new generation without
// followers, newMaster becomes a primary and the master of the cluster, and the proxy (at a new generation) points to
// no db until it converged
func etElected(cd *cluster.Data, newMaster string) *cluster.Data {
	old := cd.DBs[cd.Cluster.Status.Master]
	old.Generation++
	old.Spec.Followers = []string{}
	db := cd.DBs[newMaster]
	db.Generation++
	db.Spec.Role = common.RolePrimary
	db.Spec.FollowConfig = nil
	db.Spec.Followers = []string{}
	cd.Cluster.Status.Master = newMaster
	cd.Proxy.Generation++
	cd.Proxy.Spec.MasterDBUID = ""
	return cd
}

// etSentinel returns a sentinel set up like checkUpdateCluster does:
// reproducible UIDs (continuing after the highest dbN in cd) and convergence
// timers far in the past, so a db that has not converged counts as failed.
func etSentinel(cd *cluster.Data) *Sentinel {
	s := &Sentinel{
		uid:                    "sentinel01",
		UIDFn:                  testUIDFn,
		RandFn:                 testRandFn,
		keeperErrorTimers:      map[string]int64{},
		dbErrorTimers:          map[string]int64{},
		dbNotIncreasingXLogPos: map[string]int64{},
		dbConvergenceInfos:     map[string]*DBConvergenceInfo{},
		keeperInfoHistories:    KeeperInfoHistories{},
	}
	curUID = 0
	for _, db := range cd.DBs {
		uid, _ := strconv.Atoi(strings.TrimPrefix(db.UID, "db"))
		if uid > curUID {
			curUID = uid
		}
		s.dbConvergenceInfos[db.UID] = &DBConvergenceInfo{Generation: 0, Timer: int64(-1000 * time.Hour)}
	}
	return s
}

// etUpdateCluster runs updateCluster on cd with a sentinel of etSentinel, and fails on an error
func etUpdateCluster(cd *cluster.Data) *cluster.Data {
	GinkgoHelper()
	s := etSentinel(cd)
	outcd, err := s.updateCluster(context.Background(), cd, cluster.ProxiesInfo{})
	Ω(err).NotTo(HaveOccurred())
	return outcd
}

func etUIDs(dbs []*cluster.DB) []string {
	uids := []string{}
	for _, db := range dbs {
		uids = append(uids, db.UID)
	}
	return uids
}

// updateClusterCase is a case of the updateCluster table: cd goes in, outcd (or err) is expected
type updateClusterCase struct {
	cd    *cluster.Data
	outcd *cluster.Data
	err   error
}

// checkUpdateCluster runs updateCluster on the cd of tt and compares the result with its outcd, or its error
func checkUpdateCluster(tt updateClusterCase) {
	GinkgoHelper()
	s := &Sentinel{uid: "sentinel01", UIDFn: testUIDFn, RandFn: testRandFn,
		dbConvergenceInfos: map[string]*DBConvergenceInfo{}}

	// reset curUID func value to latest db uid
	curUID = 0
	for _, db := range tt.cd.DBs {
		uid, _ := strconv.Atoi(strings.TrimPrefix(db.UID, "db"))
		if uid > curUID {
			curUID = uid
		}
	}

	// Populate db convergence timers, these are populated with a negative
	// timer to make them result like not converged.
	for _, db := range tt.cd.DBs {
		s.dbConvergenceInfos[db.UID] = &DBConvergenceInfo{Generation: 0, Timer: int64(-1000 * time.Hour)}
	}

	outcd, err := s.updateCluster(context.Background(), tt.cd, cluster.ProxiesInfo{})
	if tt.err != nil {
		Ω(err).To(MatchError(tt.err.Error()))
		return
	}
	Ω(err).NotTo(HaveOccurred())
	Ω(testEqualCD(outcd, tt.outcd)).To(BeTrue(), func() string {
		return fmt.Sprintf("wrong outcd: got:\n%s\nwant:\n%s\nmismatch (-want +got):\n%s",
			spew.Sdump(outcd), spew.Sdump(tt.outcd), cmp.Diff(tt.outcd, outcd))
	})
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
