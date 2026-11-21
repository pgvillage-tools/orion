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

package consensus

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cluster "github.com/pgvillage-tools/orion/api/v1"
)

// memKVStore is an in-memory KVStore. The err fields make the matching operation fail, and
// corruptGet makes Get return a different value than the one stored.
type memKVStore struct {
	mu    sync.Mutex
	pairs map[string]*KVPair
	index uint64
	// ttls records the TTL of the last write of each key
	ttls map[string]time.Duration

	putErr, getErr, listErr, atomicPutErr, deleteErr error
	corruptGet                                       bool
	deleted                                          []string
}

func newMemKVStore() *memKVStore {
	return &memKVStore{pairs: map[string]*KVPair{}, ttls: map[string]time.Duration{}}
}

func (m *memKVStore) write(key string, value []byte, options *WriteOptions) *KVPair {
	m.index++
	p := &KVPair{Key: key, Value: value, LastIndex: m.index}
	m.pairs[key] = p
	if options != nil {
		m.ttls[key] = options.TTL
	} else {
		delete(m.ttls, key)
	}
	return p
}

func (m *memKVStore) Put(_ context.Context, key string, value []byte, options *WriteOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.putErr != nil {
		return m.putErr
	}
	m.write(key, value, options)
	return nil
}

func (m *memKVStore) Get(_ context.Context, key string) (*KVPair, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return nil, m.getErr
	}
	p, ok := m.pairs[key]
	if !ok {
		return nil, ErrKeyNotFound
	}
	if m.corruptGet {
		return &KVPair{Key: p.Key, Value: []byte("corrupt"), LastIndex: p.LastIndex}, nil
	}
	return p, nil
}

func (m *memKVStore) List(_ context.Context, directory string) ([]*KVPair, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listErr != nil {
		return nil, m.listErr
	}
	var pairs []*KVPair
	for k, p := range m.pairs {
		if strings.HasPrefix(k, directory) {
			pairs = append(pairs, p)
		}
	}
	if len(pairs) == 0 {
		return nil, ErrKeyNotFound
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Key < pairs[j].Key })
	return pairs, nil
}

func (m *memKVStore) AtomicPut(
	_ context.Context,
	key string,
	value []byte,
	previous *KVPair,
	options *WriteOptions,
) (*KVPair, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.atomicPutErr != nil {
		return nil, m.atomicPutErr
	}
	cur, exists := m.pairs[key]
	switch {
	case previous == nil && exists:
		// create only, but the key exists
		return nil, ErrKeyModified
	case previous != nil && (!exists || cur.LastIndex != previous.LastIndex):
		return nil, ErrKeyModified
	}
	return m.write(key, value, options), nil
}

func (m *memKVStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleted = append(m.deleted, key)
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.pairs, key)
	return nil
}

func (m *memKVStore) Close() error { return nil }

var errBoom = errors.New("boom")

var _ = Describe("KVBackedStore", func() {
	var (
		ctx context.Context
		kv  *memKVStore
		s   *KVBackedStore
	)

	BeforeEach(func() {
		ctx = context.Background()
		kv = newMemKVStore()
		s = NewKVBackedStore(kv, "/c")
	})

	It("gets and (atomically) puts the cluster data", func() {
		s = NewKVBackedStore(kv, "/orion/cluster/test")

		// nothing stored yet: no data, no pair, no error
		cd, pair, err := s.GetClusterData(ctx)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(cd).Should(BeNil())
		Ω(pair).Should(BeNil())

		// create only
		first, err := s.AtomicPutClusterData(ctx, &cluster.Data{FormatVersion: 1}, nil)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(first.Key).Should(Equal("/orion/cluster/test/clusterdata"))
		// create only again: someone else wrote first
		_, err = s.AtomicPutClusterData(ctx, &cluster.Data{FormatVersion: 2}, nil)
		Ω(err).Should(MatchError(ErrKeyModified))

		cd, pair, err = s.GetClusterData(ctx)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(cd.FormatVersion).Should(Equal(uint64(1)))
		Ω(pair.LastIndex).Should(Equal(first.LastIndex))

		// compare and swap on the last index
		second, err := s.AtomicPutClusterData(ctx, &cluster.Data{FormatVersion: 2}, pair)
		Ω(err).ShouldNot(HaveOccurred())
		// a stale previous pair conflicts
		_, err = s.AtomicPutClusterData(ctx, &cluster.Data{FormatVersion: 3}, pair)
		Ω(err).Should(MatchError(ErrKeyModified))

		// an unconditional put always wins
		Ω(s.PutClusterData(ctx, &cluster.Data{FormatVersion: 4})).Should(Succeed())
		cd, pair, err = s.GetClusterData(ctx)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(cd.FormatVersion).Should(Equal(uint64(4)))
		Ω(pair.LastIndex).Should(BeNumerically(">", second.LastIndex))
	})

	It("returns the errors of the kv store for the cluster data", func() {
		kv.getErr = errBoom
		_, _, err := s.GetClusterData(ctx)
		Ω(err).Should(MatchError(errBoom))
		kv.getErr = nil

		kv.putErr = errBoom
		Ω(s.PutClusterData(ctx, &cluster.Data{})).Should(MatchError(errBoom))
		kv.putErr = nil

		kv.atomicPutErr = errBoom
		_, err = s.AtomicPutClusterData(ctx, &cluster.Data{}, nil)
		Ω(err).Should(MatchError(errBoom))
		kv.atomicPutErr = nil

		// invalid json in the store
		kv.pairs["/c/clusterdata"] = &KVPair{Key: "/c/clusterdata", Value: []byte("{")}
		_, _, err = s.GetClusterData(ctx)
		Ω(err).Should(HaveOccurred(), "expected a json error")
	})

	It("sets and gets the keepers info", func() {
		keepers, err := s.GetKeepersInfo(ctx)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(keepers).ShouldNot(BeNil())
		Ω(keepers).Should(BeEmpty())

		for _, uid := range []string{"k1", "k2"} {
			Ω(s.SetKeeperInfo(ctx, uid, &cluster.KeeperInfo{UID: uid}, time.Second)).Should(Succeed())
		}
		// the ttl is raised to the minimum
		Ω(kv.ttls["/c/keepers/info/k1"]).Should(Equal(minTTL))
		Ω(s.SetKeeperInfo(ctx, "k3", &cluster.KeeperInfo{UID: "k3"}, time.Minute)).Should(Succeed())
		Ω(kv.ttls["/c/keepers/info/k3"]).Should(Equal(time.Minute))

		keepers, err = s.GetKeepersInfo(ctx)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(keepers).Should(HaveLen(3))
		Ω(keepers["k1"].UID).Should(Equal("k1"))
		Ω(keepers["k3"].UID).Should(Equal("k3"))

		kv.pairs["/c/keepers/info/bad"] = &KVPair{Value: []byte("{")}
		_, err = s.GetKeepersInfo(ctx)
		Ω(err).Should(HaveOccurred(), "expected a json error")

		kv.listErr = errBoom
		_, err = s.GetKeepersInfo(ctx)
		Ω(err).Should(MatchError(errBoom))
	})

	It("sets and gets the sentinels info", func() {
		sentinels, err := s.GetSentinelsInfo(ctx)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(sentinels).ShouldNot(BeNil())
		Ω(sentinels).Should(BeEmpty())

		Ω(s.SetSentinelInfo(ctx, &cluster.SentinelInfo{UID: "s1"}, 0)).Should(Succeed())
		Ω(s.SetSentinelInfo(ctx, &cluster.SentinelInfo{UID: "s2"}, time.Minute)).Should(Succeed())
		Ω(kv.ttls["/c/sentinels/info/s1"]).Should(Equal(minTTL))
		Ω(kv.ttls["/c/sentinels/info/s2"]).Should(Equal(time.Minute))

		sentinels, err = s.GetSentinelsInfo(ctx)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(sentinels).Should(HaveLen(2))
		Ω(sentinels[0].UID).Should(Equal("s1"))
		Ω(sentinels[1].UID).Should(Equal("s2"))

		kv.pairs["/c/sentinels/info/bad"] = &KVPair{Value: []byte("{")}
		_, err = s.GetSentinelsInfo(ctx)
		Ω(err).Should(HaveOccurred(), "expected a json error")

		kv.listErr = errBoom
		_, err = s.GetSentinelsInfo(ctx)
		Ω(err).Should(MatchError(errBoom))
	})

	It("sets and gets the proxies info", func() {
		proxies, err := s.GetProxiesInfo(ctx)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(proxies).ShouldNot(BeNil())
		Ω(proxies).Should(BeEmpty())

		Ω(s.SetProxyInfo(ctx, &cluster.ProxyInfo{UID: "p1"}, 0)).Should(Succeed())
		Ω(s.SetProxyInfo(ctx, &cluster.ProxyInfo{UID: "p2", Generation: 2}, time.Minute)).Should(Succeed())
		Ω(kv.ttls["/c/proxies/info/p1"]).Should(Equal(minTTL))
		Ω(kv.ttls["/c/proxies/info/p2"]).Should(Equal(time.Minute))

		proxies, err = s.GetProxiesInfo(ctx)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(proxies).Should(HaveLen(2))
		Ω(proxies["p2"].Generation).Should(BeEquivalentTo(2))

		kv.pairs["/c/proxies/info/bad"] = &KVPair{Value: []byte("{")}
		_, err = s.GetProxiesInfo(ctx)
		Ω(err).Should(HaveOccurred(), "expected a json error")

		kv.listErr = errBoom
		_, err = s.GetProxiesInfo(ctx)
		Ω(err).Should(MatchError(errBoom))
	})

	// wantDelete is whether the health marker gets cleaned up
	DescribeTable("Healthy",
		func(setup func(*memKVStore), wantErr, wantDelete bool) {
			setup(kv)
			err := s.Healthy(ctx)
			if wantErr {
				Ω(err).Should(HaveOccurred())
			} else {
				Ω(err).ShouldNot(HaveOccurred())
			}
			if wantDelete {
				Ω(kv.deleted).Should(HaveLen(1))
			} else {
				Ω(kv.deleted).ShouldNot(HaveLen(1))
			}
			for _, key := range kv.deleted {
				Ω(key).Should(HavePrefix("/c/health/"), "unexpected health key")
			}
		},
		Entry("healthy", func(*memKVStore) {}, false, true),
		Entry("put fails", func(m *memKVStore) { m.putErr = errBoom }, true, false),
		Entry("get fails", func(m *memKVStore) { m.getErr = errBoom }, true, true),
		Entry("value mismatch", func(m *memKVStore) { m.corruptGet = true }, true, true),
		// a failing cleanup is only logged
		Entry("delete fails", func(m *memKVStore) { m.deleteErr = errBoom }, false, true),
	)
})

var _ = Describe("Config", func() {
	DescribeTable("endpoints",
		func(cfg Config, wantAddrs []string, wantScheme string, wantErr bool) {
			addrs, err := cfg.EndpointAddrs()
			if wantErr {
				Ω(err).Should(HaveOccurred())
			} else {
				Ω(err).ShouldNot(HaveOccurred())
			}
			Ω(strings.Join(addrs, ",")).Should(Equal(strings.Join(wantAddrs, ",")))
			scheme, err := cfg.EndpointScheme()
			if wantErr {
				Ω(err).Should(HaveOccurred())
			} else {
				Ω(err).ShouldNot(HaveOccurred())
			}
			Ω(scheme).Should(Equal(wantScheme))
		},
		Entry("etcd default", Config{Backend: ETCDV3}, []string{"127.0.0.1:2379"}, "http", false),
		Entry("schemeless", Config{Backend: ETCDV3, Endpoints: "a:1,b:2"}, []string{"a:1", "b:2"}, "http", false),
		Entry("https",
			Config{Backend: ETCDV3, Endpoints: "https://a:1,https://b:2"}, []string{"a:1", "b:2"}, "https", false),
		Entry("unknown backend", Config{Backend: "zookeeper"}, nil, "", true),
		Entry("consul", Config{Backend: "consul"}, nil, "", true),
		Entry("etcdv2", Config{Backend: "etcdv2"}, nil, "", true),
		Entry("mixed schemes", Config{Backend: ETCDV3, Endpoints: "https://a:1,http://b:2"}, nil, "", true),
		Entry("schemeless and https", Config{Backend: ETCDV3, Endpoints: "https://a:1,b:2"}, nil, "", true),
		Entry("unparsable", Config{Backend: ETCDV3, Endpoints: "http://a b:%zz"}, nil, "", true),
	)

	DescribeTable("TLSConfig",
		func(cfg Config, wantTLS, wantErr bool) {
			tlsConfig, err := cfg.TLSConfig()
			if wantErr {
				Ω(err).Should(HaveOccurred())
			} else {
				Ω(err).ShouldNot(HaveOccurred())
			}
			if wantTLS {
				Ω(tlsConfig).ShouldNot(BeNil())
			} else {
				Ω(tlsConfig).Should(BeNil())
			}
		},
		Entry("http", Config{Backend: ETCDV3}, false, false),
		Entry("https", Config{Backend: ETCDV3, Endpoints: "https://a:1"}, true, false),
		Entry("other scheme", Config{Backend: ETCDV3, Endpoints: "unix://a"}, false, true),
		Entry("invalid endpoints", Config{Backend: "zookeeper"}, false, true),
		Entry("missing ca file",
			Config{Backend: ETCDV3, Endpoints: "https://a:1", CAFile: "/nonexistent/ca.pem"}, false, true),
	)
})

var _ = Describe("NewKVStore", func() {
	It("creates orion's etcd v3 store", func() {
		// the client connects lazily, so no server is needed
		cfg := Config{Backend: ETCDV3, Endpoints: "https://127.0.0.1:1", Timeout: time.Second}
		s, err := NewKVStore(context.Background(), cfg)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(s).Should(BeAssignableToTypeOf(&etcdV3Store{}), "expected orion's etcd v3 store")
		es, _ := s.(*etcdV3Store)
		Ω(es.c.Endpoints()).Should(Equal([]string{"https://127.0.0.1:1"}))
		Ω(es.requestTimeout).Should(Equal(time.Second))
		Ω(s.Close()).Should(Succeed())
	})

	DescribeTable("fails for an invalid config",
		func(cfg Config) {
			_, err := NewKVStore(context.Background(), cfg)
			Ω(err).Should(HaveOccurred())
		},
		Entry("invalid endpoints", Config{Backend: ETCDV3, Endpoints: "https://a:1,http://b:2"}),
		Entry("invalid tls", Config{Backend: ETCDV3, Endpoints: "unix://a"}),
		Entry("unknown backend", Config{Backend: "zookeeper", Endpoints: "http://a:1"}),
		Entry("consul", Config{Backend: "consul", Endpoints: "http://a:1"}),
		Entry("etcdv2", Config{Backend: "etcdv2", Endpoints: "http://a:1"}),
	)
})

var _ = Describe("NewKVBackedElection", func() {
	It("creates an etcdv3 election for the etcd v3 store", func() {
		e, ok := NewKVBackedElection(&etcdV3Store{}, "/c/leader", "uid", time.Second).(*etcdv3Election)
		Ω(ok).Should(BeTrue(), "expected an etcdv3 election")
		Ω(e.path).Should(Equal("/c/leader"))
		Ω(e.candidateUID).Should(Equal("uid"))
		Ω(e.requestTimeout).Should(Equal(time.Second))
		Ω(e.ttl).Should(Equal(minTTL))
	})

	It("panics for an unknown kv store", func() {
		Ω(func() { NewKVBackedElection(newMemKVStore(), "/c/leader", "uid", time.Second) }).Should(Panic())
	})
})
