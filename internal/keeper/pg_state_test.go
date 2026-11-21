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
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	pg "github.com/pgvillage-tools/orion/internal/postgresql"
)

var _ = Describe("the PostgreSQL state", func() {
	// statePG returns a keeper with an initialized instance on timeline 3 of system sys1
	statePG := func(t testing.TB) (*PostgresKeeper, *fakePG, *fakeStore) {
		p, f, s := smKeeper(t)
		p.dbLocalState = &DBLocalState{UID: "db1", Generation: 4}
		f.initialized = true
		f.configParams = common.Parameters{"work_mem": "8MB", "port": "5432", "primary_conninfo": "host=x"}
		f.syncStandbys = []string{common.OrionName("db2"), "external"}
		f.systemData = &pg.SystemData{SystemID: "sys1", TimelineID: 3, XLogPos: 1000}
		f.timelines = []*pg.TimelineHistory{
			{TimelineID: 1, SwitchPoint: 100, Reason: "a"},
			{TimelineID: 2, SwitchPoint: 200, Reason: "b"},
		}
		f.olderWalFile = "000000030000000000000001"
		return p, f, s
	}

	var ctx context.Context

	BeforeEach(func() {
		ctx = quietCtx()
	})

	Describe("GetPGState", func() {
		It("reports the state of the instance", func() {
			p, _, _ := statePG(GinkgoTB())
			got, err := p.GetPGState(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(&cluster.PostgresState{
				UID:                 "db1",
				Generation:          4,
				ListenAddress:       "10.0.0.1",
				Port:                "5432",
				Healthy:             true,
				SystemID:            "sys1",
				TimelineID:          3,
				XLogPos:             1000,
				PGParameters:        common.Parameters{"work_mem": "8MB"},
				SynchronousStandbys: []string{"db2"},
				OlderWalFile:        "000000030000000000000001",
				TimelinesHistory: cluster.PostgresTimelinesHistory{
					{TimelineID: 1, SwitchPoint: 100, Reason: "a"},
					{TimelineID: 2, SwitchPoint: 200, Reason: "b"},
				},
			}))
		})
		It("is healthy still without an older wal file", func() {
			p, f, _ := statePG(GinkgoTB())
			f.errs["OlderWalFile"] = errFake
			got, err := p.GetPGState(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(got.Healthy).To(BeTrue())
			Expect(got.OlderWalFile).To(BeEmpty())
		})
		It("has only the local state when not initialized", func() {
			p, f, _ := statePG(GinkgoTB())
			f.initialized = false
			got, err := p.GetPGState(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(got.Healthy).To(BeFalse())
			Expect(got.UID).To(Equal("db1"))
			Expect(got.SystemID).To(BeEmpty())
		})
		It("fails when IsInitialized fails", func() {
			p, f, _ := statePG(GinkgoTB())
			f.errs["IsInitialized"] = errFake
			_, err := p.GetPGState(ctx)
			Expect(err).To(HaveOccurred())
		})
		// the other failures give an unhealthy state, without an error
		DescribeTable("is unhealthy, without an error, when a method fails",
			func(method string) {
				p, f, _ := statePG(GinkgoTB())
				f.errs[method] = errFake
				got, err := p.GetPGState(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(got.Healthy).To(BeFalse())
				Expect(got.UID).To(Equal("db1"))
			},
			Entry(nil, "GetConfigFilePGParameters"),
			Entry(nil, "GetSyncStandbys"),
			Entry(nil, "GetSystemData"),
			Entry(nil, "GetTimelinesHistory"),
		)
	})

	It("updatePGState keeps the last state, and getLastPGState returns a copy", func() {
		p, f, _ := statePG(GinkgoTB())
		p.updatePGState(ctx)
		last := p.getLastPGState(ctx)
		Expect(last).NotTo(BeNil())
		Expect(last.Healthy).To(BeTrue())
		Expect(last.SystemID).To(Equal("sys1"))
		// a copy
		last.SystemID = "changed"
		Expect(p.getLastPGState(ctx).SystemID).To(Equal("sys1"),
			"getLastPGState returns the state itself, not a copy")

		f.errs["IsInitialized"] = errFake
		p.updatePGState(ctx)
		Expect(p.getLastPGState(ctx).Healthy).To(BeFalse(), "last pg state after an error")
	})

	It("GetInSyncStandbys", func() {
		p, f, _ := statePG(GinkgoTB())
		f.syncStandbys = nil
		got, err := p.GetInSyncStandbys(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).NotTo(BeNil())
		Expect(got).To(BeEmpty())
		f.errs["GetSyncStandbys"] = errFake
		_, err = p.GetInSyncStandbys(ctx)
		Expect(err).To(HaveOccurred())
	})

	It("updateKeeperInfo", func() {
		p, _, s := statePG(GinkgoTB())

		// without a cluster there is nothing to report
		Expect(p.updateKeeperInfo(ctx)).To(Succeed())
		Expect(s.keeperInfo).To(BeNil(), "keeper info without a cluster")

		p.keeperLocalState.ClusterUID = "cluster1"
		p.sleepInterval = 5 * time.Second
		p.canBeSynchronousReplica = p.canBeMaster
		p.updatePGState(ctx)
		Expect(p.updateKeeperInfo(ctx)).To(Succeed())
		ki := s.keeperInfo
		Expect(ki).NotTo(BeNil())
		Expect(ki.UID).To(Equal(testKeeperUID))
		Expect(ki.ClusterUID).To(Equal("cluster1"))
		Expect(ki.BootUUID).To(Equal(testBootUUID))
		Expect(ki.InfoUID).NotTo(BeEmpty())
		Expect(ki.PostgresBinaryVersion).To(Equal(cluster.PostgresBinaryVersion{Maj: 16, Min: 2}))
		Expect(ki.PostgresState).NotTo(BeNil())
		Expect(ki.PostgresState.SystemID).To(Equal("sys1"))
		Expect(ki.CanBeMaster).To(HaveValue(BeTrue()))
		Expect(ki.CanBeSynchronousReplica).NotTo(BeNil())
		Expect(s.keeperInfoTTL).To(Equal(5 * time.Second))

		s.err = errFake
		Expect(p.updateKeeperInfo(ctx)).NotTo(Succeed(), "no error from the store")
	})
})
