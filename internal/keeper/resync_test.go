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
	"context"

	"github.com/Masterminds/semver/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	pg "github.com/pgvillage-tools/orion/internal/postgresql"
)

var _ = Describe("resync", func() {
	var (
		ctx        context.Context
		db, master *cluster.DB
	)

	BeforeEach(func() {
		ctx = quietCtx()
		cd := smClusterData(smDB("db1", common.RoleReplica, cluster.ResyncDB))
		db, master = cd.DBs["db1"], cd.DBs["primary"]
	})

	Context("with pg_rewind", func() {
		BeforeEach(func() {
			db.Spec.UsePgrewind = true
		})

		It("uses pg_rewind", func() {
			p, f, _ := smKeeper(GinkgoTB())
			Expect(p.resync(ctx, db, master, master, true)).To(Succeed())
			Expect(f.sequence("SyncFromFollowedPGRewind", "SyncFromFollowed", "RemoveAllIfInitialized")).
				To(Equal("SyncFromFollowedPGRewind"))
			Expect(f.recovery.RecoveryMode).To(Equal(pg.RecoveryModeStandby), "recovery options = %+v", f.recovery)
			Expect(f.recovery.RecoveryParameters["primary_conninfo"]).To(ContainSubstring("host=10.0.0.2"))
			Expect(f.recovery.RecoveryParameters).To(HaveKeyWithValue("primary_slot_name", common.OrionName("db1")))
		})
		It("needs a superuser password for pg_rewind", func() {
			p, f, _ := smKeeper(GinkgoTB())
			p.pgSUPassword = ""
			Expect(p.resync(ctx, db, master, master, true)).To(Succeed())
			Expect(f.called("SyncFromFollowedPGRewind")).To(BeFalse(), "calls = %v", f.calls)
			Expect(f.called("SyncFromFollowed")).To(BeTrue(), "calls = %v", f.calls)
		})
		It("uses pg_basebackup with a replication slot", func() {
			p, f, _ := smKeeper(GinkgoTB())
			Expect(p.resync(ctx, db, master, master, false)).To(Succeed())
			Expect(f.sequence("RemoveAllIfInitialized", "SyncFromFollowed")).
				To(Equal("RemoveAllIfInitialized,SyncFromFollowed"))
			Expect(f.replSlot).To(Equal(common.OrionName("db1")))
			Expect(f.fastCheckpoint).To(BeFalse())
		})
		It("asks for a fast checkpoint when the db spec says so", func() {
			p, f, _ := smKeeper(GinkgoTB())
			db.Spec.ResyncFastCheckpoint = true
			Expect(p.resync(ctx, db, master, master, false)).To(Succeed())
			Expect(f.fastCheckpoint).To(BeTrue())
		})
		It("uses no replication slot before 9.6", func() {
			p, f, _ := smKeeper(GinkgoTB())
			f.version = semver.MustParse("9.5.0")
			Expect(p.resync(ctx, db, master, master, false)).To(Succeed())
			Expect(f.replSlot).To(BeEmpty())
		})
		DescribeTable("fails when a method fails",
			func(method string) {
				p, f, _ := smKeeper(GinkgoTB())
				f.errs[method] = errFake
				Expect(p.resync(ctx, db, master, master, false)).NotTo(Succeed())
			},
			Entry(nil, "RemoveAllIfInitialized"),
			Entry(nil, "SyncFromFollowed"),
		)
	})

	// without a binary version the keeper resyncs without a replication slot, and reports version 0.0
	It("works with an unknown binary version", func() {
		p, f, s := smKeeper(GinkgoTB())
		f.errs["BinaryVersion"] = errFake
		Expect(p.resync(ctx, db, master, master, false)).To(Succeed())
		Expect(f.called("SyncFromFollowed")).To(BeTrue(), "calls = %v", f.calls)
		Expect(f.replSlot).To(BeEmpty())

		p.keeperLocalState.ClusterUID = "cluster1"
		Expect(p.updateKeeperInfo(ctx)).To(Succeed())
		Expect(s.keeperInfo.PostgresBinaryVersion).To(Equal(cluster.PostgresBinaryVersion{}))
	})
})
