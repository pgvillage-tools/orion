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
	"time"

	. "github.com/onsi/ginkgo/v2"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/util"
)

var _ = Describe("updateCluster in the Normal phase: failover and standbys", func() {
	// oneStandbyPerSender returns etTableSpec with a MaxStandbysPerSender of 1
	oneStandbyPerSender := func() *cluster.Spec {
		spec := etTableSpec()
		spec.MaxStandbysPerSender = util.ToPtr(uint16(1))
		return spec
	}

	// newStandby returns a standby the sentinel just created: to resync from masterUID, not yet healthy or converged
	newStandby := func(uid, keeperUID, masterUID string) *cluster.DB {
		db := etTableStandby(uid, keeperUID, masterUID)
		db.Spec.InitMode = cluster.ResyncDB
		db.Status = cluster.DBStatus{}
		return db
	}

	DescribeTable("(#4-#13)",
		checkUpdateCluster,
		// Normal phase
		Entry("#4 One master and one standby, both healthy: no change from previous cd", func() updateClusterCase {
			cd := func() *cluster.Data { return etInitModeNone(etTableCluster(etTableSpec(), d2)) }
			return updateClusterCase{cd: cd(), outcd: cd()}
		}()),
		Entry("#5 One master and one standby, master db not healthy: standby elected as new master.",
			func() updateClusterCase {
				cd := func() *cluster.Data {
					cd := etTableCluster(etTableSpec(), d2)
					cd.DBs[d1].Status.Healthy = false
					return cd
				}
				return updateClusterCase{cd: cd(), outcd: etElected(cd(), d2)}
			}()),
		Entry("#6 From the previous test, new master (db2) converged. Old master setup to follow new "+
			"master (db2).", func() updateClusterCase {
			cd := etClusterData(etTableSpec(), d2, []*cluster.DB{etTableMaster(d1, k1, d2), etTableMaster(d2, k2)})
			cd.DBs[d1].Status.Healthy = false
			cd.DBs[d2].Generation, cd.DBs[d2].Status.CurrentGeneration = g2, g2
			cd.Proxy.Spec.MasterDBUID = ""

			// db1 is replaced by db3, a new standby of db2 on keeper1
			outcd := etClusterData(etTableSpec(), d2, []*cluster.DB{etTableMaster(d2, k2, d3), newStandby(d3, k1, d2)})
			outcd.DBs[d2].Generation, outcd.DBs[d2].Status.CurrentGeneration = g3, g2
			outcd.Proxy.Generation = g2
			return updateClusterCase{cd: cd, outcd: outcd}
		}()),
		Entry("#7 One master and one standby, master db not healthy, standby not converged (old "+
			"clusterview): no standby elected as new master, clusterview not changed.", func() updateClusterCase {
			cd := func() *cluster.Data {
				cd := etTableCluster(etTableSpec(), d2)
				cd.DBs[d1].Status.Healthy = false
				cd.DBs[d2].Status.CurrentGeneration = 0
				return cd
			}
			return updateClusterCase{cd: cd(), outcd: cd()}
		}()),
		Entry("#8 One master and one standby, master healthy but not converged: standby elected as new "+
			"master.", func() updateClusterCase {
			cd := func() *cluster.Data {
				cd := etTableCluster(etTableSpec(), d2)
				cd.DBs[d1].Status.CurrentGeneration = 0
				return cd
			}
			return updateClusterCase{cd: cd(), outcd: etElected(cd(), d2)}
		}()),
		Entry("#9 One master and one standby, 3 keepers (one available). Standby ok. No new standby db "+
			"on free keeper created.", func() updateClusterCase {
			cd := func() *cluster.Data {
				cd := etInitModeNone(etTableCluster(oneStandbyPerSender(), d2))
				cd.Keepers[k3] = etKeeper(k3)
				return cd
			}
			return updateClusterCase{cd: cd(), outcd: cd()}
		}()),
		Entry("#10 One master and one standby, 3 keepers (one available). Standby failed to converge "+
			"(keeper healthy). New standby db on free keeper created.", func() updateClusterCase {
			cd := func() *cluster.Data {
				cd := etTableCluster(oneStandbyPerSender(), d2)
				cd.Keepers[k3] = etKeeper(k3)
				cd.DBs[d2].Status.CurrentGeneration = 0
				return cd
			}
			outcd := cd()
			outcd.DBs[d1].Generation = g2
			outcd.DBs[d1].Spec.Followers = []string{d2, d3}
			outcd.DBs[d3] = newStandby(d3, k3, d1)
			return updateClusterCase{cd: cd(), outcd: outcd}
		}()),
		Entry("#11 From previous test. new standby db d3 converged, old standby db removed since "+
			"exceeds MaxStandbysPerSender.", func() updateClusterCase {
			cd := func() *cluster.Data {
				cd := etTableCluster(oneStandbyPerSender(), d2, d3)
				cd.DBs[d1].Generation, cd.DBs[d1].Status.CurrentGeneration = g2, g2
				cd.DBs[d2].Status.CurrentGeneration = 0
				cd.DBs[d3].Spec.InitMode = cluster.NoDB
				return cd
			}
			outcd := cd()
			delete(outcd.DBs, d2)
			outcd.DBs[d1].Generation = g3
			outcd.DBs[d1].Spec.Followers = []string{d3}
			return updateClusterCase{cd: cd(), outcd: outcd}
		}()),
		Entry("#12 One master and one standby, 2 keepers. Standby failed to converge (keeper healthy). "+
			"No standby db created since there's no free keeper.", func() updateClusterCase {
			cd := func() *cluster.Data { return etInitModeNone(etTableCluster(oneStandbyPerSender(), d2)) }
			return updateClusterCase{cd: cd(), outcd: cd()}
		}()),
		Entry("#13 One master and one keeper without db assigned. keeper2 dead for more then "+
			"DeadKeeperRemovalInterval: keeper2 removed.", func() updateClusterCase {
			cd := etInitModeNone(etTableCluster(etTableSpec()))
			cd.DBs[d1].Spec.Followers = []string{d2}
			cd.Keepers[k2] = etKeeper(k2)
			cd.Keepers[k2].Status.LastHealthyTime = now.Add(-100 * time.Hour)

			outcd := etInitModeNone(etTableCluster(etTableSpec()))
			outcd.DBs[d1].Generation = g2
			return updateClusterCase{cd: cd, outcd: outcd}
		}()),
	)
})
