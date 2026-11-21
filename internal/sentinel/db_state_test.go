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

package sentinel

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
)

var _ = Describe("dbCanSync", func() {
	const (
		master  = "master"
		standby = "standby"
		// the standby at WAL segment 5 and the master at 9 (16MB segments); the names of segments 3 and 7 (timeline 1)
		standbyPos    uint64 = 5 << 24
		masterPos     uint64 = 9 << 24
		olderWal3            = "000000010000000000000003"
		olderWal7            = "000000010000000000000007"
		notAWalName          = "not-a-wal-file"
		stuckForTimes        = cluster.DefaultDBNotIncreasingXLogPosTimes + 1
	)

	// clusterData returns a master whose oldest WAL file is olderWal, and a healthy standby that is behind, at
	// standbyPos
	clusterData := func(olderWal string) *cluster.Data {
		return &cluster.Data{
			Cluster: &cluster.Cluster{Status: cluster.ClusterStatus{Master: master}},
			DBs: cluster.DBs{
				master: &cluster.DB{
					UID:        master,
					Generation: cluster.InitialGeneration + 1,
					Spec:       &cluster.DBSpec{Role: common.RolePrimary},
					Status:     cluster.DBStatus{Healthy: true, XLogPos: masterPos, OlderWalFile: olderWal},
				},
				standby: &cluster.DB{
					UID:        standby,
					Generation: cluster.InitialGeneration + 1,
					Spec: &cluster.DBSpec{
						Role:         common.RoleReplica,
						FollowConfig: &cluster.FollowConfig{Type: cluster.FollowTypeInternal, DBUID: master},
					},
					Status: cluster.DBStatus{Healthy: true, XLogPos: standbyPos},
				},
			},
		}
	}

	DescribeTable("whether a db can still sync from the WAL the master keeps",
		func(olderWal string, stuck int64, change func(cd *cluster.Data), dbUID string, canSync bool) {
			cd := clusterData(olderWal)
			if change != nil {
				change(cd)
			}
			s := &Sentinel{dbNotIncreasingXLogPos: map[string]int64{standby: stuck}}
			Ω(s.dbCanSync(quietCtx(), cd, dbUID)).To(Equal(canSync))
		},
		Entry("can't when its WAL position stopped before the oldest WAL file of the master",
			olderWal7, int64(stuckForTimes), nil, standby, false),
		Entry("can when the master keeps the WAL file it needs",
			olderWal3, int64(stuckForTimes), nil, standby, true),
		Entry("can when the master reports no oldest WAL file",
			"", int64(stuckForTimes), nil, standby, true),
		Entry("can when the oldest WAL file of the master has a bad name",
			notAWalName, int64(stuckForTimes), nil, standby, true),
		Entry("can when it is the master",
			olderWal7, int64(stuckForTimes), nil, master, true),
		Entry("can when its WAL position still increases",
			olderWal7, int64(cluster.DefaultDBNotIncreasingXLogPosTimes), nil, standby, true),
		Entry("can when it follows an external db",
			olderWal7, int64(stuckForTimes), func(cd *cluster.Data) {
				cd.DBs[standby].Spec.FollowConfig.Type = cluster.FollowTypeExternal
			}, standby, true),
		Entry("can when it is initializing",
			olderWal7, int64(stuckForTimes), func(cd *cluster.Data) {
				cd.DBs[standby].Generation = cluster.InitialGeneration
			}, standby, true),
		Entry("can when it isn't healthy",
			olderWal7, int64(stuckForTimes), func(cd *cluster.Data) {
				cd.DBs[standby].Status.Healthy = false
			}, standby, true),
		Entry("can when it is at the WAL position of the master",
			olderWal7, int64(stuckForTimes), func(cd *cluster.Data) {
				cd.DBs[standby].Status.XLogPos = masterPos
			}, standby, true),
	)

	It("panics on an unknown db", func() {
		s := &Sentinel{dbNotIncreasingXLogPos: map[string]int64{}}
		Ω(func() { s.dbCanSync(quietCtx(), clusterData(olderWal7), "unknown") }).To(Panic())
	})
})
