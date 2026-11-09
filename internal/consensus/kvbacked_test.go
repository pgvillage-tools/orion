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
	"testing"
	"time"

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

func TestKVBackedStoreClusterData(t *testing.T) {
	ctx := context.Background()
	kv := newMemKVStore()
	s := NewKVBackedStore(kv, "/orion/cluster/test")

	// nothing stored yet: no data, no pair, no error
	cd, pair, err := s.GetClusterData(ctx)
	if err != nil || cd != nil || pair != nil {
		t.Fatalf("expected no cluster data, got %v, %v, %v", cd, pair, err)
	}

	// create only
	first, err := s.AtomicPutClusterData(ctx, &cluster.Data{FormatVersion: 1}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.Key != "/orion/cluster/test/clusterdata" {
		t.Errorf("unexpected key %q", first.Key)
	}
	// create only again: someone else wrote first
	if _, err = s.AtomicPutClusterData(ctx, &cluster.Data{FormatVersion: 2}, nil); !errors.Is(err, ErrKeyModified) {
		t.Errorf("expected ErrKeyModified, got %v", err)
	}

	cd, pair, err = s.GetClusterData(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cd.FormatVersion != 1 || pair.LastIndex != first.LastIndex {
		t.Errorf("unexpected cluster data %v, pair %v", cd, pair)
	}

	// compare and swap on the last index
	second, err := s.AtomicPutClusterData(ctx, &cluster.Data{FormatVersion: 2}, pair)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// a stale previous pair conflicts
	if _, err = s.AtomicPutClusterData(ctx, &cluster.Data{FormatVersion: 3}, pair); !errors.Is(err, ErrKeyModified) {
		t.Errorf("expected ErrKeyModified, got %v", err)
	}

	// an unconditional put always wins
	if err = s.PutClusterData(ctx, &cluster.Data{FormatVersion: 4}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cd, pair, err = s.GetClusterData(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cd.FormatVersion != 4 || pair.LastIndex <= second.LastIndex {
		t.Errorf("unexpected cluster data %v, pair %v", cd, pair)
	}
}

func TestKVBackedStoreClusterDataErrors(t *testing.T) {
	ctx := context.Background()
	kv := newMemKVStore()
	s := NewKVBackedStore(kv, "/c")

	kv.getErr = errBoom
	if _, _, err := s.GetClusterData(ctx); !errors.Is(err, errBoom) {
		t.Errorf("expected errBoom, got %v", err)
	}
	kv.getErr = nil

	kv.putErr = errBoom
	if err := s.PutClusterData(ctx, &cluster.Data{}); !errors.Is(err, errBoom) {
		t.Errorf("expected errBoom, got %v", err)
	}
	kv.putErr = nil

	kv.atomicPutErr = errBoom
	if _, err := s.AtomicPutClusterData(ctx, &cluster.Data{}, nil); !errors.Is(err, errBoom) {
		t.Errorf("expected errBoom, got %v", err)
	}
	kv.atomicPutErr = nil

	// invalid json in the store
	kv.pairs["/c/clusterdata"] = &KVPair{Key: "/c/clusterdata", Value: []byte("{")}
	if _, _, err := s.GetClusterData(ctx); err == nil {
		t.Error("expected a json error")
	}
}

func TestKVBackedStoreKeepersInfo(t *testing.T) {
	ctx := context.Background()
	kv := newMemKVStore()
	s := NewKVBackedStore(kv, "/c")

	keepers, err := s.GetKeepersInfo(ctx)
	if err != nil || keepers == nil || len(keepers) != 0 {
		t.Fatalf("expected an empty keepers info, got %v, %v", keepers, err)
	}

	for _, uid := range []string{"k1", "k2"} {
		if err = s.SetKeeperInfo(ctx, uid, &cluster.KeeperInfo{UID: uid}, time.Second); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	// the ttl is raised to the minimum
	if ttl := kv.ttls["/c/keepers/info/k1"]; ttl != minTTL {
		t.Errorf("expected ttl %s, got %s", minTTL, ttl)
	}
	if err = s.SetKeeperInfo(ctx, "k3", &cluster.KeeperInfo{UID: "k3"}, time.Minute); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ttl := kv.ttls["/c/keepers/info/k3"]; ttl != time.Minute {
		t.Errorf("expected ttl %s, got %s", time.Minute, ttl)
	}

	keepers, err = s.GetKeepersInfo(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keepers) != 3 || keepers["k1"].UID != "k1" || keepers["k3"].UID != "k3" {
		t.Errorf("unexpected keepers info %v", keepers)
	}

	kv.pairs["/c/keepers/info/bad"] = &KVPair{Value: []byte("{")}
	if _, err = s.GetKeepersInfo(ctx); err == nil {
		t.Error("expected a json error")
	}

	kv.listErr = errBoom
	if _, err = s.GetKeepersInfo(ctx); !errors.Is(err, errBoom) {
		t.Errorf("expected errBoom, got %v", err)
	}
}

func TestKVBackedStoreSentinelsInfo(t *testing.T) {
	ctx := context.Background()
	kv := newMemKVStore()
	s := NewKVBackedStore(kv, "/c")

	sentinels, err := s.GetSentinelsInfo(ctx)
	if err != nil || sentinels == nil || len(sentinels) != 0 {
		t.Fatalf("expected an empty sentinels info, got %v, %v", sentinels, err)
	}

	if err = s.SetSentinelInfo(ctx, &cluster.SentinelInfo{UID: "s1"}, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err = s.SetSentinelInfo(ctx, &cluster.SentinelInfo{UID: "s2"}, time.Minute); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ttl := kv.ttls["/c/sentinels/info/s1"]; ttl != minTTL {
		t.Errorf("expected ttl %s, got %s", minTTL, ttl)
	}
	if ttl := kv.ttls["/c/sentinels/info/s2"]; ttl != time.Minute {
		t.Errorf("expected ttl %s, got %s", time.Minute, ttl)
	}

	sentinels, err = s.GetSentinelsInfo(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sentinels) != 2 || sentinels[0].UID != "s1" || sentinels[1].UID != "s2" {
		t.Errorf("unexpected sentinels info %v", sentinels)
	}

	kv.pairs["/c/sentinels/info/bad"] = &KVPair{Value: []byte("{")}
	if _, err = s.GetSentinelsInfo(ctx); err == nil {
		t.Error("expected a json error")
	}

	kv.listErr = errBoom
	if _, err = s.GetSentinelsInfo(ctx); !errors.Is(err, errBoom) {
		t.Errorf("expected errBoom, got %v", err)
	}
}

func TestKVBackedStoreProxiesInfo(t *testing.T) {
	ctx := context.Background()
	kv := newMemKVStore()
	s := NewKVBackedStore(kv, "/c")

	proxies, err := s.GetProxiesInfo(ctx)
	if err != nil || proxies == nil || len(proxies) != 0 {
		t.Fatalf("expected an empty proxies info, got %v, %v", proxies, err)
	}

	if err = s.SetProxyInfo(ctx, &cluster.ProxyInfo{UID: "p1"}, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err = s.SetProxyInfo(ctx, &cluster.ProxyInfo{UID: "p2", Generation: 2}, time.Minute); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ttl := kv.ttls["/c/proxies/info/p1"]; ttl != minTTL {
		t.Errorf("expected ttl %s, got %s", minTTL, ttl)
	}
	if ttl := kv.ttls["/c/proxies/info/p2"]; ttl != time.Minute {
		t.Errorf("expected ttl %s, got %s", time.Minute, ttl)
	}

	proxies, err = s.GetProxiesInfo(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(proxies) != 2 || proxies["p2"].Generation != 2 {
		t.Errorf("unexpected proxies info %v", proxies)
	}

	kv.pairs["/c/proxies/info/bad"] = &KVPair{Value: []byte("{")}
	if _, err = s.GetProxiesInfo(ctx); err == nil {
		t.Error("expected a json error")
	}

	kv.listErr = errBoom
	if _, err = s.GetProxiesInfo(ctx); !errors.Is(err, errBoom) {
		t.Errorf("expected errBoom, got %v", err)
	}
}

func TestKVBackedStoreHealthy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		setup   func(*memKVStore)
		wantErr bool
		// whether the marker gets cleaned up
		wantDelete bool
	}{
		{name: "healthy", setup: func(*memKVStore) {}, wantDelete: true},
		{name: "put fails", setup: func(m *memKVStore) { m.putErr = errBoom }, wantErr: true},
		{name: "get fails", setup: func(m *memKVStore) { m.getErr = errBoom }, wantErr: true, wantDelete: true},
		{name: "value mismatch", setup: func(m *memKVStore) { m.corruptGet = true }, wantErr: true, wantDelete: true},
		// a failing cleanup is only logged
		{name: "delete fails", setup: func(m *memKVStore) { m.deleteErr = errBoom }, wantDelete: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kv := newMemKVStore()
			tc.setup(kv)
			err := NewKVBackedStore(kv, "/c").Healthy(context.Background())
			if (err != nil) != tc.wantErr {
				t.Errorf("expected error %t, got %v", tc.wantErr, err)
			}
			if got := len(kv.deleted) == 1; got != tc.wantDelete {
				t.Errorf("expected delete %t, got deletes %v", tc.wantDelete, kv.deleted)
			}
			for _, key := range kv.deleted {
				if !strings.HasPrefix(key, "/c/health/") {
					t.Errorf("unexpected health key %q", key)
				}
			}
		})
	}
}

func TestConfigEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cfg        Config
		wantAddrs  []string
		wantScheme string
		wantErr    bool
	}{
		{
			name:       "etcd default",
			cfg:        Config{Backend: ETCDV3},
			wantAddrs:  []string{"127.0.0.1:2379"},
			wantScheme: "http",
		},
		{
			name:       "etcdv2 default",
			cfg:        Config{Backend: ETCDV2},
			wantAddrs:  []string{"127.0.0.1:2379"},
			wantScheme: "http",
		},
		{
			name:       "consul default",
			cfg:        Config{Backend: CONSUL},
			wantAddrs:  []string{"127.0.0.1:8500"},
			wantScheme: "http",
		},
		{
			name:       "schemeless",
			cfg:        Config{Backend: ETCDV3, Endpoints: "a:1,b:2"},
			wantAddrs:  []string{"a:1", "b:2"},
			wantScheme: "http",
		},
		{
			name:       "https",
			cfg:        Config{Backend: ETCDV3, Endpoints: "https://a:1,https://b:2"},
			wantAddrs:  []string{"a:1", "b:2"},
			wantScheme: "https",
		},
		{name: "unknown backend", cfg: Config{Backend: "zookeeper"}, wantErr: true},
		{name: "mixed schemes", cfg: Config{Backend: ETCDV3, Endpoints: "https://a:1,http://b:2"}, wantErr: true},
		{name: "schemeless and https", cfg: Config{Backend: ETCDV3, Endpoints: "https://a:1,b:2"}, wantErr: true},
		{name: "unparsable", cfg: Config{Backend: ETCDV3, Endpoints: "http://a b:%zz"}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			addrs, err := tc.cfg.EndpointAddrs()
			if (err != nil) != tc.wantErr {
				t.Fatalf("expected error %t, got %v", tc.wantErr, err)
			}
			if strings.Join(addrs, ",") != strings.Join(tc.wantAddrs, ",") {
				t.Errorf("expected addrs %v, got %v", tc.wantAddrs, addrs)
			}
			scheme, err := tc.cfg.EndpointScheme()
			if (err != nil) != tc.wantErr {
				t.Fatalf("expected error %t, got %v", tc.wantErr, err)
			}
			if scheme != tc.wantScheme {
				t.Errorf("expected scheme %q, got %q", tc.wantScheme, scheme)
			}
		})
	}
}

func TestConfigTLSConfig(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cfg     Config
		wantTLS bool
		wantErr bool
	}{
		{name: "http", cfg: Config{Backend: ETCDV3}},
		{name: "https", cfg: Config{Backend: ETCDV3, Endpoints: "https://a:1"}, wantTLS: true},
		{name: "other scheme", cfg: Config{Backend: ETCDV3, Endpoints: "unix://a"}, wantErr: true},
		{name: "invalid endpoints", cfg: Config{Backend: "zookeeper"}, wantErr: true},
		{
			name:    "missing ca file",
			cfg:     Config{Backend: ETCDV3, Endpoints: "https://a:1", CAFile: "/nonexistent/ca.pem"},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tlsConfig, err := tc.cfg.TLSConfig()
			if (err != nil) != tc.wantErr {
				t.Fatalf("expected error %t, got %v", tc.wantErr, err)
			}
			if (tlsConfig != nil) != tc.wantTLS {
				t.Errorf("expected tls config %t, got %v", tc.wantTLS, tlsConfig)
			}
		})
	}
}

func TestNewKVStore(t *testing.T) {
	// the clients connect lazily, so no server is needed
	for _, cfg := range []Config{
		{Backend: CONSUL, Endpoints: "http://127.0.0.1:1", Timeout: time.Second},
		{Backend: ETCDV2, Endpoints: "http://127.0.0.1:1", Timeout: time.Second},
		{Backend: ETCDV3, Endpoints: "https://127.0.0.1:1", Timeout: time.Second},
	} {
		t.Run(cfg.Backend.string(), func(t *testing.T) {
			s, err := NewKVStore(context.Background(), cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if _, ok := s.(*libKVStore); !ok {
				t.Errorf("expected a libkv store, got %T", s)
			}
			if err = s.Close(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestNewKVStoreErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  Config
	}{
		{name: "invalid endpoints", cfg: Config{Backend: ETCDV3, Endpoints: "https://a:1,http://b:2"}},
		{name: "invalid tls", cfg: Config{Backend: ETCDV3, Endpoints: "unix://a"}},
		{name: "unknown backend", cfg: Config{Backend: "zookeeper", Endpoints: "http://a:1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewKVStore(context.Background(), tc.cfg); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestNewKVBackedElection(t *testing.T) {
	lkv := &libKVStore{store: newFakeLibKV()}
	if e, ok := NewKVBackedElection(lkv, "/c/leader", "uid", time.Second).(*libkvElection); !ok ||
		e.path != "/c/leader" || e.candidate.node != "uid" {
		t.Errorf("unexpected libkv election %v", e)
	}

	ekv := &etcdV3Store{}
	if e, ok := NewKVBackedElection(ekv, "/c/leader", "uid", time.Second).(*etcdv3Election); !ok ||
		e.path != "/c/leader" || e.candidateUID != "uid" || e.requestTimeout != time.Second || e.ttl != minTTL {
		t.Errorf("unexpected etcdv3 election %v", e)
	}

	defer func() {
		if recover() == nil {
			t.Error("expected a panic for an unknown kv store")
		}
	}()
	NewKVBackedElection(newMemKVStore(), "/c/leader", "uid", time.Second)
}
