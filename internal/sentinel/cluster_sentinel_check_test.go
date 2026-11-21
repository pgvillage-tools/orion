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
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/consensus"
)

// checkStore is a consensus.Store for clusterSentinelCheck: it serves cd (with pair) and the keepers and proxies info,
// fails the calls with an error set, and records what the sentinel writes. It is the only declaration outside the
// Describe: Go doesn't allow methods on a type declared in a function.
type checkStore struct {
	consensus.Store

	cd          *cluster.Data
	pair        *consensus.KVPair
	keepersInfo cluster.KeepersInfo
	proxiesInfo cluster.ProxiesInfo

	getCDErr, putCDErr, sentinelInfoErr, keepersInfoErr, proxiesInfoErr error

	// the cluster data written with AtomicPutClusterData, and the previous pair it was written with
	puts     []*cluster.Data
	previous []*consensus.KVPair
	// the sentinel info set, and its ttl
	sentinelInfos []*cluster.SentinelInfo
	sentinelTTL   time.Duration
	// whether the keepers and proxies info were read
	keepersRead, proxiesRead bool
}

func (c *checkStore) GetClusterData(context.Context) (*cluster.Data, *consensus.KVPair, error) {
	return c.cd, c.pair, c.getCDErr
}

func (c *checkStore) AtomicPutClusterData(_ context.Context, cd *cluster.Data, previous *consensus.KVPair) (
	*consensus.KVPair, error) {
	c.puts = append(c.puts, cd)
	c.previous = append(c.previous, previous)
	return &consensus.KVPair{}, c.putCDErr
}

func (c *checkStore) SetSentinelInfo(_ context.Context, si *cluster.SentinelInfo, ttl time.Duration) error {
	c.sentinelInfos = append(c.sentinelInfos, si)
	c.sentinelTTL = ttl
	return c.sentinelInfoErr
}

func (c *checkStore) GetKeepersInfo(context.Context) (cluster.KeepersInfo, error) {
	c.keepersRead = true
	return c.keepersInfo, c.keepersInfoErr
}

func (c *checkStore) GetProxiesInfo(context.Context) (cluster.ProxiesInfo, error) {
	c.proxiesRead = true
	return c.proxiesInfo, c.proxiesInfoErr
}

var _ = Describe("clusterSentinelCheck", func() {
	var (
		ctx   context.Context
		store *checkStore
		s     *Sentinel
	)
	errStore := errors.New("store error")

	// checkCD returns the cluster data of a converged cluster: db1 on keeper1 is its primary
	checkCD := func() *cluster.Data {
		cd := etClusterData(initSpec(cluster.New), "db1", []*cluster.DB{etMaster("db1", "keeper1", 0)})
		cd.FormatVersion = cluster.CurrentCDFormatVersion
		return cd
	}

	BeforeEach(func() {
		ctx = quietCtx()
		store = &checkStore{cd: checkCD(), pair: &consensus.KVPair{LastIndex: 7}}
		s = etSentinel(store.cd)
		s.e = store
		s.sleepInterval = time.Hour
	})

	// lead makes the sentinel the leader, for the leadershipCount-th time
	lead := func(leadershipCount uint) {
		s.leader = true
		s.leadershipCount = leadershipCount
	}

	Context("when it can't use the cluster data", func() {
		It("stops when the cluster data can't be read", func() {
			store.getCDErr = errStore
			s.clusterSentinelCheck(ctx)
			Ω(store.sentinelInfos).To(BeEmpty())
			Ω(store.puts).To(BeEmpty())
		})

		It("stops on an unsupported format version", func() {
			store.cd.FormatVersion = cluster.CurrentCDFormatVersion + 1
			s.clusterSentinelCheck(ctx)
			Ω(store.sentinelInfos).To(BeEmpty())
		})

		It("stops on an invalid spec", func() {
			store.cd.Cluster.Spec.SleepInterval = &cluster.Duration{Duration: -time.Second}
			s.clusterSentinelCheck(ctx)
			Ω(store.sentinelInfos).To(BeEmpty())
		})
	})

	Context("without cluster data", func() {
		BeforeEach(func() { store.cd, store.pair = nil, nil })

		It("waits for it without an initial cluster spec", func() {
			s.clusterSentinelCheck(ctx)
			Ω(store.puts).To(BeEmpty())
			Ω(store.sentinelInfos).To(BeEmpty())
		})

		It("writes a new cluster from the initial cluster spec, only when there still is none", func() {
			spec := initSpec(cluster.New)
			s.initialClusterSpec = spec
			s.clusterSentinelCheck(ctx)
			Ω(store.puts).To(HaveLen(1))
			Ω(store.previous[0]).To(BeNil())
			Ω(store.puts[0].Cluster.Spec).To(Equal(spec))
			Ω(store.puts[0].Cluster.Status.Phase).To(Equal(cluster.Initializing))
			// it waits for the next check before it does more
			Ω(store.sentinelInfos).To(BeEmpty())
		})

		It("survives a failing write of the new cluster", func() {
			s.initialClusterSpec = initSpec(cluster.New)
			store.putCDErr = errStore
			s.clusterSentinelCheck(ctx)
			Ω(store.puts).To(HaveLen(1))
		})
	})

	Context("with cluster data", func() {
		It("takes its check interval and request timeout from the spec", func() {
			store.cd.Cluster.Spec.SleepInterval = &cluster.Duration{Duration: 3 * time.Second}
			store.cd.Cluster.Spec.RequestTimeout = &cluster.Duration{Duration: 4 * time.Second}
			s.clusterSentinelCheck(ctx)
			Ω(s.sleepInterval).To(Equal(3 * time.Second))
			Ω(s.requestTimeout).To(Equal(4 * time.Second))
		})

		It("publishes its info for two check intervals", func() {
			s.clusterSentinelCheck(ctx)
			Ω(store.sentinelInfos).To(ConsistOf(&cluster.SentinelInfo{UID: "sentinel01"}))
			Ω(store.sentinelTTL).To(Equal(2 * s.sleepInterval))
		})

		DescribeTable("stops when the store fails",
			func(fail func(), keepersRead, proxiesRead bool) {
				lead(1)
				fail()
				s.clusterSentinelCheck(ctx)
				Ω(store.keepersRead).To(Equal(keepersRead))
				Ω(store.proxiesRead).To(Equal(proxiesRead))
				Ω(store.puts).To(BeEmpty())
			},
			Entry("to set its info", func() { store.sentinelInfoErr = errStore }, false, false),
			Entry("to get the keepers info", func() { store.keepersInfoErr = errStore }, true, false),
			Entry("to get the proxies info", func() { store.proxiesInfoErr = errStore }, true, true),
		)

		It("doesn't change the cluster when it isn't the leader", func() {
			s.clusterSentinelCheck(ctx)
			Ω(store.keepersRead).To(BeTrue())
			Ω(store.puts).To(BeEmpty())
		})
	})

	Context("as the leader", func() {
		It("writes the new cluster data over the cluster data it read", func() {
			lead(1)
			s.clusterSentinelCheck(ctx)
			Ω(store.puts).To(HaveLen(1))
			Ω(store.previous[0]).To(Equal(store.pair))
			Ω(store.puts[0].ChangeTime).NotTo(BeZero())
			Ω(s.dbConvergenceInfos).To(HaveKey("db1"))
		})

		It("resets its timers on the first check after (re)gaining the leadership only", func() {
			lead(1)
			s.keeperErrorTimers["keeper1"] = 1
			s.clusterSentinelCheck(ctx)
			Ω(s.lastLeadershipCount).To(BeEquivalentTo(1))
			Ω(s.keeperErrorTimers).NotTo(HaveKeyWithValue("keeper1", int64(1)))

			// the next check of the same leadership keeps them
			s.keeperErrorTimers["keeper1"] = 1
			s.clusterSentinelCheck(ctx)
			Ω(s.keeperErrorTimers).To(HaveKeyWithValue("keeper1", int64(1)))

			// leading again (after losing it) resets them
			lead(2)
			s.clusterSentinelCheck(ctx)
			Ω(s.lastLeadershipCount).To(BeEquivalentTo(2))
			Ω(s.keeperErrorTimers).NotTo(HaveKeyWithValue("keeper1", int64(1)))
		})

		It("keeps the info of the keepers of its cluster it has seen", func() {
			lead(1)
			store.keepersInfo = cluster.KeepersInfo{
				"keeper1": {UID: "keeper1", InfoUID: "info1", ClusterUID: etCluster},
				"other":   {UID: "other", InfoUID: "info2", ClusterUID: "another cluster"},
			}
			s.clusterSentinelCheck(ctx)
			Ω(s.keeperInfoHistories).To(HaveKey("keeper1"))
			Ω(s.keeperInfoHistories).NotTo(HaveKey("other"))
		})

		It("doesn't write when it can't update the cluster", func() {
			lead(1)
			// a new cluster without keepers: there is no keeper to initialize it
			store.cd = etClusterData(initSpec(cluster.New), "", nil)
			store.cd.FormatVersion = cluster.CurrentCDFormatVersion
			store.cd.Cluster.Status.Phase = cluster.Initializing
			s.clusterSentinelCheck(ctx)
			Ω(store.puts).To(BeEmpty())
		})

		It("survives a failing write of the new cluster data", func() {
			lead(1)
			store.putCDErr = errStore
			s.clusterSentinelCheck(ctx)
			Ω(store.puts).To(HaveLen(1))
		})
	})
})
