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
	"time"

	etcdclientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

type etcdv3Election struct {
	c            *etcdclientv3.Client
	path         string
	candidateUID string
	ttl          time.Duration

	requestTimeout time.Duration

	running bool
	cancel  context.CancelFunc
}

func (e *etcdv3Election) RunForElection(_ context.Context) (<-chan bool, <-chan error) {
	if e.running {
		panic("already running")
	}

	// every campaign has its own context and channels: a campaign that is still ending after Stop doesn't share them
	// with the next one
	electedCh := make(chan bool)
	errCh := make(chan error)
	var ctx context.Context
	ctx, e.cancel = context.WithCancel(context.Background())

	e.running = true
	go e.campaign(ctx, electedCh, errCh)

	return electedCh, errCh
}

func (e *etcdv3Election) Stop() {
	if !e.running {
		panic("not running")
	}
	e.cancel()
	e.running = false
}

// Leader reads the leader like concurrency.Election.Leader does, the value of the key under the election prefix that
// was created first, but without a session: a session would grant (and revoke) a lease in etcd for every read, without
// honouring ctx.
func (e *etcdv3Election) Leader(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, e.requestTimeout)
	defer cancel()
	resp, err := e.c.Get(ctx, e.path+"/", etcdclientv3.WithFirstCreate()...)
	if err != nil {
		return "", fromEtcV3Error(err)
	}
	if len(resp.Kvs) == 0 {
		return "", ErrElectionNoLeader
	}
	return string(resp.Kvs[0].Value), nil
}

// campaign runs for election until ctx is done (Stop), and then closes its channels. It never blocks on a send after
// Stop: the caller stops reading then.
func (e *etcdv3Election) campaign(ctx context.Context, electedCh chan<- bool, errCh chan<- error) {
	defer close(electedCh)
	defer close(errCh)

	// sendElected sends v on electedCh, unless ctx is done (checked first): it returns whether the campaign goes on
	sendElected := func(v bool) bool {
		if ctx.Err() != nil {
			return false
		}
		select {
		case electedCh <- v:
			return true
		case <-ctx.Done():
			return false
		}
	}
	sendErr := func(err error) {
		if ctx.Err() != nil {
			return
		}
		select {
		case errCh <- err:
		case <-ctx.Done():
		}
	}

	for sendElected(false) {
		s, err := concurrency.NewSession(e.c, concurrency.WithTTL(int(e.ttl.Seconds())), concurrency.WithContext(ctx))
		if err != nil {
			sendErr(err)
			return
		}

		etcdElection := concurrency.NewElection(s, e.path)
		if err = etcdElection.Campaign(ctx, e.candidateUID); err != nil {
			sendErr(err)
			return
		}

		if !sendElected(true) {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-s.Done():
			// The session is created with ctx, so Stop ends it too: sendElected(false) then sees ctx is done
		}
	}
}
