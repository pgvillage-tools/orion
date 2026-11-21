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

package api

import (
	"encoding/json"
	"maps"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
)

var _ = Describe("cluster status", func() {
	role := func(r apiv1.Role) *apiv1.Spec { return &apiv1.Spec{Role: &r} }
	// data returns cluster data with a cluster (spec), the primary db master and the keepers
	data := func(spec *apiv1.Spec, master string, keepers ...string) *apiv1.Data {
		cd := &apiv1.Data{
			Cluster: &apiv1.Cluster{Spec: spec, Status: apiv1.ClusterStatus{Master: master}},
			DBs: apiv1.DBs{
				"db1": {UID: "db1", Spec: &apiv1.DBSpec{KeeperUID: "keeper1"}},
				"db2": {UID: "db2", Spec: &apiv1.DBSpec{KeeperUID: "keeper2"}},
			},
			Keepers: apiv1.Keepers{},
		}
		for _, k := range keepers {
			cd.Keepers[k] = &apiv1.Keeper{UID: k}
		}
		return cd
	}
	noSpec := data(nil, "db1", "keeper1")
	noSpec.DBs["db1"].Spec = nil

	DescribeTable("clusterStatus",
		func(cd *apiv1.Data, expected apiv1.InfoGenericStatus) {
			got := clusterStatus(cd)
			Expect(maps.Equal(got, expected)).To(BeTrue(), "expected %v, got %v", expected, got)
		},
		Entry("no cluster data", nil, apiv1.InfoGenericStatus{"available": false}),
		Entry("no cluster", &apiv1.Data{}, apiv1.InfoGenericStatus{"available": false}),
		Entry("primary cluster with a primary",
			data(&apiv1.Spec{}, "db1", "keeper1", "keeper2"),
			apiv1.InfoGenericStatus{"available": true, "role": "master",
				"primaryDB": "db1", "primaryKeeper": "keeper1"},
		),
		Entry("standby cluster: its primary replicates from the external primary",
			data(role(apiv1.Replica), "db2", "keeper1", "keeper2"),
			apiv1.InfoGenericStatus{"available": true, "role": "standby",
				"primaryDB": "db2", "primaryKeeper": "keeper2"},
		),
		Entry("no primary (yet)",
			data(&apiv1.Spec{}, "", "keeper1"),
			apiv1.InfoGenericStatus{"available": false, "role": "master"},
		),
		Entry("the primary db is not in the cluster data",
			data(&apiv1.Spec{}, "db3", "keeper1"),
			apiv1.InfoGenericStatus{"available": false, "role": "master"},
		),
		Entry("the keeper of the primary is not in the cluster data",
			data(&apiv1.Spec{}, "db1", "keeper2"),
			apiv1.InfoGenericStatus{"available": false, "role": "master"},
		),
		Entry("the primary db has no spec",
			noSpec,
			apiv1.InfoGenericStatus{"available": false, "role": "master"},
		),
	)

	It("encodes empty lists in the cluster info", func() {
		data, err := json.Marshal(newInfoCluster(nil, nil, nil, apiv1.InfoGenericStatus{"available": false}))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal(`{"proxies":[],"keepers":[],"sentinels":[],"status":{"available":false}}`))
	})
})
