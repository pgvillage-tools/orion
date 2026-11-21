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

var _ = Describe("syncStandbyLimits", func() {
	const (
		specMin = 2
		specMax = 3
	)

	// limits returns the limits for a spec with specMin and specMax, and a master keeper running PostgreSQL maj.min
	limits := func(maj, minor int) (int, int, bool, error) {
		spec := etSyncSpec()
		spec.MinSynchronousStandbys = util.ToPtr(uint16(specMin))
		spec.MaxSynchronousStandbys = util.ToPtr(uint16(specMax))
		keeper := etKeeper(etK1)
		keeper.Status.PostgresBinaryVersion = cluster.PostgresBinaryVersion{Maj: maj, Min: minor}
		return syncStandbyLimits(spec, keeper)
	}

	DescribeTable("the limits of the synchronous standbys",
		func(maj, minor, wantMin, wantMax int, wantMerge bool) {
			gotMin, gotMax, merge, err := limits(maj, minor)
			Ω(err).NotTo(HaveOccurred())
			Ω(gotMin).To(Equal(wantMin))
			Ω(gotMax).To(Equal(wantMax))
			Ω(merge).To(Equal(wantMerge))
		},
		Entry("come from the spec, merging, when the PostgreSQL version of the master keeper is unknown",
			0, 0, specMin, specMax, true),
		Entry("come from the spec, merging, on PostgreSQL 9.6",
			9, 6, specMin, specMax, true),
		Entry("come from the spec, merging, on PostgreSQL 10",
			10, 0, specMin, specMax, true),
		Entry("come from the spec, merging, on PostgreSQL 17",
			17, 2, specMin, specMax, true),
		Entry("are one, without merging, on PostgreSQL 9.5",
			9, 5, 1, 1, false),
		Entry("are one, without merging, on PostgreSQL 9.4",
			9, 4, 1, 1, false),
	)

	DescribeTable("fails on a PostgreSQL version that isn't a semver",
		func(maj, minor int, wantErr string) {
			_, _, _, err := limits(maj, minor)
			Ω(err).To(MatchError(wantErr))
		},
		Entry("with a negative major version", -1, 0, "unable to convert to semver: -1.0"),
		Entry("with a negative minor version", 9, -1, "unable to convert to semver: 9.-1"),
	)
})

var _ = Describe("pruneSynchronousStandbys", func() {
	const etK4 = "keeper4"

	// set returns a set of the uids
	set := func(uids ...string) map[string]struct{} {
		m := map[string]struct{}{}
		for _, uid := range uids {
			m[uid] = struct{}{}
		}
		return m
	}

	// prune runs pruneSynchronousStandbys on a cluster of master db1 and standbys db2, db3 and db4, without the dbs in
	// missing, with the synchronous standbys sync and the good standbys good, and returns the synchronous standbys left
	prune := func(sync, good, missing []string, maxSync int) map[string]struct{} {
		GinkgoHelper()
		cd := etClusterData(etSyncSpec(), etD1, []*cluster.DB{
			etMaster(etD1, etK1, 0, etD2, etD3, etD4),
			etStandby(etD2, etK2, etD1, 0),
			etStandby(etD3, etK3, etD1, 0),
			etStandby(etD4, etK4, etD1, 0),
		})
		goodStandbys := map[string]*cluster.DB{}
		for _, uid := range good {
			goodStandbys[uid] = cd.DBs[uid]
		}
		for _, uid := range missing {
			delete(cd.DBs, uid)
		}
		synchronousStandbys := set(sync...)
		s := etSentinel(cd)
		s.pruneSynchronousStandbys(quietCtx(), cd, cd.DBs[etD1], goodStandbys, synchronousStandbys, maxSync)
		return synchronousStandbys
	}

	DescribeTable("the synchronous standbys it keeps",
		func(sync, good, missing []string, maxSync int, want []string) {
			Ω(prune(sync, good, missing, maxSync)).To(Equal(set(want...)))
		},
		Entry("keeps good standbys up to the maximum",
			[]string{etD2, etD3}, []string{etD2, etD3, etD4}, nil, 2, []string{etD2, etD3}),
		Entry("removes a standby that no longer exists",
			[]string{etD2, etD3}, []string{etD2, etD3}, []string{etD3}, 2, []string{etD2}),
		Entry("removes a standby that no longer exists even when it was a good one",
			[]string{etD2, etD3}, []string{etD2, etD3}, []string{etD2}, 2, []string{etD3}),
		Entry("removes a standby that isn't good",
			[]string{etD2, etD3}, []string{etD2}, nil, 2, []string{etD2}),
		Entry("removes the failed and missing standbys before counting the excess",
			[]string{etD2, etD3, etD4}, []string{etD2, etD3}, []string{etD3}, 1, []string{etD2}),
		Entry("removes all of them when there are no good standbys",
			[]string{etD2, etD3}, nil, nil, 2, []string{}),
		Entry("removes all of them when the maximum is zero",
			[]string{etD2, etD3}, []string{etD2, etD3}, nil, 0, []string{}),
		Entry("leaves an empty set empty",
			nil, []string{etD2, etD3}, nil, 1, []string{}),
		Entry("doesn't add good standbys that aren't synchronous",
			[]string{etD2}, []string{etD2, etD3, etD4}, nil, 3, []string{etD2}),
	)

	It("removes any of the good standbys in excess of the maximum", func() {
		sync := []string{etD2, etD3, etD4}
		got := prune(sync, sync, nil, 1)
		Ω(got).To(HaveLen(1))
		for uid := range got {
			Ω(sync).To(ContainElement(uid))
		}
	})
})

var _ = Describe("updateProxySpec", func() {
	const (
		px1 = "proxy1"
		px2 = "proxy2"
		px3 = "proxy3"
	)

	// proxies returns the infos of the proxies uids, at generation gen
	proxies := func(gen int64, uids ...string) cluster.ProxiesInfo {
		pis := cluster.ProxiesInfo{}
		for _, uid := range uids {
			pis[uid] = &cluster.ProxyInfo{UID: uid, Generation: gen}
		}
		return pis
	}

	// update runs updateProxySpec, with new master db2, on a cluster with a proxy spec of masterDBUID and enabled
	// proxies at generation etGen2, and returns the proxy
	update := func(masterDBUID string, enabled []string, pis cluster.ProxiesInfo) *cluster.Proxy {
		cd := etClusterData(etSyncSpec(), etD1, []*cluster.DB{etMaster(etD1, etK1, 0)})
		cd.Proxy.Generation = etGen2
		cd.Proxy.Spec = cluster.ProxySpec{MasterDBUID: masterDBUID, EnabledProxies: enabled}
		etSentinel(cd).updateProxySpec(quietCtx(), cd, pis, etD2)
		return cd.Proxy
	}

	// expectProxy asserts the proxy spec and generation
	expectProxy := func(proxy *cluster.Proxy, masterDBUID string, enabled []string, gen int64) {
		GinkgoHelper()
		Ω(proxy.Spec.MasterDBUID).To(Equal(masterDBUID))
		Ω(proxy.Spec.EnabledProxies).To(Equal(enabled))
		Ω(proxy.Generation).To(Equal(gen))
	}

	Context("without a master in the proxy spec, as after a switch of the master", func() {
		DescribeTable("points the proxies to the new master once they all converged",
			func(pis cluster.ProxiesInfo) {
				expectProxy(update("", []string{px1}, pis), etD2, []string{px1}, etGen2+1)
			},
			Entry("when all proxies are at the current generation", proxies(etGen2, px1, px2)),
			Entry("when there are no proxies, as they're assumed to have closed their connections", proxies(etGen2)),
		)

		It("waits for a proxy that isn't at the current generation", func() {
			pis := proxies(etGen2, px1, px2)
			pis[px2].Generation = etGen1
			expectProxy(update("", []string{px1, px2}, pis), "", []string{px1, px2}, etGen2)
		})
	})

	Context("with a master in the proxy spec", func() {
		DescribeTable("enables the proxies that are available",
			func(enabled []string, pis cluster.ProxiesInfo, wantEnabled []string, wantGen int64) {
				expectProxy(update(etD1, enabled, pis), etD1, wantEnabled, wantGen)
			},
			Entry("enables new proxies, sorted, and bumps the generation",
				[]string{px2}, proxies(etGen2, px3, px1, px2), []string{px1, px2, px3}, int64(etGen2+1)),
			Entry("disables proxies that went away and bumps the generation",
				[]string{px1, px2, px3}, proxies(etGen2, px2), []string{px2}, int64(etGen2+1)),
			Entry("disables all proxies when none are left",
				[]string{px1}, proxies(etGen2), []string{}, int64(etGen2+1)),
			Entry("keeps the generation when the enabled proxies don't change",
				[]string{px1, px2}, proxies(etGen2, px2, px1), []string{px1, px2}, int64(etGen2)),
			Entry("keeps the generation without proxies when none were enabled",
				[]string{}, proxies(etGen2), []string{}, int64(etGen2)),
			Entry("enables proxies that haven't converged yet",
				[]string{}, proxies(etGen1, px1), []string{px1}, int64(etGen2+1)),
			// nil and an empty list differ for reflect.DeepEqual, so a nil list bumps the generation once
			Entry("bumps the generation once to replace no enabled proxies with an empty list",
				nil, proxies(etGen2), []string{}, int64(etGen2+1)),
		)

		It("keeps the master in the proxy spec, also when another master is passed", func() {
			expectProxy(update(etD1, []string{px1}, proxies(etGen2, px1)), etD1, []string{px1}, etGen2)
		})
	})
})

var _ = Describe("removeUnsyncableDBs", func() {
	const (
		// the master at WAL segment 9, keeping the WAL files from segment 7 on (timeline 1, 16MB segments)
		masterPos     uint64 = 9 << 24
		behindPos     uint64 = 5 << 24
		olderWal7            = "000000010000000000000007"
		stuckForTimes        = cluster.DefaultDBNotIncreasingXLogPosTimes + 1
	)

	// standby returns a standby of db1 at xlogPos that is past its initial generation (dbCanSync skips initializing
	// dbs)
	standby := func(uid, keeperUID string, xlogPos uint64) *cluster.DB {
		db := etStandby(uid, keeperUID, etD1, xlogPos)
		db.Generation = cluster.InitialGeneration + 1
		return db
	}

	// clusterData returns master db1, standbys db2 and db3 that are behind the oldest WAL file of the master, and
	// standby db4 at the position of the master
	clusterData := func(olderWal string) *cluster.Data {
		master := etMaster(etD1, etK1, masterPos, etD2, etD3, etD4)
		master.Status.OlderWalFile = olderWal
		return etClusterData(etSyncSpec(), etD1, []*cluster.DB{
			master,
			standby(etD2, etK2, behindPos),
			standby(etD3, etK3, behindPos),
			standby(etD4, "keeper4", masterPos),
		})
	}

	DescribeTable("the dbs it keeps",
		func(olderWal string, stuck []string, change func(cd, newcd *cluster.Data), want []string) {
			cd := clusterData(olderWal)
			newcd := cd.DeepCopy()
			if change != nil {
				change(cd, newcd)
			}
			s := etSentinel(cd)
			for _, uid := range stuck {
				s.dbNotIncreasingXLogPos[uid] = stuckForTimes
			}
			s.removeUnsyncableDBs(quietCtx(), cd, newcd)
			Ω(newcd.DBs).To(HaveLen(len(want)))
			for _, uid := range want {
				Ω(newcd.DBs).To(HaveKey(uid))
			}
		},
		Entry("removes a stuck standby that misses WAL files the master no longer has",
			olderWal7, []string{etD2}, nil, []string{etD1, etD3, etD4}),
		Entry("removes all the stuck standbys that miss WAL files",
			olderWal7, []string{etD2, etD3}, nil, []string{etD1, etD4}),
		Entry("keeps a stuck standby at the position of the master",
			olderWal7, []string{etD4}, nil, []string{etD1, etD2, etD3, etD4}),
		Entry("keeps the standbys whose position still increases",
			olderWal7, nil, nil, []string{etD1, etD2, etD3, etD4}),
		Entry("keeps all dbs when the master doesn't report its oldest WAL file",
			"", []string{etD2, etD3}, nil, []string{etD1, etD2, etD3, etD4}),
		Entry("keeps a stuck standby that is still initializing",
			olderWal7, []string{etD2}, func(cd, _ *cluster.Data) {
				cd.DBs[etD2].Generation = cluster.InitialGeneration
			}, []string{etD1, etD2, etD3, etD4}),
		Entry("keeps a stuck standby that isn't healthy",
			olderWal7, []string{etD2}, func(cd, _ *cluster.Data) {
				cd.DBs[etD2].Status.Healthy = false
			}, []string{etD1, etD2, etD3, etD4}),
		Entry("judges the dbs by their state in the current cluster data, not the new one",
			olderWal7, []string{etD2}, func(_, newcd *cluster.Data) {
				newcd.DBs[etD2].Status.Healthy = false
			}, []string{etD1, etD3, etD4}),
		Entry("leaves dbs that were already removed from the new cluster data out",
			olderWal7, nil, func(_, newcd *cluster.Data) {
				delete(newcd.DBs, etD3)
			}, []string{etD1, etD2, etD4}),
	)

	It("panics on a db in the new cluster data that the current one doesn't have", func() {
		cd := clusterData(olderWal7)
		newcd := cd.DeepCopy()
		newcd.DBs["db5"] = standby("db5", "keeper5", behindPos)
		s := etSentinel(cd)
		Ω(func() { s.removeUnsyncableDBs(quietCtx(), cd, newcd) }).To(Panic())
	})
})
