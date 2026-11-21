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
	. "github.com/onsi/ginkgo/v2"
	cluster "github.com/pgvillage-tools/orion/api/v1"
)

var _ = DescribeTable("updateCluster copying spec changes to the db specs (#14)",
	checkUpdateCluster,
	Entry("#14 Changed clusterSpec parameters. RequestTimeout, MaxStandbys, UsePgrewind, "+
		"PGParameters should bet updated in the DBSpecs.", func() updateClusterCase {
		cd := func() *cluster.Data { return etInitModeNone(etTableCluster(etOverridingSpec(), d2)) }

		// both dbs get the overridden spec values, and db1 synchronous replication with db2
		outcd := cd()
		master := etSyncStandbys(etOverridden(outcd.DBs[d1]), d2)
		master.Generation = g2
		master.Status.SynchronousStandbys = []string{}
		etOverridden(outcd.DBs[d2]).Generation = g2
		return updateClusterCase{cd: cd(), outcd: outcd}
	}()),
)
