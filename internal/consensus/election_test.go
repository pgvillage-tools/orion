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
	"sync"
	"testing"
	"time"
)

// fakeLocker is a lock that is acquired at once (or fails with lockErr). Every Lock returns a new
// lost channel, which the test can close to make the candidate lose the lock.
type fakeLocker struct {
	mu      sync.Mutex
	lockErr error
	lost    []chan struct{}
	unlocks int
}

func (l *fakeLocker) Lock(context.Context) (<-chan struct{}, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.lockErr != nil {
		return nil, l.lockErr
	}
	lost := make(chan struct{})
	l.lost = append(l.lost, lost)
	return lost, nil
}

func (l *fakeLocker) Unlock(context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.unlocks++
	return nil
}

func (l *fakeLocker) loseLock() {
	l.mu.Lock()
	defer l.mu.Unlock()
	close(l.lost[len(l.lost)-1])
}

func (l *fakeLocker) unlockCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.unlocks
}

const testTimeout = 5 * time.Second

func expectElected(t *testing.T, electedCh <-chan bool, expected bool) {
	t.Helper()
	select {
	case elected, ok := <-electedCh:
		if !ok {
			t.Fatal("elected channel closed")
		}
		if elected != expected {
			t.Fatalf("expected elected %t, got %t", expected, elected)
		}
	case <-time.After(testTimeout):
		t.Fatalf("timeout waiting for elected %t", expected)
	}
}

func expectClosed(t *testing.T, electedCh <-chan bool, errCh <-chan error) {
	t.Helper()
	for _, closed := range []func() bool{
		func() bool { _, ok := <-electedCh; return !ok },
		func() bool { _, ok := <-errCh; return !ok },
	} {
		done := make(chan bool)
		go func() { done <- closed() }()
		select {
		case ok := <-done:
			if !ok {
				t.Fatal("expected the channel to be closed")
			}
		case <-time.After(testTimeout):
			t.Fatal("timeout waiting for the channel to be closed")
		}
	}
}

func TestCandidateErrors(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*fakeLibKV)
	}{
		{name: "new lock fails", setup: func(f *fakeLibKV) { f.newLockErr = errBoom }},
		{name: "lock fails", setup: func(f *fakeLibKV) { f.lock = &fakeLocker{lockErr: errBoom} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeLibKV()
			tc.setup(f)
			c := NewCandidate(f, "/c/leader", "node1", 0)
			electedCh, errCh := c.RunForElection(context.Background())
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
			if c.IsLeader() {
				t.Error("expected not to be leader")
			}
			// without a ttl the default is used
			if ttl := f.lockOptions[0].TTL; ttl != defaultLockTTL {
				t.Errorf("expected ttl %s, got %s", defaultLockTTL, ttl)
			}
			if string(f.lockOptions[0].Value) != "node1" {
				t.Errorf("expected lock value node1, got %q", f.lockOptions[0].Value)
			}
		})
	}
}

func TestCandidateLeadership(t *testing.T) {
	f := newFakeLibKV()
	lock := &fakeLocker{}
	f.lock = lock
	c := NewCandidate(f, "/c/leader", "node1", time.Minute)

	// resigning when not leader has no effect
	c.Resign()

	electedCh, errCh := c.RunForElection(context.Background())
	expectElected(t, electedCh, false)
	expectElected(t, electedCh, true)
	if !c.IsLeader() {
		t.Error("expected to be leader")
	}
	if ttl := f.lockOptions[0].TTL; ttl != time.Minute {
		t.Errorf("expected ttl %s, got %s", time.Minute, ttl)
	}

	// losing the lock makes the candidate campaign again
	lock.loseLock()
	expectElected(t, electedCh, false)
	expectElected(t, electedCh, true)
	// the renewal of the previous lock is stopped
	select {
	case <-f.lockOptions[0].RenewLock:
	default:
		t.Error("expected the renewal of the first lock to be stopped")
	}

	// resigning unlocks and campaigns again; the resign is only picked up once the candidate waits
	// for it, so retry until it is
	resigned := false
	for !resigned {
		c.Resign()
		select {
		case elected := <-electedCh:
			if elected {
				t.Fatal("expected to lose the leadership")
			}
			resigned = true
		case <-time.After(10 * time.Millisecond):
		}
	}
	expectElected(t, electedCh, true)
	if n := lock.unlockCount(); n != 1 {
		t.Errorf("expected 1 unlock, got %d", n)
	}

	c.Stop()
	expectClosed(t, electedCh, errCh)
	if n := lock.unlockCount(); n != 2 {
		t.Errorf("expected 2 unlocks, got %d", n)
	}
	// resigning after stopping has no effect
	c.Resign()
}

func TestCandidateContextCanceled(t *testing.T) {
	f := newFakeLibKV()
	lock := &fakeLocker{}
	f.lock = lock
	c := NewCandidate(f, "/c/leader", "node1", time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	electedCh, errCh := c.RunForElection(ctx)
	expectElected(t, electedCh, false)
	expectElected(t, electedCh, true)
	cancel()
	expectClosed(t, electedCh, errCh)
	if c.IsLeader() {
		t.Error("expected not to be leader")
	}
	if n := lock.unlockCount(); n != 1 {
		t.Errorf("expected 1 unlock, got %d", n)
	}
}

func TestLibKVElectionRunForElection(t *testing.T) {
	f := newFakeLibKV()
	f.lock = &fakeLocker{}
	e, ok := NewKVBackedElection(&libKVStore{store: f}, "/c/leader", "node1", time.Second).(*libkvElection)
	if !ok {
		t.Fatal("expected a libkv election")
	}
	electedCh, errCh := e.RunForElection(context.Background())
	expectElected(t, electedCh, false)
	expectElected(t, electedCh, true)
	e.Stop()
	expectClosed(t, electedCh, errCh)
}
