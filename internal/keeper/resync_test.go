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
	"strings"
	"testing"

	"github.com/Masterminds/semver/v3"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	pg "github.com/pgvillage-tools/orion/internal/postgresql"
)

func TestResync(t *testing.T) {
	ctx := quietCtx()
	cd := smClusterData(smDB("db1", common.RoleReplica, cluster.ResyncDB))
	db, master := cd.DBs["db1"], cd.DBs["primary"]
	db.Spec.UsePgrewind = true

	t.Run("pg_rewind", func(t *testing.T) {
		p, f, _ := smKeeper(t)
		if err := p.resync(ctx, db, master, master, true); err != nil {
			t.Fatal(err)
		}
		if f.sequence("SyncFromFollowedPGRewind", "SyncFromFollowed", "RemoveAllIfInitialized") !=
			"SyncFromFollowedPGRewind" {
			t.Errorf("calls = %v", f.calls)
		}
		conninfo := f.recovery.RecoveryParameters["primary_conninfo"]
		if f.recovery.RecoveryMode != pg.RecoveryModeStandby || !strings.Contains(conninfo, "host=10.0.0.2") ||
			f.recovery.RecoveryParameters["primary_slot_name"] != common.OrionName("db1") {
			t.Errorf("recovery options = %+v", f.recovery)
		}
	})
	t.Run("pg_rewind needs a superuser password", func(t *testing.T) {
		p, f, _ := smKeeper(t)
		p.pgSUPassword = ""
		if err := p.resync(ctx, db, master, master, true); err != nil {
			t.Fatal(err)
		}
		if f.called("SyncFromFollowedPGRewind") || !f.called("SyncFromFollowed") {
			t.Errorf("calls = %v", f.calls)
		}
	})
	t.Run("pg_basebackup with a replication slot", func(t *testing.T) {
		p, f, _ := smKeeper(t)
		if err := p.resync(ctx, db, master, master, false); err != nil {
			t.Fatal(err)
		}
		if f.sequence("RemoveAllIfInitialized", "SyncFromFollowed") != "RemoveAllIfInitialized,SyncFromFollowed" {
			t.Errorf("calls = %v", f.calls)
		}
		if f.replSlot != common.OrionName("db1") {
			t.Errorf("replication slot = %q", f.replSlot)
		}
	})
	t.Run("no replication slot before 9.6", func(t *testing.T) {
		p, f, _ := smKeeper(t)
		f.version = semver.MustParse("9.5.0")
		if err := p.resync(ctx, db, master, master, false); err != nil {
			t.Fatal(err)
		}
		if f.replSlot != "" {
			t.Errorf("replication slot = %q", f.replSlot)
		}
	})
	for _, method := range []string{"RemoveAllIfInitialized", "SyncFromFollowed"} {
		t.Run(method+" fails", func(t *testing.T) {
			p, f, _ := smKeeper(t)
			f.errs[method] = errFake
			if err := p.resync(ctx, db, master, master, false); err == nil {
				t.Error("no error")
			}
		})
	}
}
