// Copyright 2026 PgVillage
// Copyright 2017 Sorint.lab
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

// Package store contains all code regarding consensus stores
package consensus

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"
	"time"

	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	etcdclientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

func fromEtcV3Error(err error) error {
	if errors.Is(err, rpctypes.ErrKeyNotFound) {
		return ErrKeyNotFound
	}
	if errors.Is(err, concurrency.ErrElectionNoLeader) {
		return ErrElectionNoLeader
	}
	return err
}

type etcdV3Store struct {
	c              *etcdclientv3.Client
	requestTimeout time.Duration
}

// newEtcdV3Store connects to etcd on addrs (host:port), with https when tlsConfig is set. timeout is the dial timeout,
// the timeout of every request, and the endpoint auto sync interval, as libkv used them.
func newEtcdV3Store(addrs []string, tlsConfig *tls.Config, timeout time.Duration) (*etcdV3Store, error) {
	scheme := "http"
	if tlsConfig != nil {
		scheme = "https"
	}
	endpoints := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		endpoints = append(endpoints, scheme+"://"+addr)
	}
	c, err := etcdclientv3.New(etcdclientv3.Config{
		Endpoints:        endpoints,
		TLS:              tlsConfig,
		DialTimeout:      timeout,
		AutoSyncInterval: timeout,
	})
	if err != nil {
		return nil, err
	}
	return &etcdV3Store{c: c, requestTimeout: timeout}, nil
}

// normalizeKey removes a leading / from key, as libkv did: the keys stay where libkv stored them
func normalizeKey(key string) string {
	return strings.TrimPrefix(key, "/")
}

func (s *etcdV3Store) Put(pctx context.Context, key string, value []byte, options *WriteOptions) error {
	etcdv3Options := []etcdclientv3.OpOption{}
	if options != nil {
		if options.TTL > 0 {
			ctx, cancel := context.WithTimeout(pctx, s.requestTimeout)
			lease, err := s.c.Grant(ctx, int64(options.TTL.Seconds()))
			cancel()
			if err != nil {
				return err
			}
			etcdv3Options = append(etcdv3Options, etcdclientv3.WithLease(lease.ID))
		}
	}
	ctx, cancel := context.WithTimeout(pctx, s.requestTimeout)
	_, err := s.c.Put(ctx, normalizeKey(key), string(value), etcdv3Options...)
	cancel()
	return fromEtcV3Error(err)
}

func (s *etcdV3Store) Get(pctx context.Context, key string) (*KVPair, error) {
	ctx, cancel := context.WithTimeout(pctx, s.requestTimeout)
	resp, err := s.c.Get(ctx, normalizeKey(key))
	cancel()
	if err != nil {
		return nil, fromEtcV3Error(err)
	}
	if len(resp.Kvs) == 0 {
		return nil, ErrKeyNotFound
	}
	kv := resp.Kvs[0]
	return &KVPair{Key: string(kv.Key), Value: kv.Value, LastIndex: uint64(kv.ModRevision)}, nil
}

func (s *etcdV3Store) List(pctx context.Context, directory string) ([]*KVPair, error) {
	ctx, cancel := context.WithTimeout(pctx, s.requestTimeout)
	resp, err := s.c.Get(ctx, normalizeKey(directory), etcdclientv3.WithPrefix())
	cancel()
	if err != nil {
		return nil, fromEtcV3Error(err)
	}
	kvPairs := make([]*KVPair, len(resp.Kvs))
	for i, kv := range resp.Kvs {
		kvPairs[i] = &KVPair{Key: string(kv.Key), Value: kv.Value, LastIndex: uint64(kv.ModRevision)}
	}
	return kvPairs, nil
}

func (s *etcdV3Store) AtomicPut(
	pctx context.Context,
	key string,
	value []byte,
	previous *KVPair,
	options *WriteOptions,
) (*KVPair, error) {
	etcdv3Options := []etcdclientv3.OpOption{}
	if options != nil {
		if options.TTL > 0 {
			ctx, cancel := context.WithTimeout(pctx, s.requestTimeout)
			lease, err := s.c.Grant(ctx, int64(options.TTL.Seconds()))
			cancel()
			if err != nil {
				return nil, err
			}
			etcdv3Options = append(etcdv3Options, etcdclientv3.WithLease(lease.ID))
		}
	}
	key = normalizeKey(key)
	var cmp etcdclientv3.Cmp
	if previous != nil {
		cmp = etcdclientv3.Compare(etcdclientv3.ModRevision(key), "=", int64(previous.LastIndex))
	} else {
		// key doens't exists
		cmp = etcdclientv3.Compare(etcdclientv3.CreateRevision(key), "=", 0)
	}
	ctx, cancel := context.WithTimeout(pctx, s.requestTimeout)
	txn := s.c.Txn(ctx).If(cmp)
	txn = txn.Then(etcdclientv3.OpPut(key, string(value), etcdv3Options...))
	tresp, err := txn.Commit()
	cancel()
	if err != nil {
		return nil, fromEtcV3Error(err)
	}
	if !tresp.Succeeded {
		return nil, ErrKeyModified
	}
	revision := tresp.Responses[0].GetResponsePut().Header.Revision
	return &KVPair{Key: key, Value: value, LastIndex: uint64(revision)}, nil
}

func (s *etcdV3Store) Delete(pctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(pctx, s.requestTimeout)
	_, err := s.c.Delete(ctx, normalizeKey(key))
	cancel()
	return fromEtcV3Error(err)
}

func (s *etcdV3Store) Close() error {
	return s.c.Close()
}
