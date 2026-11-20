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

package proxy

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/consensus"
	mockconsensus "github.com/pgvillage-tools/orion/internal/mock/consensus"
)

// listening returns whether the proxy listens
func listening(c *ClusterChecker) bool {
	c.pollonMutex.Lock()
	defer c.pollonMutex.Unlock()
	return c.listener != nil
}

var _ = Describe("Start", func() {
	var (
		store  *mockconsensus.MockStore
		c      *ClusterChecker
		cd     *cluster.Data
		ctx    context.Context
		cancel context.CancelFunc
		done   chan error
		// whether the store answers: once false, GetClusterData fails
		storeUp atomic.Bool
	)

	BeforeEach(func() {
		host, port := echoServer(GinkgoTB())
		cd = clusterData(host, port, proxyUID)
		// short timeouts from the spec, which a successful check takes over
		cd.Cluster.Spec.ProxyCheckInterval = &cluster.Duration{Duration: checkInterval}
		cd.Cluster.Spec.ProxyTimeout = &cluster.Duration{Duration: shortTimeout}
		store = mockconsensus.NewMockStore(gomock.NewController(GinkgoT()))
		store.EXPECT().SetProxyInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		c = newChecker(store)
		storeUp.Store(true)
		ctx, cancel = context.WithCancel(context.Background())
		done = make(chan error, 1)
		DeferCleanup(func() {
			cancel()
			Eventually(done).Should(Receive(BeNil()), "Start returns when its context is done")
			stop(c)
		})
	})

	start := func() {
		go func() { done <- c.Start(ctx) }()
		Eventually(func() bool { return proxies(c) }).WithTimeout(waitFor).Should(BeTrue(),
			"the proxy sends connections to the master")
	}

	It("keeps proxying while its checks succeed", func() {
		store.EXPECT().GetClusterData(gomock.Any()).Return(cd, nil, nil).AnyTimes()
		start()
		// many proxy timeouts long
		Consistently(func() bool { return proxies(c) }).WithTimeout(5 * shortTimeout).
			WithPolling(shortTimeout / 4).Should(BeTrue())
	})

	It("fences itself when its checks fail for the proxy timeout", func() {
		store.EXPECT().GetClusterData(gomock.Any()).DoAndReturn(func(context.Context) (*cluster.Data,
			*consensus.KVPair, error) {
			if storeUp.Load() {
				return cd, nil, nil
			}
			return nil, nil, errors.New("store down")
		}).AnyTimes()
		start()
		storeUp.Store(false)
		Eventually(func() bool { return listening(c) }).WithTimeout(waitFor).Should(BeFalse(),
			"the proxy stops listening")
	})

	It("fences itself when a check hangs, and cancels the check", func() {
		var canceled atomic.Bool
		store.EXPECT().GetClusterData(gomock.Any()).DoAndReturn(func(checkCtx context.Context) (*cluster.Data,
			*consensus.KVPair, error) {
			if storeUp.Load() {
				return cd, nil, nil
			}
			// a store that doesn't answer: the check waits until its context ends
			<-checkCtx.Done()
			canceled.Store(true)
			return nil, nil, checkCtx.Err()
		}).AnyTimes()
		start()
		storeUp.Store(false)
		Eventually(func() bool { return listening(c) }).WithTimeout(waitFor).Should(BeFalse(),
			"the proxy stops listening")
		Eventually(canceled.Load).WithTimeout(waitFor).Should(BeTrue(), "the hanging check is canceled")
	})

	It("serves again once a check succeeds after fencing", func() {
		store.EXPECT().GetClusterData(gomock.Any()).DoAndReturn(func(context.Context) (*cluster.Data,
			*consensus.KVPair, error) {
			if storeUp.Load() {
				return cd, nil, nil
			}
			return nil, nil, errors.New("store down")
		}).AnyTimes()
		start()
		storeUp.Store(false)
		Eventually(func() bool { return listening(c) }).WithTimeout(waitFor).Should(BeFalse())
		storeUp.Store(true)
		Eventually(func() bool { return proxies(c) }).WithTimeout(waitFor).Should(BeTrue())
	})
})
