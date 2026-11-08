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

package v1

import (
	"testing"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
)

func TestCanFailOver(t *testing.T) {
	// data returns cluster data with a primary db1 on keeper1 (timeline 3), and a standby db2 on keeper2 that is
	// changed by standby
	data := func(standby func(k *apiv1.Keeper, db *apiv1.DB)) *apiv1.Data {
		k2 := &apiv1.Keeper{UID: "keeper2", Status: apiv1.KeeperStatus{Healthy: true}}
		db2 := &apiv1.DB{UID: "db2", Generation: 5, Spec: &apiv1.DBSpec{KeeperUID: "keeper2"},
			Status: apiv1.DBStatus{Healthy: true, CurrentGeneration: 5, TimelineID: 3}}
		if standby != nil {
			standby(k2, db2)
		}
		return &apiv1.Data{
			Cluster: &apiv1.Cluster{Status: apiv1.ClusterStatus{Master: "db1"}},
			Keepers: apiv1.Keepers{"keeper1": {UID: "keeper1", Status: apiv1.KeeperStatus{Healthy: true}}, "keeper2": k2},
			DBs: apiv1.DBs{
				"db1": {UID: "db1", Spec: &apiv1.DBSpec{KeeperUID: "keeper1"}, Status: apiv1.DBStatus{TimelineID: 3}},
				"db2": db2,
			},
		}
	}
	for name, tc := range map[string]struct {
		cd       *apiv1.Data
		keeper   string
		expected bool
	}{
		"a standby can take over":            {data(nil), "keeper1", true},
		"failing a standby is fine":          {data(func(k *apiv1.Keeper, _ *apiv1.DB) { k.Status.Healthy = false }), "keeper2", true},
		"the standby's keeper is unhealthy":  {data(func(k *apiv1.Keeper, _ *apiv1.DB) { k.Status.Healthy = false }), "keeper1", false},
		"the standby's db is unhealthy":      {data(func(_ *apiv1.Keeper, db *apiv1.DB) { db.Status.Healthy = false }), "keeper1", false},
		"the standby's db isn't converged":   {data(func(_ *apiv1.Keeper, db *apiv1.DB) { db.Status.CurrentGeneration = 4 }), "keeper1", false},
		"the standby is on another timeline": {data(func(_ *apiv1.Keeper, db *apiv1.DB) { db.Status.TimelineID = 2 }), "keeper1", false},
		"the only keeper": {func() *apiv1.Data {
			cd := data(nil)
			delete(cd.DBs, "db2")
			delete(cd.Keepers, "keeper2")
			return cd
		}(), "keeper1", false},
		"no primary": {func() *apiv1.Data {
			cd := data(nil)
			cd.Cluster.Status.Master = ""
			return cd
		}(), "keeper1", true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := canFailOver(tc.cd, tc.cd.Keepers[tc.keeper]); got != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}
