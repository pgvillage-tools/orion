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
	"github.com/pgvillage-tools/orion/internal/util"
)

var _ = Describe("setDBSpecFromClusterSpec", func() {
	It("copies the resync settings of the cluster spec to every db", func() {
		spec := etAsyncSpec()
		spec.UsePgrewind = util.ToPtr(true)
		spec.ResyncFastCheckpoint = util.ToPtr(true)
		cd := etClusterData(spec, "db1", []*cluster.DB{etMaster("db1", "keeper1", 0), etStandby("db2", "keeper2", "db1", 0)})

		(&Sentinel{}).setDBSpecFromClusterSpec(cd)
		for _, db := range cd.DBs {
			Ω(db.Spec.UsePgrewind).To(BeTrue(), db.UID)
			Ω(db.Spec.ResyncFastCheckpoint).To(BeTrue(), db.UID)
		}
	})
})
