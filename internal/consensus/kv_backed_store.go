// Copyright 2026 PgVillage
// Copyright 2018 Sorint.lab
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
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/logging"
)

const (
	keepersInfoDir   = "/keepers/info/"
	clusterDataFile  = "clusterdata"
	sentinelsInfoDir = "/sentinels/info/"
	proxiesInfoDir   = "/proxies/info/"
)

// KVBackedStore defines a config store backed by a KV backend
type KVBackedStore struct {
	clusterPath string
	store       KVStore
}

// NewKVBackedStore returns a freshly initialized KVBackedStore
func NewKVBackedStore(kvStore KVStore, path string) *KVBackedStore {
	return &KVBackedStore{
		clusterPath: path,
		store:       kvStore,
	}
}

// Healthy verifies the KV backend is reachable and consistent by writing,
// reading back, and deleting a short-lived marker. It returns a non-nil
// error if any step fails or the round-tripped value does not match.
func (s *KVBackedStore) Healthy(ctx context.Context) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.StoreComponent)
	healthMsg := uuid.NewString()
	key := filepath.Join(s.clusterPath, "health", healthMsg)

	if err := s.store.Put(ctx, key, []byte(healthMsg), &WriteOptions{TTL: minTTL}); err != nil {
		logger.Debug().AnErr("error", err).Msg("health check: put failed")
		return err
	}
	// Best-effort cleanup regardless of subsequent outcome.
	defer func() {
		if err := s.store.Delete(ctx, key); err != nil {
			logger.Debug().AnErr("error", err).Msg("health check: cleanup delete failed")
		}
	}()

	pair, err := s.store.Get(ctx, key)
	if err != nil {
		logger.Debug().AnErr("error", err).Msg("health check: get failed")
		return err
	}
	if string(pair.Value) != healthMsg {
		logger.Debug().
			Bytes("got", pair.Value).
			Str("expected", healthMsg).
			Msg("health check: value mismatch")
		return errors.New("kv store health check value mismatch")
	}
	return nil
}

// AtomicPutClusterData is an atomic way to store CLusterData to a kv store
func (s *KVBackedStore) AtomicPutClusterData(ctx context.Context, cd *cluster.Data, previous *KVPair) (*KVPair, error) {
	cdj, err := json.Marshal(cd)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(s.clusterPath, clusterDataFile)
	// Skip prev Value since LastIndex is enough for a CAS and it gives
	// problem with etcd v2 api with big prev values.
	var prev *KVPair
	if previous != nil {
		prev = &KVPair{
			Key:       previous.Key,
			LastIndex: previous.LastIndex,
		}
	}
	return s.store.AtomicPut(ctx, path, cdj, prev, nil)
}

// PutClusterData stores ClusterData to a kv store
func (s *KVBackedStore) PutClusterData(ctx context.Context, cd *cluster.Data) error {
	cdj, err := json.Marshal(cd)
	if err != nil {
		return err
	}
	path := filepath.Join(s.clusterPath, clusterDataFile)
	return s.store.Put(ctx, path, cdj, nil)
}

// GetClusterData retrieves ClusterData from a kv store
func (s *KVBackedStore) GetClusterData(ctx context.Context) (*cluster.Data, *KVPair, error) {
	var cd *cluster.Data
	path := filepath.Join(s.clusterPath, clusterDataFile)
	pair, err := s.store.Get(ctx, path)
	if err != nil {
		if err != ErrKeyNotFound {
			return nil, nil, err
		}
		return nil, nil, nil
	}
	if err := json.Unmarshal(pair.Value, &cd); err != nil {
		return nil, nil, err
	}
	return cd, pair, nil
}

// putInfo stores the info of a member (keeper, sentinel or proxy) in dir, under its uid, with ttl (at least minTTL)
func (s *KVBackedStore) putInfo(ctx context.Context, dir, uid string, info any, ttl time.Duration) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	if ttl < minTTL {
		ttl = minTTL
	}
	return s.store.Put(ctx, filepath.Join(s.clusterPath, dir, uid), data, &WriteOptions{TTL: ttl})
}

// listInfos returns the infos of the members (keepers, sentinels or proxies) in dir; none when dir doesn't exist
func listInfos[T any](ctx context.Context, s *KVBackedStore, dir string) ([]*T, error) {
	pairs, err := s.store.List(ctx, filepath.Join(s.clusterPath, dir))
	if err != nil {
		if err != ErrKeyNotFound {
			return nil, err
		}
		return nil, nil
	}
	infos := make([]*T, 0, len(pairs))
	for _, pair := range pairs {
		var info T
		if err = json.Unmarshal(pair.Value, &info); err != nil {
			return nil, err
		}
		infos = append(infos, &info)
	}
	return infos, nil
}

// SetKeeperInfo stores info on a keeper to the kv store
func (s *KVBackedStore) SetKeeperInfo(ctx context.Context, id string, ms *cluster.KeeperInfo, ttl time.Duration) error {
	return s.putInfo(ctx, keepersInfoDir, id, ms, ttl)
}

// GetKeepersInfo retrieves all keeper info from a kv store
func (s *KVBackedStore) GetKeepersInfo(ctx context.Context) (cluster.KeepersInfo, error) {
	infos, err := listInfos[cluster.KeeperInfo](ctx, s, keepersInfoDir)
	if err != nil {
		return nil, err
	}
	keepers := cluster.KeepersInfo{}
	for _, ki := range infos {
		keepers[ki.UID] = ki
	}
	return keepers, nil
}

// SetSentinelInfo stores info on a sentinel to the kv store
func (s *KVBackedStore) SetSentinelInfo(ctx context.Context, si *cluster.SentinelInfo, ttl time.Duration) error {
	return s.putInfo(ctx, sentinelsInfoDir, si.UID, si, ttl)
}

// GetSentinelsInfo retrieves all sentinel info from a kv store
func (s *KVBackedStore) GetSentinelsInfo(ctx context.Context) (cluster.SentinelsInfo, error) {
	infos, err := listInfos[cluster.SentinelInfo](ctx, s, sentinelsInfoDir)
	if err != nil {
		return nil, err
	}
	return append(cluster.SentinelsInfo{}, infos...), nil
}

// SetProxyInfo stores info on a proxy to the kv store
func (s *KVBackedStore) SetProxyInfo(ctx context.Context, pi *cluster.ProxyInfo, ttl time.Duration) error {
	return s.putInfo(ctx, proxiesInfoDir, pi.UID, pi, ttl)
}

// GetProxiesInfo retrieves all proxy info from a kv store
func (s *KVBackedStore) GetProxiesInfo(ctx context.Context) (cluster.ProxiesInfo, error) {
	infos, err := listInfos[cluster.ProxyInfo](ctx, s, proxiesInfoDir)
	if err != nil {
		return nil, err
	}
	psi := cluster.ProxiesInfo{}
	for _, pi := range infos {
		psi[pi.UID] = pi
	}
	return psi, nil
}

// NewKVBackedElection starts a campaign for getting elected with a kv backed store
func NewKVBackedElection(kvStore KVStore, path, candidateUID string, timeout time.Duration) Election {
	switch kvStore := kvStore.(type) {
	case *libKVStore:
		s := kvStore
		candidate := NewCandidate(s.store, path, candidateUID, minTTL)
		return &libkvElection{store: s, path: path, candidate: candidate}
	case *etcdV3Store:
		etcdV3Store := kvStore
		return &etcdv3Election{
			c:              etcdV3Store.c,
			path:           path,
			candidateUID:   candidateUID,
			ttl:            minTTL,
			requestTimeout: timeout,
		}
	default:
		panic("unknown kvstore")
	}
}
