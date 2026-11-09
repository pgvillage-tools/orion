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
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
	"go.etcd.io/etcd/api/v3/mvccpb"
	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	etcdclientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// fakeEtcdKV is an in-memory etcd KV. It supports what etcdV3Store and the concurrency package
// use: put, (prefix) get with a limit and a max create revision, delete, and transactions comparing
// a create or mod revision. Ranges are always sorted on the create revision. The err fields make
// the matching operation fail, and getBlocks makes Get wait for the end of its context, like an unreachable etcd.
type fakeEtcdKV struct {
	etcdclientv3.KV

	mu  sync.Mutex
	kvs map[string]*mvccpb.KeyValue
	rev int64

	putErr, getErr, deleteErr, txnErr error
	getBlocks                         bool
}

func newFakeEtcdKV() *fakeEtcdKV {
	// like a fresh etcd, the first write gets revision 2
	return &fakeEtcdKV{kvs: map[string]*mvccpb.KeyValue{}, rev: 1}
}

func (f *fakeEtcdKV) header() *pb.ResponseHeader { return &pb.ResponseHeader{Revision: f.rev} }

func (f *fakeEtcdKV) put(key string, value []byte) {
	f.rev++
	kv, ok := f.kvs[key]
	if !ok {
		kv = &mvccpb.KeyValue{Key: []byte(key), CreateRevision: f.rev}
		f.kvs[key] = kv
	}
	kv.Value = value
	kv.ModRevision = f.rev
}

func (f *fakeEtcdKV) get(op etcdclientv3.Op) *pb.RangeResponse {
	key, end := string(op.KeyBytes()), string(op.RangeBytes())
	var kvs []*mvccpb.KeyValue
	for k, kv := range f.kvs {
		inRange := k == key || (end != "" && k >= key && k < end)
		if inRange && (op.MaxCreateRev() == 0 || kv.CreateRevision <= op.MaxCreateRev()) {
			kvs = append(kvs, kv)
		}
	}
	sort.Slice(kvs, func(i, j int) bool { return kvs[i].CreateRevision < kvs[j].CreateRevision })
	if op.Limit() > 0 && int64(len(kvs)) > op.Limit() {
		kvs = kvs[:op.Limit()]
	}
	return &pb.RangeResponse{Header: f.header(), Kvs: kvs, Count: int64(len(kvs))}
}

func (f *fakeEtcdKV) Put(
	_ context.Context,
	key, val string,
	_ ...etcdclientv3.OpOption,
) (*etcdclientv3.PutResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.putErr != nil {
		return nil, f.putErr
	}
	f.put(key, []byte(val))
	return &etcdclientv3.PutResponse{Header: f.header()}, nil
}

func (f *fakeEtcdKV) Get(
	ctx context.Context,
	key string,
	opts ...etcdclientv3.OpOption,
) (*etcdclientv3.GetResponse, error) {
	f.mu.Lock()
	if f.getBlocks {
		f.mu.Unlock()
		<-ctx.Done()
		return nil, ctx.Err()
	}
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	return (*etcdclientv3.GetResponse)(f.get(etcdclientv3.OpGet(key, opts...))), nil
}

func (f *fakeEtcdKV) Delete(
	_ context.Context,
	key string,
	_ ...etcdclientv3.OpOption,
) (*etcdclientv3.DeleteResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	delete(f.kvs, key)
	return &etcdclientv3.DeleteResponse{Header: f.header()}, nil
}

func (f *fakeEtcdKV) Txn(context.Context) etcdclientv3.Txn {
	return &fakeEtcdTxn{kv: f}
}

type fakeEtcdTxn struct {
	kv              *fakeEtcdKV
	cmps            []etcdclientv3.Cmp
	thenOps, elseOp []etcdclientv3.Op
}

func (t *fakeEtcdTxn) If(cs ...etcdclientv3.Cmp) etcdclientv3.Txn {
	t.cmps = cs
	return t
}

func (t *fakeEtcdTxn) Then(ops ...etcdclientv3.Op) etcdclientv3.Txn {
	t.thenOps = ops
	return t
}

func (t *fakeEtcdTxn) Else(ops ...etcdclientv3.Op) etcdclientv3.Txn {
	t.elseOp = ops
	return t
}

func (t *fakeEtcdTxn) Commit() (*etcdclientv3.TxnResponse, error) {
	f := t.kv
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.txnErr != nil {
		return nil, f.txnErr
	}
	succeeded := true
	for _, c := range t.cmps {
		cmp := c.GetCompare()
		var cur, expected int64
		kv := f.kvs[string(cmp.Key)]
		switch target := cmp.TargetUnion.(type) {
		case *pb.Compare_CreateRevision:
			if kv != nil {
				cur = kv.CreateRevision
			}
			expected = target.CreateRevision
		case *pb.Compare_ModRevision:
			if kv != nil {
				cur = kv.ModRevision
			}
			expected = target.ModRevision
		default:
			panic(fmt.Sprintf("unsupported compare target %T", target))
		}
		if cmp.Result != pb.Compare_EQUAL {
			panic("only equal compares are supported")
		}
		succeeded = succeeded && cur == expected
	}
	ops := t.thenOps
	if !succeeded {
		ops = t.elseOp
	}
	var responses []*pb.ResponseOp
	for _, op := range ops {
		switch {
		case op.IsPut():
			f.put(string(op.KeyBytes()), op.ValueBytes())
			responses = append(responses, &pb.ResponseOp{
				Response: &pb.ResponseOp_ResponsePut{ResponsePut: &pb.PutResponse{Header: f.header()}},
			})
		case op.IsGet():
			responses = append(responses, &pb.ResponseOp{
				Response: &pb.ResponseOp_ResponseRange{ResponseRange: f.get(op)},
			})
		default:
			panic("unsupported txn op")
		}
	}
	return &etcdclientv3.TxnResponse{Header: f.header(), Succeeded: succeeded, Responses: responses}, nil
}

// fakeEtcdLease grants leases (recording their ttl) that are kept alive until their context ends. grantBlocks makes
// Grant wait for the end of its context, like an unreachable etcd.
type fakeEtcdLease struct {
	etcdclientv3.Lease

	mu          sync.Mutex
	nextID      etcdclientv3.LeaseID
	ttls        []int64
	grantErr    error
	grantBlocks bool
	closed      bool
}

func (l *fakeEtcdLease) Grant(ctx context.Context, ttl int64) (*etcdclientv3.LeaseGrantResponse, error) {
	l.mu.Lock()
	if l.grantBlocks {
		l.mu.Unlock()
		<-ctx.Done()
		return nil, ctx.Err()
	}
	defer l.mu.Unlock()
	if l.grantErr != nil {
		return nil, l.grantErr
	}
	l.nextID++
	l.ttls = append(l.ttls, ttl)
	return &etcdclientv3.LeaseGrantResponse{ID: l.nextID, TTL: ttl}, nil
}

func (l *fakeEtcdLease) grantedTTLs() []int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]int64(nil), l.ttls...)
}

func (l *fakeEtcdLease) KeepAlive(
	ctx context.Context,
	_ etcdclientv3.LeaseID,
) (<-chan *etcdclientv3.LeaseKeepAliveResponse, error) {
	ch := make(chan *etcdclientv3.LeaseKeepAliveResponse)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func (l *fakeEtcdLease) Revoke(context.Context, etcdclientv3.LeaseID) (*etcdclientv3.LeaseRevokeResponse, error) {
	return &etcdclientv3.LeaseRevokeResponse{}, nil
}

func (l *fakeEtcdLease) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	return nil
}

func newFakeEtcdClient() (*etcdclientv3.Client, *fakeEtcdKV, *fakeEtcdLease) {
	c := etcdclientv3.NewCtxClient(context.Background())
	kv := newFakeEtcdKV()
	lease := &fakeEtcdLease{}
	c.KV = kv
	c.Lease = lease
	return c, kv, lease
}

func TestFromEtcV3Error(t *testing.T) {
	other := errors.New("other")
	for in, expected := range map[error]error{
		rpctypes.ErrKeyNotFound:                                    ErrKeyNotFound,
		concurrency.ErrElectionNoLeader:                            ErrElectionNoLeader,
		fmt.Errorf("wrapped: %w", concurrency.ErrElectionNoLeader): ErrElectionNoLeader,
		other: other,
		nil:   nil,
	} {
		if got := fromEtcV3Error(in); got != expected {
			t.Errorf("%v: expected %v, got %v", in, expected, got)
		}
	}
}

func TestEtcdV3StorePutGetList(t *testing.T) {
	ctx := context.Background()
	c, kv, lease := newFakeEtcdClient()
	s := &etcdV3Store{c: c, requestTimeout: time.Second}

	if _, err := s.Get(ctx, "/a"); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}

	if err := s.Put(ctx, "/dir/a", []byte("1"), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// without a ttl no lease is granted
	if err := s.Put(ctx, "/dir/b", []byte("2"), &WriteOptions{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := s.Put(ctx, "/other", []byte("3"), &WriteOptions{TTL: 30 * time.Second}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ttls := lease.grantedTTLs(); len(ttls) != 1 || ttls[0] != 30 {
		t.Errorf("expected one lease of 30 seconds, got %v", ttls)
	}

	pair, err := s.Get(ctx, "/dir/a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pair.Key != "/dir/a" || string(pair.Value) != "1" || pair.LastIndex != 2 {
		t.Errorf("unexpected pair %v", pair)
	}

	pairs, err := s.List(ctx, "/dir/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pairs) != 2 || pairs[0].Key != "/dir/a" || string(pairs[1].Value) != "2" || pairs[1].LastIndex != 3 {
		t.Errorf("unexpected pairs %v", pairs)
	}

	if err = s.Delete(ctx, "/dir/a"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err = s.Get(ctx, "/dir/a"); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}

	kv.putErr = errBoom
	if err = s.Put(ctx, "/a", nil, nil); !errors.Is(err, errBoom) {
		t.Errorf("expected errBoom, got %v", err)
	}
	kv.getErr = rpctypes.ErrKeyNotFound
	if _, err = s.Get(ctx, "/a"); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
	kv.getErr = errBoom
	if _, err = s.List(ctx, "/dir/"); !errors.Is(err, errBoom) {
		t.Errorf("expected errBoom, got %v", err)
	}
	kv.deleteErr = errBoom
	if err = s.Delete(ctx, "/a"); !errors.Is(err, errBoom) {
		t.Errorf("expected errBoom, got %v", err)
	}
	lease.grantErr = errBoom
	if err = s.Put(ctx, "/a", nil, &WriteOptions{TTL: time.Minute}); !errors.Is(err, errBoom) {
		t.Errorf("expected errBoom, got %v", err)
	}
}

func TestEtcdV3StoreAtomicPut(t *testing.T) {
	ctx := context.Background()
	c, kv, lease := newFakeEtcdClient()
	s := &etcdV3Store{c: c, requestTimeout: time.Second}

	first, err := s.AtomicPut(ctx, "/a", []byte("1"), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.Key != "/a" || string(first.Value) != "1" || first.LastIndex != 2 {
		t.Errorf("unexpected pair %v", first)
	}
	// create only on an existing key
	if _, err = s.AtomicPut(ctx, "/a", []byte("2"), nil, nil); !errors.Is(err, ErrKeyModified) {
		t.Errorf("expected ErrKeyModified, got %v", err)
	}

	second, err := s.AtomicPut(ctx, "/a", []byte("2"), first, &WriteOptions{TTL: 30 * time.Second})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second.LastIndex != 3 {
		t.Errorf("unexpected pair %v", second)
	}
	// the lease ttl is in seconds
	if ttls := lease.grantedTTLs(); len(ttls) != 1 || ttls[0] != 30 {
		t.Errorf("expected one lease of 30 seconds, got %v", ttls)
	}
	// stale previous pair
	if _, err = s.AtomicPut(ctx, "/a", []byte("3"), first, nil); !errors.Is(err, ErrKeyModified) {
		t.Errorf("expected ErrKeyModified, got %v", err)
	}

	kv.txnErr = errBoom
	if _, err = s.AtomicPut(ctx, "/a", []byte("3"), second, nil); !errors.Is(err, errBoom) {
		t.Errorf("expected errBoom, got %v", err)
	}
	lease.grantErr = errBoom
	if _, err = s.AtomicPut(ctx, "/a", []byte("3"), second, &WriteOptions{TTL: time.Minute}); !errors.Is(err, errBoom) {
		t.Errorf("expected errBoom, got %v", err)
	}
}

func TestEtcdV3StoreClose(t *testing.T) {
	c, _, lease := newFakeEtcdClient()
	s := &etcdV3Store{c: c, requestTimeout: time.Second}
	// a client without a connection reports its canceled context
	if err := s.Close(); !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if !lease.closed {
		t.Error("expected the lease client to be closed")
	}
}

func newTestEtcdv3Election(c *etcdclientv3.Client) *etcdv3Election {
	e, ok := NewKVBackedElection(&etcdV3Store{c: c}, "/c/leader", "keeper1", time.Second).(*etcdv3Election)
	if !ok {
		panic("expected an etcdv3 election")
	}
	return e
}

func TestEtcdv3ElectionLeader(t *testing.T) {
	c, kv, _ := newFakeEtcdClient()
	e := newTestEtcdv3Election(c)

	// a key next to the election prefix that merely starts with it is not a candidate
	kv.put("/c/leader2", []byte("other"))
	if _, err := e.Leader(t.Context()); !errors.Is(err, ErrElectionNoLeader) {
		t.Errorf("expected ErrElectionNoLeader, got %v", err)
	}

	// the first created key under the election prefix is the leader
	kv.put("/c/leader/1", []byte("keeper1"))
	kv.put("/c/leader/2", []byte("keeper2"))
	if leader, err := e.Leader(t.Context()); err != nil || leader != "keeper1" {
		t.Errorf("expected leader keeper1, got %q, %v", leader, err)
	}
	kv.getErr = errBoom
	if _, err := e.Leader(t.Context()); !errors.Is(err, errBoom) {
		t.Errorf("expected errBoom, got %v", err)
	}
}

// Reading the leader is a read: it must not grant (and revoke) a session lease in etcd for every call.
func TestEtcdv3ElectionLeaderGrantsNoLease(t *testing.T) {
	c, kv, lease := newFakeEtcdClient()
	e := newTestEtcdv3Election(c)
	kv.put("/c/leader/1", []byte("keeper1"))

	if leader, err := e.Leader(t.Context()); err != nil || leader != "keeper1" {
		t.Fatalf("expected leader keeper1, got %q, %v", leader, err)
	}
	if ttls := lease.grantedTTLs(); len(ttls) != 0 {
		t.Errorf("expected no leases for reading the leader, got %v", ttls)
	}
}

// With an unreachable etcd, reading the leader must give up within the request timeout instead of hanging.
func TestEtcdv3ElectionLeaderUnreachable(t *testing.T) {
	c, kv, lease := newFakeEtcdClient()
	e := newTestEtcdv3Election(c)
	e.requestTimeout = 100 * time.Millisecond
	kv.getBlocks, lease.grantBlocks = true, true

	done := make(chan error, 1)
	go func() {
		_, err := e.Leader(t.Context())
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected context.DeadlineExceeded, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Leader() still hangs 2s after its 100ms timeout")
	}
}

func TestEtcdv3ElectionRunForElection(t *testing.T) {
	c, kv, _ := newFakeEtcdClient()
	e := newTestEtcdv3Election(c)

	electedCh, errCh := e.RunForElection(context.Background())
	expectElected(t, electedCh, false)
	expectElected(t, electedCh, true)
	if leader, err := e.Leader(t.Context()); err != nil || leader != "keeper1" {
		t.Errorf("expected leader keeper1, got %q, %v", leader, err)
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected a panic when running twice")
			}
		}()
		e.RunForElection(context.Background())
	}()

	e.Stop()
	expectClosed(t, electedCh, errCh)

	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected a panic when stopping twice")
			}
		}()
		e.Stop()
	}()

	// a failing campaign reports its error
	kv.txnErr = errBoom
	electedCh, errCh = e.RunForElection(context.Background())
	expectElected(t, electedCh, false)
	select {
	case err := <-errCh:
		if !errors.Is(err, errBoom) {
			t.Errorf("expected errBoom, got %v", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("timeout waiting for an error")
	}
	expectClosed(t, electedCh, errCh)
	e.Stop()
}

func TestEtcdv3ElectionSessionFails(t *testing.T) {
	c, _, lease := newFakeEtcdClient()
	lease.grantErr = errBoom
	e := newTestEtcdv3Election(c)

	electedCh, errCh := e.RunForElection(context.Background())
	expectElected(t, electedCh, false)
	select {
	case err := <-errCh:
		if !errors.Is(err, errBoom) {
			t.Errorf("expected errBoom, got %v", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("timeout waiting for an error")
	}
	expectClosed(t, electedCh, errCh)
	e.Stop()
}
