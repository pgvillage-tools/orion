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
	"fmt"
	"time"

	"github.com/kvtools/consul"
	"github.com/kvtools/etcdv2"
	"github.com/kvtools/etcdv3"
	"github.com/kvtools/valkeyrie"
	"github.com/pgvillage-tools/orion/internal/logging"
)

// TODO(sgotti) fix this in libkv?
// consul min ttl is 10s and libkv divides this by 2
const minTTL = 20 * time.Second

// KVPair represents {Key, Value, Lastindex} tuple
type KVPair struct {
	Key       string
	Value     []byte
	LastIndex uint64
}

// WriteOptions defines options to be used when writing to the backend
type WriteOptions struct {
	TTL time.Duration
}

// KVStore is an interface representing a backend (e.a. etcd, k8s en Consul)
type KVStore interface {
	// Put a value at the specified key
	Put(ctx context.Context, key string, value []byte, options *WriteOptions) error

	// Get a value given its key
	Get(ctx context.Context, key string) (*KVPair, error)

	// List the content of a given prefix
	List(ctx context.Context, directory string) ([]*KVPair, error)

	// Atomic CAS operation on a single value.
	// Pass previous = nil to create a new key.
	AtomicPut(ctx context.Context, key string, value []byte, previous *KVPair, options *WriteOptions) (*KVPair, error)

	Delete(ctx context.Context, key string) error

	// Close the store connection
	Close() error
}

// NewKVStore returns a freshly initialized KVStore
func NewKVStore(ctx context.Context, cfg Config) (KVStore, error) {
	ctx, logger := logging.GetLogComponent(ctx, logging.StoreComponent)
	addrs, err := cfg.EndpointAddrs()
	if err != nil {
		return nil, err
	}
	logger.Debug().Any("endpoints", addrs).Msg("")
	tlsConfig, err := cfg.TLSConfig()
	if err != nil {
		return nil, err
	}
	logger.Debug().Bool("tls enabled", tlsConfig != nil).Msg("")

	var config valkeyrie.Config
	switch cfg.Backend {
	case CONSUL:
		config = &consul.Config{
			TLS:               tlsConfig,
			ConnectionTimeout: cfg.Timeout,
		}
	case ETCDV2:
		config = &etcdv2.Config{
			TLS:               tlsConfig,
			ConnectionTimeout: cfg.Timeout,
		}

	case ETCDV3:
		config = &etcdv3.Config{
			TLS:               tlsConfig,
			ConnectionTimeout: cfg.Timeout,
			SyncPeriod:        cfg.Timeout,
		}
	default:
		return nil, fmt.Errorf("Unknown store backend: %q", cfg.Backend)
	}
	logger.Debug().Any("store config", config).Msg("")

	store, err := valkeyrie.NewStore(ctx, cfg.Backend.string(), addrs, config)
	if err != nil {
		return nil, err
	}
	return &libKVStore{store: store}, nil
}
