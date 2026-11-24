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
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/consensus"
	pg "github.com/pgvillage-tools/orion/internal/postgresql"
	"github.com/pgvillage-tools/orion/internal/util"
)

// fakePG is a pgManager without PostgreSQL. It records the calls (by method name) and fails the methods in errs.
// The calls change its state like the real ones: Init and the syncs initialize the instance, the starts start it, a
// promote makes it a primary. Like a *pg.Manager it keeps the parameters, recovery options and hba that are set apart
// from the current ones, the ones the instance was (re)started or reloaded with.
type fakePG struct {
	calls []string
	errs  map[string]error

	initialized bool
	started     bool
	role        common.Role
	systemID    string
	version     *semver.Version

	params          common.Parameters
	curParams       common.Parameters
	recovery        *pg.RecoveryOptions
	curRecovery     *pg.RecoveryOptions
	hba             []string
	curHba          []string
	configParams    common.Parameters
	restartRequired bool

	syncStandbys []string
	systemData   *pg.SystemData
	timelines    cluster.PostgresTimelinesHistory
	olderWalFile string
	slots        []string
	// replSlot is the replication slot of the last SyncFromFollowed
	replSlot string
	// fastCheckpoint is what the last SyncFromFollowed got for it
	fastCheckpoint bool

	// waitReady, when set, answers WaitReady instead of errs
	waitReady func() error
}

var _ pgManager = (*fakePG)(nil)

func newFakePG() *fakePG {
	return &fakePG{
		errs:        map[string]error{},
		version:     semver.MustParse("16.2"),
		params:      common.Parameters{},
		curParams:   common.Parameters{},
		recovery:    pg.NewRecoveryOptions(),
		curRecovery: pg.NewRecoveryOptions(),
	}
}

func (f *fakePG) call(name string) error {
	f.calls = append(f.calls, name)
	return f.errs[name]
}

// called tells if method was called
func (f *fakePG) called(method string) bool { return slices.Contains(f.calls, method) }

// sequence returns the calls of the given methods, in order
func (f *fakePG) sequence(methods ...string) string {
	var seq []string
	for _, c := range f.calls {
		if slices.Contains(methods, c) {
			seq = append(seq, c)
		}
	}
	return strings.Join(seq, ",")
}

func (f *fakePG) GetTimelinesHistory(_ context.Context, _ uint64) (cluster.PostgresTimelinesHistory, error) {
	return f.timelines, f.call("GetTimelinesHistory")
}

func (f *fakePG) SetParameters(parameters common.Parameters) {
	f.calls = append(f.calls, "SetParameters")
	f.params = parameters
}

func (f *fakePG) CurParameters() common.Parameters { return f.curParams }

func (f *fakePG) SetRecoveryOptions(recoveryOptions *pg.RecoveryOptions) {
	f.calls = append(f.calls, "SetRecoveryOptions")
	if recoveryOptions == nil {
		recoveryOptions = pg.NewRecoveryOptions()
	}
	f.recovery = recoveryOptions
}

func (f *fakePG) CurRecoveryOptions() *pg.RecoveryOptions { return f.curRecovery }

func (f *fakePG) SetHba(hba []string) {
	f.calls = append(f.calls, "SetHba")
	f.hba = hba
}

func (f *fakePG) CurHba() []string { return f.curHba }

func (f *fakePG) GetConfigFilePGParameters(_ context.Context) (common.Parameters, error) {
	return f.configParams, f.call("GetConfigFilePGParameters")
}

func (f *fakePG) IsRestartRequired(_ context.Context, _ []string) (bool, error) {
	return f.restartRequired, f.call("IsRestartRequired")
}

func (f *fakePG) IsInitialized() (bool, error) { return f.initialized, f.call("IsInitialized") }

func (f *fakePG) Init(_ context.Context, _ *cluster.NewConfig) error {
	if err := f.call("Init"); err != nil {
		return err
	}
	f.initialized, f.role = true, common.RolePrimary
	return nil
}

func (f *fakePG) Restore(_ context.Context, _ string) error {
	if err := f.call("Restore"); err != nil {
		return err
	}
	f.initialized, f.role = true, common.RolePrimary
	return nil
}

func (f *fakePG) RemoveAllIfInitialized(_ context.Context) error {
	if err := f.call("RemoveAllIfInitialized"); err != nil {
		return err
	}
	f.initialized = false
	return nil
}

func (f *fakePG) SyncFromFollowed(_ context.Context, _ pg.ConnParams, replSlot string, fastCheckpoint bool) error {
	f.replSlot, f.fastCheckpoint = replSlot, fastCheckpoint
	if err := f.call("SyncFromFollowed"); err != nil {
		return err
	}
	f.initialized, f.role = true, common.RoleReplica
	return nil
}

func (f *fakePG) SyncFromFollowedPGRewind(_ context.Context, _ pg.ConnParams, _ string) error {
	if err := f.call("SyncFromFollowedPGRewind"); err != nil {
		return err
	}
	f.role = common.RoleReplica
	return nil
}

// apply makes the settings current, as a start or reload does
func (f *fakePG) apply() {
	f.curParams = maps.Clone(f.params)
	f.curRecovery = f.recovery.DeepCopy()
	f.curHba = slices.Clone(f.hba)
}

func (f *fakePG) start(name string) error {
	if err := f.call(name); err != nil {
		return err
	}
	f.started = true
	f.apply()
	return nil
}

func (f *fakePG) Start(_ context.Context) error          { return f.start("Start") }
func (f *fakePG) StartTmpMerged(_ context.Context) error { return f.start("StartTmpMerged") }
func (f *fakePG) Restart(_ context.Context, _ bool) error {
	return f.start("Restart")
}

func (f *fakePG) StopIfStarted(_ context.Context, _ bool) error {
	if err := f.call("StopIfStarted"); err != nil {
		return err
	}
	f.started = false
	return nil
}

func (f *fakePG) Reload(_ context.Context) error {
	if err := f.call("Reload"); err != nil {
		return err
	}
	f.apply()
	return nil
}

func (f *fakePG) IsStarted() (bool, error) { return f.started, f.call("IsStarted") }

func (f *fakePG) WaitReady(_ context.Context, _ time.Duration) error {
	if f.waitReady != nil {
		f.calls = append(f.calls, "WaitReady")
		return f.waitReady()
	}
	return f.call("WaitReady")
}

func (f *fakePG) WaitRecoveryDone(_ time.Duration) error { return f.call("WaitRecoveryDone") }

func (f *fakePG) Promote(_ context.Context) error {
	if err := f.call("Promote"); err != nil {
		return err
	}
	f.role = common.RolePrimary
	return nil
}

func (f *fakePG) SetupRoles(_ context.Context) error { return f.call("SetupRoles") }

// BinaryVersion returns no version with an error, like the real one
func (f *fakePG) BinaryVersion() (*semver.Version, error) {
	if err := f.call("BinaryVersion"); err != nil {
		return nil, err
	}
	return f.version, nil
}

func (f *fakePG) GetRole() (common.Role, error) { return f.role, f.call("GetRole") }

func (f *fakePG) GetSystemID() (string, error) { return f.systemID, f.call("GetSystemID") }

func (f *fakePG) GetSystemData(_ context.Context) (*pg.SystemData, error) {
	return f.systemData, f.call("GetSystemData")
}

func (f *fakePG) GetSyncStandbys(_ context.Context) ([]string, error) {
	return f.syncStandbys, f.call("GetSyncStandbys")
}

func (f *fakePG) OlderWalFile() (string, error) { return f.olderWalFile, f.call("OlderWalFile") }

func (f *fakePG) GetReplicationSlots(_ context.Context) ([]string, error) {
	return slices.Clone(f.slots), f.call("GetReplicationSlots")
}

func (f *fakePG) CreateReplicationSlot(_ context.Context, name string) error {
	if err := f.call("CreateReplicationSlot"); err != nil {
		return err
	}
	f.slots = append(f.slots, name)
	return nil
}

func (f *fakePG) DropReplicationSlot(_ context.Context, name string) error {
	if err := f.call("DropReplicationSlot"); err != nil {
		return err
	}
	f.slots = slices.DeleteFunc(f.slots, func(s string) bool { return s == name })
	return nil
}

// fakeStore serves cd (or err) as the cluster data and keeps the last keeper info set. The other Store methods are
// not used by the keeper, and panic.
type fakeStore struct {
	consensus.Store

	cd  *cluster.Data
	err error

	keeperInfo    *cluster.KeeperInfo
	keeperInfoTTL time.Duration
}

func (s *fakeStore) GetClusterData(_ context.Context) (*cluster.Data, *consensus.KVPair, error) {
	return s.cd, nil, s.err
}

func (s *fakeStore) SetKeeperInfo(_ context.Context, _ string, ki *cluster.KeeperInfo, ttl time.Duration) error {
	if s.err != nil {
		return s.err
	}
	s.keeperInfo, s.keeperInfoTTL = ki, ttl
	return nil
}

const (
	testKeeperUID = "keeper1"
	testBootUUID  = "boot1"
)

// smKeeper returns a keeper (uid keeper1, boot uuid boot1) with its local state in a temporary data dir, a fake
// PostgreSQL manager and a fake store
func smKeeper(t testing.TB) (*PostgresKeeper, *fakePG, *fakeStore) {
	t.Helper()
	f := newFakePG()
	s := &fakeStore{}
	p := &PostgresKeeper{
		cfg:                   &Config{DataDir: t.TempDir()},
		bootUUID:              testBootUUID,
		pgListenAddress:       "127.0.0.1",
		pgAdvertiseAddress:    "10.0.0.1",
		pgPort:                "5432",
		pgAdvertisePort:       "5432",
		pgSUConnType:          connTypeHost,
		pgSUAuthMethod:        authScram,
		pgSULocalAuthMethod:   authScram,
		pgSUUsername:          "superuser",
		pgSUPassword:          "supass",
		pgReplConnType:        connTypeHost,
		pgReplAuthMethod:      authScram,
		pgReplLocalAuthMethod: authScram,
		pgReplUsername:        "repluser",
		pgReplPassword:        "replpass",
		keeperLocalState:      &LocalState{UID: testKeeperUID},
		dbLocalState:          &DBLocalState{},
		canBeMaster:           util.ToPtr(true),
		e:                     s,
		pgm:                   f,
	}
	return p, f, s
}

// smDB returns a db of keeper1 with the given role and init mode; a replica follows db "primary"
func smDB(uid string, role common.Role, initMode cluster.DBInitMode) *cluster.DB {
	db := &cluster.DB{
		UID:        uid,
		Generation: 3,
		Spec: &cluster.DBSpec{
			KeeperUID: testKeeperUID,
			Role:      role,
			InitMode:  initMode,
		},
	}
	if role == common.RoleReplica {
		db.Spec.FollowConfig = &cluster.FollowConfig{Type: cluster.FollowTypeInternal, DBUID: "primary"}
	}
	return db
}

// smClusterData returns valid cluster data (uid cluster1) with keeper1 (boot uuid boot1), db and the other dbs. A
// db "primary" on another keeper is the master.
func smClusterData(db *cluster.DB, others ...*cluster.DB) *cluster.Data {
	cd := cluster.NewClusterData(cluster.NewCluster("cluster1", &cluster.Spec{InitMode: util.ToPtr(cluster.New)}))
	cd.Keepers[testKeeperUID] = &cluster.Keeper{UID: testKeeperUID, Status: cluster.KeeperStatus{BootUUID: testBootUUID}}
	primary := &cluster.DB{
		UID:    "primary",
		Spec:   &cluster.DBSpec{KeeperUID: "keeper0", Role: common.RolePrimary},
		Status: cluster.DBStatus{ListenAddress: "10.0.0.2", Port: "5432", SystemID: "sys1", TimelineID: 1},
	}
	cd.DBs[primary.UID] = primary
	cd.Cluster.Status.Master = primary.UID
	if db != nil {
		cd.DBs[db.UID] = db
	}
	for _, o := range others {
		cd.DBs[o.UID] = o
	}
	return cd
}
