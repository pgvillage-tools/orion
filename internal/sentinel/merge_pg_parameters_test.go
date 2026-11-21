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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cluster "github.com/pgvillage-tools/orion/api/v1"
)

// With mergePgParameters, the cluster spec gets the parameters of the initialized db. The keeper reports them without
// the ones it manages, so the spec values of those must stay.
var _ = DescribeTable("merging the parameters of the initialized db into the cluster spec",
	func(mode cluster.InitMode) {
		spec := initSpec(mode)
		spec.PGParameters = cluster.PGParameters{"wal_keep_size": "1GB", "work_mem": "8MB"}
		cd := etClusterData(spec, "", nil, "keeper1", "keeper2")
		cd.Cluster.Status.Phase = cluster.Initializing
		// initSpec takes the existing db from keeper2
		keeper := "keeper1"
		if mode == cluster.ExistingCluster {
			keeper = "keeper2"
		}
		db := etMaster("db1", keeper, 0)
		db.Generation = 2
		db.Status.CurrentGeneration = 2
		// what the keeper reports: its configuration (initdb's, with the spec's), without wal_keep_size
		db.Status.PGParameters = cluster.PGParameters{"max_connections": "150", "work_mem": "8MB"}
		cd.DBs[db.UID] = db
		cd.Cluster.Status.Master = db.UID

		outcd, err := etSentinel(cd).updateCluster(context.Background(), cd, cluster.ProxiesInfo{})
		Ω(err).NotTo(HaveOccurred())
		Ω(outcd.Cluster.Status.Phase).To(Equal(cluster.Normal))
		Ω(outcd.Cluster.Spec.PGParameters).To(Equal(cluster.PGParameters{
			"max_connections": "150", "work_mem": "8MB", "wal_keep_size": "1GB"}))
	},
	Entry("keeps a managed parameter of the spec in a new cluster", cluster.New),
	Entry("keeps a managed parameter of the spec in an existing cluster", cluster.ExistingCluster),
	Entry("keeps a managed parameter of the spec in a pitr cluster", cluster.PITR),
)

var _ = Describe("mergedPGParameters", func() {
	It("puts the spec values over the reported ones", func() {
		Ω(mergedPGParameters(cluster.PGParameters{"work_mem": "8MB"},
			cluster.PGParameters{"work_mem": "4MB", "max_connections": "150"})).
			To(Equal(cluster.PGParameters{"work_mem": "8MB", "max_connections": "150"}))
	})
	It("doesn't change its arguments", func() {
		spec, reported := cluster.PGParameters{"a": "1"}, cluster.PGParameters{"b": "2"}
		mergedPGParameters(spec, reported)
		Ω(spec).To(Equal(cluster.PGParameters{"a": "1"}))
		Ω(reported).To(Equal(cluster.PGParameters{"b": "2"}))
	})
})
