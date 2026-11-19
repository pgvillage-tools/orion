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
	"testing"

	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/util"
)

func TestUpdateReplSlots(t *testing.T) {
	ctx := quietCtx()
	orion := common.OrionName
	tests := []struct {
		name       string
		cur        []string
		followers  []string
		additional []string
		want       []string
	}{
		{
			name: "nothing",
		},
		{
			name:       "creates the slots of the followers and the additional slots",
			followers:  []string{"db2", "db3"},
			additional: []string{"extra"},
			want:       []string{orion("db2"), orion("db3"), orion("extra")},
		},
		{
			name:      "no slot for the db itself",
			followers: []string{"db1", "db2"},
			want:      []string{orion("db2")},
		},
		{
			name:      "drops the orion slots of former followers, and keeps the others",
			cur:       []string{orion("db2"), orion("old"), "user_slot"},
			followers: []string{"db2"},
			want:      []string{orion("db2"), "user_slot"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, f, _ := smKeeper(t)
			f.slots = tt.cur
			if err := p.updateReplSlots(ctx, tt.cur, "db1", tt.followers, tt.additional); err != nil {
				t.Fatal(err)
			}
			if !util.CompareStringSliceNoOrder(f.slots, tt.want) {
				t.Errorf("slots = %v, want %v", f.slots, tt.want)
			}
		})
	}
}

func TestUpdateReplSlotsErrors(t *testing.T) {
	ctx := quietCtx()

	// a slot that can't be dropped (its standby is still connected) is tried again next time
	p, f, _ := smKeeper(t)
	f.errs["DropReplicationSlot"] = errFake
	cur := []string{common.OrionName("old")}
	if err := p.updateReplSlots(ctx, cur, "db1", []string{"db2"}, nil); err != nil {
		t.Errorf("a failed drop is an error: %v", err)
	}
	if !f.called("CreateReplicationSlot") {
		t.Error("no slots created after a failed drop")
	}

	p, f, _ = smKeeper(t)
	f.errs["CreateReplicationSlot"] = errFake
	if err := p.updateReplSlots(ctx, nil, "db1", []string{"db2"}, nil); err == nil {
		t.Error("a failed create is no error")
	}
}

func TestRefreshReplicationSlots(t *testing.T) {
	ctx := quietCtx()
	p, f, _ := smKeeper(t)
	db := smDB("db1", common.RolePrimary, "")
	db.Spec.Followers = []string{"db2"}
	db.Spec.AdditionalReplicationSlots = []string{"extra"}
	f.slots = []string{common.OrionName("old")}
	if err := p.refreshReplicationSlots(ctx, nil, db); err != nil {
		t.Fatal(err)
	}
	if want := []string{common.OrionName("db2"), common.OrionName("extra")}; !util.CompareStringSliceNoOrder(f.slots,
		want) {
		t.Errorf("slots = %v, want %v", f.slots, want)
	}

	for _, method := range []string{"GetReplicationSlots", "CreateReplicationSlot"} {
		p, f, _ := smKeeper(t)
		f.errs[method] = errFake
		if err := p.refreshReplicationSlots(ctx, nil, db); err == nil {
			t.Errorf("%s fails: no error", method)
		}
	}
}
