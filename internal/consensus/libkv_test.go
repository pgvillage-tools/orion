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

package consensus

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	libkvstore "github.com/kvtools/valkeyrie/store"
)

func TestFromLibKVStoreErr(t *testing.T) {
	other := errors.New("other")
	for in, expected := range map[error]error{
		libkvstore.ErrKeyNotFound: ErrKeyNotFound,
		libkvstore.ErrKeyModified: ErrKeyModified,
		// create only (AtomicPut without a previous pair) on an existing key
		libkvstore.ErrKeyExists:                            ErrKeyModified,
		fmt.Errorf("wrapped: %w", libkvstore.ErrKeyExists): ErrKeyModified,
		other: other,
		nil:   nil,
	} {
		if got := fromLibKVStoreErr(in); !errors.Is(got, expected) && got != expected {
			t.Errorf("%v: expected %v, got %v", in, expected, got)
		}
	}
}

// fakeLibKV is an in-memory valkeyrie store. The err fields make the matching operation fail.
// Only the methods used by libKVStore and Candidate are implemented.
type fakeLibKV struct {
	mu    sync.Mutex
	pairs map[string]*libkvstore.KVPair
	index uint64

	putErr, getErr, listErr, atomicPutErr, deleteErr, closeErr, newLockErr error
	// lastPutOptions and lastPrevious record the arguments of the last (atomic) put
	lastPutOptions *libkvstore.WriteOptions
	lastPrevious   *libkvstore.KVPair
	// lock is returned by NewLock, lockOptions records its options
	lock        libkvstore.Locker
	lockOptions []*libkvstore.LockOptions
}

func newFakeLibKV() *fakeLibKV {
	return &fakeLibKV{pairs: map[string]*libkvstore.KVPair{}}
}

func (f *fakeLibKV) write(key string, value []byte, opts *libkvstore.WriteOptions) *libkvstore.KVPair {
	f.index++
	p := &libkvstore.KVPair{Key: key, Value: value, LastIndex: f.index}
	f.pairs[key] = p
	f.lastPutOptions = opts
	return p
}

func (f *fakeLibKV) Put(_ context.Context, key string, value []byte, opts *libkvstore.WriteOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.putErr != nil {
		return f.putErr
	}
	f.write(key, value, opts)
	return nil
}

func (f *fakeLibKV) Get(_ context.Context, key string, _ *libkvstore.ReadOptions) (*libkvstore.KVPair, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	p, ok := f.pairs[key]
	if !ok {
		return nil, libkvstore.ErrKeyNotFound
	}
	return p, nil
}

func (f *fakeLibKV) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if _, ok := f.pairs[key]; !ok {
		return libkvstore.ErrKeyNotFound
	}
	delete(f.pairs, key)
	return nil
}

func (f *fakeLibKV) Exists(context.Context, string, *libkvstore.ReadOptions) (bool, error) {
	panic("not implemented")
}

func (f *fakeLibKV) Watch(context.Context, string, *libkvstore.ReadOptions) (<-chan *libkvstore.KVPair, error) {
	panic("not implemented")
}

func (f *fakeLibKV) WatchTree(context.Context, string, *libkvstore.ReadOptions) (<-chan []*libkvstore.KVPair, error) {
	panic("not implemented")
}

func (f *fakeLibKV) NewLock(_ context.Context, _ string, opts *libkvstore.LockOptions) (libkvstore.Locker, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lockOptions = append(f.lockOptions, opts)
	if f.newLockErr != nil {
		return nil, f.newLockErr
	}
	return f.lock, nil
}

func (f *fakeLibKV) List(_ context.Context, directory string, _ *libkvstore.ReadOptions) ([]*libkvstore.KVPair, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	var pairs []*libkvstore.KVPair
	for k, p := range f.pairs {
		if strings.HasPrefix(k, directory) {
			pairs = append(pairs, p)
		}
	}
	if len(pairs) == 0 {
		return nil, libkvstore.ErrKeyNotFound
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Key < pairs[j].Key })
	return pairs, nil
}

func (f *fakeLibKV) DeleteTree(context.Context, string) error {
	panic("not implemented")
}

func (f *fakeLibKV) AtomicPut(
	_ context.Context,
	key string,
	value []byte,
	previous *libkvstore.KVPair,
	opts *libkvstore.WriteOptions,
) (bool, *libkvstore.KVPair, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastPrevious = previous
	if f.atomicPutErr != nil {
		return false, nil, f.atomicPutErr
	}
	cur, exists := f.pairs[key]
	switch {
	case previous == nil && exists:
		return false, nil, libkvstore.ErrKeyExists
	case previous != nil && !exists:
		return false, nil, libkvstore.ErrKeyNotFound
	case previous != nil && cur.LastIndex != previous.LastIndex:
		return false, nil, libkvstore.ErrKeyModified
	}
	return true, f.write(key, value, opts), nil
}

func (f *fakeLibKV) AtomicDelete(context.Context, string, *libkvstore.KVPair) (bool, error) {
	panic("not implemented")
}

func (f *fakeLibKV) Close() error { return f.closeErr }

func TestLibKVStorePutGet(t *testing.T) {
	ctx := context.Background()
	f := newFakeLibKV()
	s := &libKVStore{store: f}

	if _, err := s.Get(ctx, "/a"); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}

	if err := s.Put(ctx, "/a", []byte("1"), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.lastPutOptions != nil {
		t.Errorf("expected no write options, got %v", f.lastPutOptions)
	}
	if err := s.Put(ctx, "/b", []byte("2"), &WriteOptions{TTL: time.Minute}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.lastPutOptions == nil || f.lastPutOptions.TTL != time.Minute {
		t.Errorf("expected a ttl of a minute, got %v", f.lastPutOptions)
	}

	pair, err := s.Get(ctx, "/a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pair.Key != "/a" || string(pair.Value) != "1" || pair.LastIndex != 1 {
		t.Errorf("unexpected pair %v", pair)
	}

	f.putErr = libkvstore.ErrKeyModified
	if err = s.Put(ctx, "/a", nil, nil); !errors.Is(err, ErrKeyModified) {
		t.Errorf("expected ErrKeyModified, got %v", err)
	}
	f.getErr = errBoom
	if _, err = s.Get(ctx, "/a"); !errors.Is(err, errBoom) {
		t.Errorf("expected errBoom, got %v", err)
	}
}

func TestLibKVStoreList(t *testing.T) {
	ctx := context.Background()
	f := newFakeLibKV()
	s := &libKVStore{store: f}

	if _, err := s.List(ctx, "/dir/"); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
	for _, k := range []string{"/dir/a", "/dir/b", "/other"} {
		if err := s.Put(ctx, k, []byte(k), nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	pairs, err := s.List(ctx, "/dir/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pairs) != 2 || pairs[0].Key != "/dir/a" || string(pairs[1].Value) != "/dir/b" || pairs[1].LastIndex != 2 {
		t.Errorf("unexpected pairs %v", pairs)
	}

	f.listErr = errBoom
	if _, err = s.List(ctx, "/dir/"); !errors.Is(err, errBoom) {
		t.Errorf("expected errBoom, got %v", err)
	}
}

func TestLibKVStoreAtomicPut(t *testing.T) {
	ctx := context.Background()
	f := newFakeLibKV()
	s := &libKVStore{store: f}

	first, err := s.AtomicPut(ctx, "/a", []byte("1"), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.Key != "/a" || string(first.Value) != "1" || first.LastIndex != 1 {
		t.Errorf("unexpected pair %v", first)
	}
	if f.lastPrevious != nil || f.lastPutOptions != nil {
		t.Errorf("expected no previous pair and options, got %v, %v", f.lastPrevious, f.lastPutOptions)
	}

	// create only on an existing key
	if _, err = s.AtomicPut(ctx, "/a", []byte("2"), nil, nil); !errors.Is(err, ErrKeyModified) {
		t.Errorf("expected ErrKeyModified, got %v", err)
	}

	second, err := s.AtomicPut(ctx, "/a", []byte("2"), first, &WriteOptions{TTL: time.Minute})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second.LastIndex != 2 {
		t.Errorf("unexpected pair %v", second)
	}
	if f.lastPrevious.Key != "/a" || f.lastPrevious.LastIndex != 1 || f.lastPrevious.Value != nil {
		t.Errorf("expected the previous key and index only, got %v", f.lastPrevious)
	}
	if f.lastPutOptions == nil || f.lastPutOptions.TTL != time.Minute {
		t.Errorf("expected a ttl of a minute, got %v", f.lastPutOptions)
	}

	// stale previous pair
	if _, err = s.AtomicPut(ctx, "/a", []byte("3"), first, nil); !errors.Is(err, ErrKeyModified) {
		t.Errorf("expected ErrKeyModified, got %v", err)
	}
	// previous pair of a key that doesn't exist
	if _, err = s.AtomicPut(ctx, "/b", []byte("3"), first, nil); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestLibKVStoreDeleteClose(t *testing.T) {
	ctx := context.Background()
	f := newFakeLibKV()
	s := &libKVStore{store: f}

	if err := s.Delete(ctx, "/a"); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
	if err := s.Put(ctx, "/a", []byte("1"), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := s.Delete(ctx, "/a"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if _, ok := f.pairs["/a"]; ok {
		t.Error("expected /a to be deleted")
	}

	if err := s.Close(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	f.closeErr = errBoom
	if err := s.Close(); !errors.Is(err, errBoom) {
		t.Errorf("expected errBoom, got %v", err)
	}
}

func TestLibKVElectionLeader(t *testing.T) {
	f := newFakeLibKV()
	e := &libkvElection{store: &libKVStore{store: f}, path: "/c/leader"}

	// no leader yet
	if leader, err := e.Leader(t.Context()); err != nil || leader != "" {
		t.Errorf("expected no leader, got %q, %v", leader, err)
	}
	f.pairs["/c/leader"] = &libkvstore.KVPair{Key: "/c/leader", Value: []byte("keeper1")}
	if leader, err := e.Leader(t.Context()); err != nil || leader != "keeper1" {
		t.Errorf("expected leader keeper1, got %q, %v", leader, err)
	}
	f.getErr = errBoom
	if _, err := e.Leader(t.Context()); !errors.Is(err, errBoom) {
		t.Errorf("expected errBoom, got %v", err)
	}
}
