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
	"reflect"
	"testing"
	"time"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	pg "github.com/pgvillage-tools/orion/internal/postgresql"
)

// statePG returns a keeper with an initialized instance on timeline 3 of system sys1
func statePG(t *testing.T) (*PostgresKeeper, *fakePG, *fakeStore) {
	p, f, s := smKeeper(t)
	p.dbLocalState = &DBLocalState{UID: "db1", Generation: 4}
	f.initialized = true
	f.configParams = common.Parameters{"work_mem": "8MB", "port": "5432", "primary_conninfo": "host=x"}
	f.syncStandbys = []string{common.OrionName("db2"), "external"}
	f.systemData = &pg.SystemData{SystemID: "sys1", TimelineID: 3, XLogPos: 1000}
	f.timelines = []*pg.TimelineHistory{
		{TimelineID: 1, SwitchPoint: 100, Reason: "a"},
		{TimelineID: 2, SwitchPoint: 200, Reason: "b"},
	}
	f.olderWalFile = "000000030000000000000001"
	return p, f, s
}

func TestGetPGState(t *testing.T) {
	ctx := quietCtx()
	p, _, _ := statePG(t)
	got, err := p.GetPGState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := &cluster.PostgresState{
		UID:                 "db1",
		Generation:          4,
		ListenAddress:       "10.0.0.1",
		Port:                "5432",
		Healthy:             true,
		SystemID:            "sys1",
		TimelineID:          3,
		XLogPos:             1000,
		PGParameters:        common.Parameters{"work_mem": "8MB"},
		SynchronousStandbys: []string{"db2"},
		OlderWalFile:        "000000030000000000000001",
		TimelinesHistory: cluster.PostgresTimelinesHistory{
			{TimelineID: 1, SwitchPoint: 100, Reason: "a"},
			{TimelineID: 2, SwitchPoint: 200, Reason: "b"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pg state = %+v\nwant %+v", got, want)
	}

	// without an older wal file the state is healthy still
	p, f, _ := statePG(t)
	f.errs["OlderWalFile"] = errFake
	if got, err := p.GetPGState(ctx); err != nil || !got.Healthy || got.OlderWalFile != "" {
		t.Errorf("without older wal file: %+v, %v", got, err)
	}

	// not initialized: only the local state
	p, f, _ = statePG(t)
	f.initialized = false
	if got, err := p.GetPGState(ctx); err != nil || got.Healthy || got.UID != "db1" || got.SystemID != "" {
		t.Errorf("not initialized: %+v, %v", got, err)
	}
}

func TestGetPGStateErrors(t *testing.T) {
	ctx := quietCtx()
	p, f, _ := statePG(t)
	f.errs["IsInitialized"] = errFake
	if _, err := p.GetPGState(ctx); err == nil {
		t.Error("no error when IsInitialized fails")
	}

	// the other failures give an unhealthy state, without an error
	for _, method := range []string{"GetConfigFilePGParameters", "GetSyncStandbys", "GetSystemData",
		"GetTimelinesHistory"} {
		p, f, _ := statePG(t)
		f.errs[method] = errFake
		got, err := p.GetPGState(ctx)
		if err != nil || got.Healthy || got.UID != "db1" {
			t.Errorf("%s fails: %+v, %v", method, got, err)
		}
	}
}

func TestUpdatePGState(t *testing.T) {
	ctx := quietCtx()
	p, f, _ := statePG(t)
	p.updatePGState(ctx)
	last := p.getLastPGState(ctx)
	if last == nil || !last.Healthy || last.SystemID != "sys1" {
		t.Fatalf("last pg state = %+v", last)
	}
	// a copy
	last.SystemID = "changed"
	if p.getLastPGState(ctx).SystemID != "sys1" {
		t.Error("getLastPGState returns the state itself, not a copy")
	}

	f.errs["IsInitialized"] = errFake
	p.updatePGState(ctx)
	if last := p.getLastPGState(ctx); last.Healthy {
		t.Errorf("last pg state after an error = %+v", last)
	}
}

func TestGetInSyncStandbys(t *testing.T) {
	ctx := quietCtx()
	p, f, _ := statePG(t)
	f.syncStandbys = nil
	if got, err := p.GetInSyncStandbys(ctx); err != nil || got == nil || len(got) != 0 {
		t.Errorf("no sync standbys: %#v, %v", got, err)
	}
	f.errs["GetSyncStandbys"] = errFake
	if _, err := p.GetInSyncStandbys(ctx); err == nil {
		t.Error("no error")
	}
}

func TestUpdateKeeperInfo(t *testing.T) {
	ctx := quietCtx()
	p, _, s := statePG(t)

	// without a cluster there is nothing to report
	if err := p.updateKeeperInfo(ctx); err != nil || s.keeperInfo != nil {
		t.Fatalf("keeper info without a cluster: %+v, %v", s.keeperInfo, err)
	}

	p.keeperLocalState.ClusterUID = "cluster1"
	p.sleepInterval = 5 * time.Second
	p.canBeSynchronousReplica = p.canBeMaster
	p.updatePGState(ctx)
	if err := p.updateKeeperInfo(ctx); err != nil {
		t.Fatal(err)
	}
	ki := s.keeperInfo
	if ki == nil || ki.UID != testKeeperUID || ki.ClusterUID != "cluster1" || ki.BootUUID != testBootUUID ||
		ki.InfoUID == "" {
		t.Fatalf("keeper info = %+v", ki)
	}
	if ki.PostgresBinaryVersion != (cluster.PostgresBinaryVersion{Maj: 16, Min: 2}) {
		t.Errorf("binary version = %+v", ki.PostgresBinaryVersion)
	}
	if ki.PostgresState == nil || ki.PostgresState.SystemID != "sys1" {
		t.Errorf("pg state = %+v", ki.PostgresState)
	}
	if ki.CanBeMaster == nil || !*ki.CanBeMaster || ki.CanBeSynchronousReplica == nil {
		t.Errorf("can be master %v, can be sync replica %v", ki.CanBeMaster, ki.CanBeSynchronousReplica)
	}
	if s.keeperInfoTTL != 5*time.Second {
		t.Errorf("ttl = %v", s.keeperInfoTTL)
	}

	s.err = errFake
	if err := p.updateKeeperInfo(ctx); err == nil {
		t.Error("no error from the store")
	}
}
