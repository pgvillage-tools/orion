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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/common"
)

var _ = Describe("the replication slots", func() {
	var ctx context.Context
	orion := common.OrionName

	BeforeEach(func() {
		ctx = quietCtx()
	})

	DescribeTable("updateReplSlots",
		func(cur, followers, additional, want []string) {
			p, f, _ := smKeeper(GinkgoTB())
			f.slots = cur
			Expect(p.updateReplSlots(ctx, cur, "db1", followers, additional)).To(Succeed())
			if len(want) == 0 {
				Expect(f.slots).To(BeEmpty())
			} else {
				Expect(f.slots).To(ConsistOf(want))
			}
		},
		Entry("nothing", nil, nil, nil, nil),
		Entry("creates the slots of the followers and the additional slots",
			nil, []string{"db2", "db3"}, []string{"extra"}, []string{orion("db2"), orion("db3"), orion("extra")}),
		Entry("no slot for the db itself", nil, []string{"db1", "db2"}, nil, []string{orion("db2")}),
		Entry("drops the orion slots of former followers, and keeps the others",
			[]string{orion("db2"), orion("old"), "user_slot"}, []string{"db2"}, nil,
			[]string{orion("db2"), "user_slot"}),
	)

	// a slot that can't be dropped (its standby is still connected) is tried again next time
	It("a failed drop is no error", func() {
		p, f, _ := smKeeper(GinkgoTB())
		f.errs["DropReplicationSlot"] = errFake
		cur := []string{orion("old")}
		Expect(p.updateReplSlots(ctx, cur, "db1", []string{"db2"}, nil)).To(Succeed(), "a failed drop is an error")
		Expect(f.called("CreateReplicationSlot")).To(BeTrue(), "no slots created after a failed drop")
	})

	It("a failed create is an error", func() {
		p, f, _ := smKeeper(GinkgoTB())
		f.errs["CreateReplicationSlot"] = errFake
		Expect(p.updateReplSlots(ctx, nil, "db1", []string{"db2"}, nil)).NotTo(Succeed(),
			"a failed create is no error")
	})

	Describe("refreshReplicationSlots", func() {
		It("creates the slots of the followers and drops the old ones", func() {
			p, f, _ := smKeeper(GinkgoTB())
			db := smDB("db1", common.RolePrimary, "")
			db.Spec.Followers = []string{"db2"}
			db.Spec.AdditionalReplicationSlots = []string{"extra"}
			f.slots = []string{orion("old")}
			Expect(p.refreshReplicationSlots(ctx, nil, db)).To(Succeed())
			Expect(f.slots).To(ConsistOf(orion("db2"), orion("extra")))
		})

		DescribeTable("fails when a method fails",
			func(method string) {
				p, f, _ := smKeeper(GinkgoTB())
				db := smDB("db1", common.RolePrimary, "")
				db.Spec.Followers = []string{"db2"}
				db.Spec.AdditionalReplicationSlots = []string{"extra"}
				f.errs[method] = errFake
				Expect(p.refreshReplicationSlots(ctx, nil, db)).NotTo(Succeed())
			},
			Entry(nil, "GetReplicationSlots"),
			Entry(nil, "CreateReplicationSlot"),
		)
	})
})
