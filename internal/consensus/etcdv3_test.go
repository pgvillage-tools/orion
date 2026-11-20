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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

var _ = Describe("fromEtcV3Error", func() {
	other := errors.New("other")

	DescribeTable("translates the etcd errors",
		func(in, expected error) {
			if expected == nil {
				Ω(fromEtcV3Error(in)).Should(BeNil())
			} else {
				Ω(fromEtcV3Error(in)).Should(BeIdenticalTo(expected), "%v", in)
			}
		},
		Entry("key not found", rpctypes.ErrKeyNotFound, ErrKeyNotFound),
		Entry("no leader", concurrency.ErrElectionNoLeader, ErrElectionNoLeader),
		Entry("wrapped no leader", fmt.Errorf("wrapped: %w", concurrency.ErrElectionNoLeader), ErrElectionNoLeader),
		Entry("other", other, other),
		Entry("nil", nil, nil),
	)
})

var _ = Describe("etcdV3Store", func() {
	var (
		ctx   context.Context
		kv    *fakeEtcdKV
		lease *fakeEtcdLease
		s     *etcdV3Store
	)

	BeforeEach(func() {
		ctx = context.Background()
		var c *etcdclientv3.Client
		c, kv, lease = newFakeEtcdClient()
		s = &etcdV3Store{c: c, requestTimeout: time.Second}
	})

	It("puts, gets, lists and deletes keys", func() {
		_, err := s.Get(ctx, "/a")
		Ω(err).Should(MatchError(ErrKeyNotFound))

		Ω(s.Put(ctx, "/dir/a", []byte("1"), nil)).Should(Succeed())
		// without a ttl no lease is granted
		Ω(s.Put(ctx, "/dir/b", []byte("2"), &WriteOptions{})).Should(Succeed())
		Ω(s.Put(ctx, "/other", []byte("3"), &WriteOptions{TTL: 30 * time.Second})).Should(Succeed())
		Ω(lease.grantedTTLs()).Should(Equal([]int64{30}), "expected one lease of 30 seconds")

		pair, err := s.Get(ctx, "/dir/a")
		Ω(err).ShouldNot(HaveOccurred())
		// keys are stored without the leading /, as libkv stored them
		Ω(kv.kvs).Should(HaveKey("dir/a"))
		Ω(pair.Key).Should(Equal("dir/a"))
		Ω(string(pair.Value)).Should(Equal("1"))
		Ω(pair.LastIndex).Should(Equal(uint64(2)))

		pairs, err := s.List(ctx, "/dir/")
		Ω(err).ShouldNot(HaveOccurred())
		Ω(pairs).Should(HaveLen(2))
		Ω(pairs[0].Key).Should(Equal("dir/a"))
		Ω(string(pairs[1].Value)).Should(Equal("2"))
		Ω(pairs[1].LastIndex).Should(Equal(uint64(3)))

		Ω(s.Delete(ctx, "/dir/a")).Should(Succeed())
		_, err = s.Get(ctx, "/dir/a")
		Ω(err).Should(MatchError(ErrKeyNotFound))

		kv.putErr = errBoom
		Ω(s.Put(ctx, "/a", nil, nil)).Should(MatchError(errBoom))
		kv.getErr = rpctypes.ErrKeyNotFound
		_, err = s.Get(ctx, "/a")
		Ω(err).Should(MatchError(ErrKeyNotFound))
		kv.getErr = errBoom
		_, err = s.List(ctx, "/dir/")
		Ω(err).Should(MatchError(errBoom))
		kv.deleteErr = errBoom
		Ω(s.Delete(ctx, "/a")).Should(MatchError(errBoom))
		lease.grantErr = errBoom
		Ω(s.Put(ctx, "/a", nil, &WriteOptions{TTL: time.Minute})).Should(MatchError(errBoom))
	})

	It("puts atomically", func() {
		first, err := s.AtomicPut(ctx, "/a", []byte("1"), nil, nil)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(first.Key).Should(Equal("a"))
		Ω(string(first.Value)).Should(Equal("1"))
		Ω(first.LastIndex).Should(Equal(uint64(2)))
		// create only on an existing key
		_, err = s.AtomicPut(ctx, "/a", []byte("2"), nil, nil)
		Ω(err).Should(MatchError(ErrKeyModified))

		second, err := s.AtomicPut(ctx, "/a", []byte("2"), first, &WriteOptions{TTL: 30 * time.Second})
		Ω(err).ShouldNot(HaveOccurred())
		Ω(second.LastIndex).Should(Equal(uint64(3)))
		// the lease ttl is in seconds
		Ω(lease.grantedTTLs()).Should(Equal([]int64{30}), "expected one lease of 30 seconds")
		// stale previous pair
		_, err = s.AtomicPut(ctx, "/a", []byte("3"), first, nil)
		Ω(err).Should(MatchError(ErrKeyModified))

		kv.txnErr = errBoom
		_, err = s.AtomicPut(ctx, "/a", []byte("3"), second, nil)
		Ω(err).Should(MatchError(errBoom))
		lease.grantErr = errBoom
		_, err = s.AtomicPut(ctx, "/a", []byte("3"), second, &WriteOptions{TTL: time.Minute})
		Ω(err).Should(MatchError(errBoom))
	})

	It("closes the client and its lease client", func() {
		// a client without a connection reports its canceled context
		Ω(s.Close()).Should(MatchError(context.Canceled))
		Ω(lease.closed).Should(BeTrue(), "expected the lease client to be closed")
	})
})

func newTestEtcdv3Election(c *etcdclientv3.Client) *etcdv3Election {
	e, ok := NewKVBackedElection(&etcdV3Store{c: c}, "/c/leader", "keeper1", time.Second).(*etcdv3Election)
	if !ok {
		panic("expected an etcdv3 election")
	}
	return e
}

// expectError waits for the next error of a campaign, which must be the expected one.
func expectError(errCh <-chan error, expected error) {
	GinkgoHelper()
	var err error
	Eventually(errCh).WithTimeout(testTimeout).Should(Receive(&err), "waiting for an error")
	Ω(err).Should(MatchError(expected))
}

var _ = Describe("etcdv3Election", func() {
	var (
		kv    *fakeEtcdKV
		lease *fakeEtcdLease
		e     *etcdv3Election
	)

	BeforeEach(func() {
		var c *etcdclientv3.Client
		c, kv, lease = newFakeEtcdClient()
		e = newTestEtcdv3Election(c)
	})

	Describe("Leader", func() {
		It("is the first candidate", func(ctx SpecContext) {
			// a key next to the election prefix that merely starts with it is not a candidate
			kv.put("/c/leader2", []byte("other"))
			_, err := e.Leader(ctx)
			Ω(err).Should(MatchError(ErrElectionNoLeader))

			// the first created key under the election prefix is the leader
			kv.put("/c/leader/1", []byte("keeper1"))
			kv.put("/c/leader/2", []byte("keeper2"))
			Ω(e.Leader(ctx)).Should(Equal("keeper1"))
			kv.getErr = errBoom
			_, err = e.Leader(ctx)
			Ω(err).Should(MatchError(errBoom))
		})

		// Reading the leader is a read: it must not grant (and revoke) a session lease in etcd for every call.
		It("grants no lease", func(ctx SpecContext) {
			kv.put("/c/leader/1", []byte("keeper1"))

			Ω(e.Leader(ctx)).Should(Equal("keeper1"))
			Ω(lease.grantedTTLs()).Should(BeEmpty(), "expected no leases for reading the leader")
		})

		// With an unreachable etcd, reading the leader must give up within the request timeout instead of hanging.
		It("gives up on an unreachable etcd", func(ctx SpecContext) {
			e.requestTimeout = 100 * time.Millisecond
			kv.getBlocks, lease.grantBlocks = true, true

			done := make(chan error, 1)
			go func() {
				_, err := e.Leader(ctx)
				done <- err
			}()
			var err error
			Eventually(done).WithTimeout(2*time.Second).Should(Receive(&err),
				"Leader() still hangs 2s after its 100ms timeout")
			Ω(err).Should(MatchError(context.DeadlineExceeded))
		})
	})

	Describe("RunForElection", func() {
		It("gets elected, and reports a failing campaign", func(ctx SpecContext) {
			electedCh, errCh := e.RunForElection(context.Background())
			expectElected(electedCh, false)
			expectElected(electedCh, true)
			Ω(e.Leader(ctx)).Should(Equal("keeper1"))

			Ω(func() { e.RunForElection(context.Background()) }).Should(Panic(), "expected a panic when running twice")

			e.Stop()
			expectClosed(electedCh, errCh)

			Ω(e.Stop).Should(Panic(), "expected a panic when stopping twice")

			// a failing campaign reports its error
			kv.txnErr = errBoom
			electedCh, errCh = e.RunForElection(context.Background())
			expectElected(electedCh, false)
			expectError(errCh, errBoom)
			expectClosed(electedCh, errCh)
			e.Stop()
		})

		It("reports a failing session", func() {
			lease.grantErr = errBoom

			electedCh, errCh := e.RunForElection(context.Background())
			expectElected(electedCh, false)
			expectError(errCh, errBoom)
			expectClosed(electedCh, errCh)
			e.Stop()
		})

		It("stops without a reader", func() {
			// stopped before anybody reads the first false: the campaign must end (and close its channels) instead of
			// blocking on the send forever
			electedCh, errCh := e.RunForElection(context.Background())
			e.Stop()
			time.Sleep(50 * time.Millisecond)
			expectClosed(electedCh, errCh)

			// the next campaign has channels of its own
			electedCh, errCh = e.RunForElection(context.Background())
			expectElected(electedCh, false)
			expectElected(electedCh, true)
			e.Stop()
			expectClosed(electedCh, errCh)
		})
	})
})
