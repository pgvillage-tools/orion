// Copyright 2026 PgVillage
// Copyright 2015 Sorint.lab
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
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	mockconsensus "github.com/pgvillage-tools/orion/internal/mock/consensus"
	"github.com/pgvillage-tools/orion/internal/util"
)

// the consts and helpers below are shared with start_test.go
const (
	proxyUID      = "proxy1"
	masterDBUID   = "db1"
	proxyGen      = int64(3)
	waitFor       = 2 * time.Second
	shortTimeout  = 100 * time.Millisecond
	checkInterval = 10 * time.Millisecond
)

// newChecker returns a ClusterChecker on a free local port, with store
func newChecker(store *mockconsensus.MockStore) *ClusterChecker {
	return &ClusterChecker{
		uid:                proxyUID,
		listenAddress:      "127.0.0.1",
		port:               "0",
		stopListening:      true,
		e:                  store,
		endPollonProxyCh:   make(chan error, 1),
		proxyCheckInterval: cluster.DefaultProxyCheckInterval,
		proxyTimeout:       cluster.DefaultProxyTimeout,
	}
}

// echoServer plays the primary: it sends back what it gets
func echoServer(t testing.TB) (host, port string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	return addr.IP.String(), strconv.Itoa(addr.Port)
}

// clusterData returns valid cluster data with db1 on host:port as the master in the proxy spec
func clusterData(host, port string, enabled ...string) *cluster.Data {
	cd := cluster.NewClusterData(cluster.NewCluster("cluster1", &cluster.Spec{InitMode: util.ToPtr(cluster.New)}))
	cd.DBs[masterDBUID] = &cluster.DB{UID: masterDBUID, Status: cluster.DBStatus{ListenAddress: host, Port: port}}
	cd.Proxy = &cluster.Proxy{Generation: proxyGen,
		Spec: cluster.ProxySpec{MasterDBUID: masterDBUID, EnabledProxies: enabled}}
	return cd
}

// proxies returns whether a connection to the proxy reaches the echo server
func proxies(c *ClusterChecker) bool {
	c.pollonMutex.Lock()
	ln := c.listener
	c.pollonMutex.Unlock()
	if ln == nil {
		return false
	}
	conn, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second)
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	if _, err = conn.Write([]byte("ping")); err != nil {
		return false
	}
	buf := make([]byte, 4)
	_, err = io.ReadFull(conn, buf)
	return err == nil && string(buf) == "ping"
}

func stop(c *ClusterChecker) {
	c.stopPollonProxy(context.Background())
}

var _ = Describe("ClusterChecker", func() {
	var store *mockconsensus.MockStore

	BeforeEach(func() {
		store = mockconsensus.NewMockStore(gomock.NewController(GinkgoT()))
	})

	// checker returns a checker on store, stopped at the end of the spec
	checker := func() *ClusterChecker {
		c := newChecker(store)
		DeferCleanup(stop, c)
		return c
	}

	Describe("Check", func() {
		var host, port string

		BeforeEach(func() {
			host, port = echoServer(GinkgoTB())
		})

		It("proxies to the master", func(ctx SpecContext) {
			store.EXPECT().GetClusterData(gomock.Any()).Return(clusterData(host, port, proxyUID), nil, nil)
			store.EXPECT().SetProxyInfo(gomock.Any(), gomock.Any(), 2*cluster.DefaultProxyTimeout).
				DoAndReturn(func(_ context.Context, pi *cluster.ProxyInfo, _ time.Duration) error {
					Expect(pi).To(SatisfyAll(
						HaveField("UID", proxyUID),
						HaveField("Generation", proxyGen),
						HaveField("ProxyTimeout", cluster.DefaultProxyTimeout),
					), "unexpected proxy info")
					return nil
				})
			c := checker()

			Expect(c.Check(ctx)).To(Succeed())
			Eventually(func() bool { return proxies(c) }).WithTimeout(waitFor).WithPolling(checkInterval).
				Should(BeTrue(), "the proxy sends connections to the master")
		})

		It("does not proxy when the proxy isn't enabled", func(ctx SpecContext) {
			store.EXPECT().GetClusterData(gomock.Any()).Return(clusterData(host, port, "other-proxy"), nil, nil)
			store.EXPECT().SetProxyInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			c := checker()

			Expect(c.Check(ctx)).To(Succeed())
			// it listens (the sentinel must see it first), but closes the connections
			Expect(c.listener).NotTo(BeNil(), "expected the proxy to listen")
			Expect(proxies(c)).To(BeFalse(), "a proxy that isn't enabled sends connections to the master")
		})

		DescribeTable("closes the connections",
			// cd builds the cluster data on the echo server; infoGen is the generation in the proxy info,
			// -1: no proxy info
			func(ctx SpecContext, cd func(host, port string) *cluster.Data, infoGen int64) {
				store.EXPECT().GetClusterData(gomock.Any()).Return(cd(host, port), nil, nil)
				if infoGen != -1 {
					store.EXPECT().SetProxyInfo(gomock.Any(), gomock.Any(), gomock.Any()).
						DoAndReturn(func(_ context.Context, pi *cluster.ProxyInfo, _ time.Duration) error {
							Expect(pi.Generation).To(Equal(infoGen), "proxy info generation")
							return nil
						})
				}
				c := checker()

				Expect(c.Check(ctx)).To(Succeed())
				Expect(proxies(c)).To(BeFalse(), "the proxy sends connections without a master")
			},
			Entry("when there is no cluster data, without proxy info",
				func(string, string) *cluster.Data { return nil }, int64(-1)),
			Entry("when there is no proxy object, with proxy info without a generation",
				func(host, port string) *cluster.Data {
					cd := clusterData(host, port, proxyUID)
					cd.Proxy = nil
					return cd
				}, cluster.NoGeneration),
			Entry("when there is no db for the master, with proxy info of the proxy generation",
				func(host, port string) *cluster.Data {
					cd := clusterData(host, port, proxyUID)
					cd.Proxy.Spec.MasterDBUID = "unknown"
					return cd
				}, proxyGen),
			Entry("when the address of the master doesn't resolve, without proxy info",
				func(_, port string) *cluster.Data { return clusterData("no such host.invalid", port, proxyUID) },
				int64(-1)),
		)

		DescribeTable("fails, and doesn't proxy",
			func(ctx SpecContext, cd func(host, port string) *cluster.Data, storeErr, infoErr error, listens bool) {
				store.EXPECT().GetClusterData(gomock.Any()).Return(cd(host, port), nil, storeErr)
				if infoErr != nil {
					store.EXPECT().SetProxyInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(infoErr)
				}
				c := checker()

				Expect(c.Check(ctx)).NotTo(Succeed())
				Expect(c.listener != nil).To(Equal(listens), "listening")
				Expect(proxies(c)).To(BeFalse(), "the proxy sends connections after a failed check")
			},
			Entry("when the store fails, without listening",
				func(string, string) *cluster.Data { return nil }, errors.New("store down"), nil, false),
			Entry("when the cluster data has an unsupported format version, listening",
				func(host, port string) *cluster.Data {
					cd := clusterData(host, port, proxyUID)
					cd.FormatVersion = cluster.CurrentCDFormatVersion + 1
					return cd
				}, nil, nil, true),
			Entry("when the cluster spec is invalid, listening",
				func(host, port string) *cluster.Data {
					cd := clusterData(host, port, proxyUID)
					cd.Cluster.Spec.InitMode = nil
					return cd
				}, nil, nil, true),
			Entry("when the proxy info can't be set, listening",
				func(host, port string) *cluster.Data { return clusterData(host, port, proxyUID) },
				nil, errors.New("store down"), true),
		)

		It("takes the timeouts of the spec", func(ctx SpecContext) {
			cd := clusterData(host, port, proxyUID)
			cd.Cluster.Spec.ProxyCheckInterval = &cluster.Duration{Duration: time.Second}
			cd.Cluster.Spec.ProxyTimeout = &cluster.Duration{Duration: time.Minute}
			store.EXPECT().GetClusterData(gomock.Any()).Return(cd, nil, nil)
			// the proxy info uses the greater of the current and the new timeout
			store.EXPECT().SetProxyInfo(gomock.Any(), gomock.Any(), 2*time.Minute).Return(nil)
			c := checker()

			Expect(c.Check(ctx)).To(Succeed())
			Expect(c.proxyCheckInterval).To(Equal(time.Second), "check interval")
			Expect(c.proxyTimeout).To(Equal(time.Minute), "timeout")
		})
	})

	It("returns the error of the pollon proxy from Start", func(ctx SpecContext) {
		store.EXPECT().GetClusterData(gomock.Any()).Return(nil, nil, nil).AnyTimes()
		c := checker()
		c.endPollonProxyCh <- errors.New("listener closed")
		Expect(c.Start(ctx)).NotTo(Succeed(), "expected the error of the pollon proxy")
	})
})

var _ = Describe("NewClusterChecker", func() {
	It("returns a cluster checker with the config", func(ctx SpecContext) {
		cfg := Config{ListenAddress: "127.0.0.1", Port: "0", StopListening: true, KeepAliveIdle: 1}
		cfg.ClusterName = "cluster1"
		cfg.StoreBackend = "etcdv3"
		cfg.StoreEndpoints = "http://127.0.0.1:1"
		cfg.StorePrefix = "orion/cluster"
		cfg.StoreTimeout = time.Second
		c, err := NewClusterChecker(ctx, proxyUID, cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(c.uid).To(Equal(proxyUID))
		Expect(c.port).To(Equal("0"))
		Expect(c.stopListening).To(BeTrue())
		Expect(c.cfg.KeepAliveIdle).To(Equal(1))
	})
})

var _ = Describe("Run", func() {
	It("fails without a store", func(ctx SpecContext) {
		// the kubernetes backend needs POD_NAME (and a cluster): no store, so no proxy
		GinkgoT().Setenv("POD_NAME", "")
		cfg := Config{ListenAddress: "127.0.0.1", Port: "0"}
		cfg.ClusterName = "cluster1"
		cfg.StoreBackend = "kubernetes"
		Expect(Run(ctx, cfg)).NotTo(Succeed(), "expected an error without a store")
	})

	It("runs until the context is done", func(ctx SpecContext) {
		cfg := Config{ListenAddress: "127.0.0.1", Port: "0", StopListening: true}
		cfg.ClusterName = "cluster1"
		cfg.StoreBackend = "etcdv3"
		cfg.StoreEndpoints = "http://127.0.0.1:1"
		cfg.StorePrefix = "orion/cluster"
		cfg.StoreTimeout = shortTimeout
		cfg.MetricsListenAddress = "127.0.0.1:0"
		runCtx, cancel := context.WithTimeout(ctx, 4*shortTimeout)
		DeferCleanup(cancel)
		// the store doesn't answer, so the checks fail; Run stops with ctx
		Expect(Run(runCtx, cfg)).To(Succeed())
	})
})
