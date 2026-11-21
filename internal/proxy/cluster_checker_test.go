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
	cluster "github.com/pgvillage-tools/orion/api/v1"
	mockconsensus "github.com/pgvillage-tools/orion/internal/mock/consensus"
	"github.com/pgvillage-tools/orion/internal/util"
)

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

// eventually waits for cond
func eventually(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(waitFor)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for: %s", what)
		}
		time.Sleep(checkInterval)
	}
}

func stop(c *ClusterChecker) {
	c.stopPollonProxy(context.Background())
}

func TestCheckProxiesToTheMaster(t *testing.T) {
	host, port := echoServer(t)
	store := mockconsensus.NewMockStore(gomock.NewController(t))
	store.EXPECT().GetClusterData(gomock.Any()).Return(clusterData(host, port, proxyUID), nil, nil)
	store.EXPECT().SetProxyInfo(gomock.Any(), gomock.Any(), 2*cluster.DefaultProxyTimeout).
		DoAndReturn(func(_ context.Context, pi *cluster.ProxyInfo, _ time.Duration) error {
			if pi.UID != proxyUID || pi.Generation != proxyGen || pi.ProxyTimeout != cluster.DefaultProxyTimeout {
				t.Errorf("unexpected proxy info %+v", pi)
			}
			return nil
		})
	c := newChecker(store)
	defer stop(c)

	if err := c.Check(t.Context()); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return proxies(c) }, "the proxy sends connections to the master")
}

func TestCheckDoesNotProxyWhenNotEnabled(t *testing.T) {
	host, port := echoServer(t)
	store := mockconsensus.NewMockStore(gomock.NewController(t))
	store.EXPECT().GetClusterData(gomock.Any()).Return(clusterData(host, port, "other-proxy"), nil, nil)
	store.EXPECT().SetProxyInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	c := newChecker(store)
	defer stop(c)

	if err := c.Check(t.Context()); err != nil {
		t.Fatal(err)
	}
	// it listens (the sentinel must see it first), but closes the connections
	if c.listener == nil {
		t.Fatal("expected the proxy to listen")
	}
	if proxies(c) {
		t.Error("a proxy that isn't enabled sends connections to the master")
	}
}

func TestCheckClosesConnections(t *testing.T) {
	host, port := echoServer(t)
	noProxy := clusterData(host, port, proxyUID)
	noProxy.Proxy = nil
	noMasterDB := clusterData(host, port, proxyUID)
	noMasterDB.Proxy.Spec.MasterDBUID = "unknown"
	badAddress := clusterData("no such host.invalid", port, proxyUID)
	for name, tc := range map[string]struct {
		cd      *cluster.Data
		infoGen int64 // the generation in the proxy info; -1: no proxy info
	}{
		"no cluster data":      {cd: nil, infoGen: -1},
		"no proxy object":      {cd: noProxy, infoGen: cluster.NoGeneration},
		"no db for the master": {cd: noMasterDB, infoGen: proxyGen},
		"unresolvable address": {cd: badAddress, infoGen: -1},
	} {
		t.Run(name, func(t *testing.T) {
			store := mockconsensus.NewMockStore(gomock.NewController(t))
			store.EXPECT().GetClusterData(gomock.Any()).Return(tc.cd, nil, nil)
			if tc.infoGen != -1 {
				store.EXPECT().SetProxyInfo(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, pi *cluster.ProxyInfo, _ time.Duration) error {
						if pi.Generation != tc.infoGen {
							t.Errorf("proxy info generation %d, want %d", pi.Generation, tc.infoGen)
						}
						return nil
					})
			}
			c := newChecker(store)
			defer stop(c)

			if err := c.Check(t.Context()); err != nil {
				t.Fatal(err)
			}
			if proxies(c) {
				t.Error("the proxy sends connections without a master")
			}
		})
	}
}

func TestCheckErrors(t *testing.T) {
	host, port := echoServer(t)
	oldFormat := clusterData(host, port, proxyUID)
	oldFormat.FormatVersion = cluster.CurrentCDFormatVersion + 1
	invalid := clusterData(host, port, proxyUID)
	invalid.Cluster.Spec.InitMode = nil
	for name, tc := range map[string]struct {
		cd       *cluster.Data
		storeErr error
		infoErr  error
		listens  bool
	}{
		"the store fails":            {storeErr: errors.New("store down")},
		"unsupported format version": {cd: oldFormat, listens: true},
		"invalid spec":               {cd: invalid, listens: true},
		"the proxy info can't be set": {cd: clusterData(host, port, proxyUID), infoErr: errors.New("store down"),
			listens: true},
	} {
		t.Run(name, func(t *testing.T) {
			store := mockconsensus.NewMockStore(gomock.NewController(t))
			store.EXPECT().GetClusterData(gomock.Any()).Return(tc.cd, nil, tc.storeErr)
			if tc.infoErr != nil {
				store.EXPECT().SetProxyInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(tc.infoErr)
			}
			c := newChecker(store)
			defer stop(c)

			if err := c.Check(t.Context()); err == nil {
				t.Fatal("expected an error")
			}
			if (c.listener != nil) != tc.listens {
				t.Errorf("listening: %v, want %v", c.listener != nil, tc.listens)
			}
			if proxies(c) {
				t.Error("the proxy sends connections after a failed check")
			}
		})
	}
}

func TestCheckTakesTheTimeoutsOfTheSpec(t *testing.T) {
	host, port := echoServer(t)
	cd := clusterData(host, port, proxyUID)
	cd.Cluster.Spec.ProxyCheckInterval = &cluster.Duration{Duration: time.Second}
	cd.Cluster.Spec.ProxyTimeout = &cluster.Duration{Duration: time.Minute}
	store := mockconsensus.NewMockStore(gomock.NewController(t))
	store.EXPECT().GetClusterData(gomock.Any()).Return(cd, nil, nil)
	// the proxy info uses the greater of the current and the new timeout
	store.EXPECT().SetProxyInfo(gomock.Any(), gomock.Any(), 2*time.Minute).Return(nil)
	c := newChecker(store)
	defer stop(c)

	if err := c.Check(t.Context()); err != nil {
		t.Fatal(err)
	}
	if c.proxyCheckInterval != time.Second || c.proxyTimeout != time.Minute {
		t.Errorf("got check interval %s and timeout %s", c.proxyCheckInterval, c.proxyTimeout)
	}
}

func TestStartReturnsTheErrorOfThePollonProxy(t *testing.T) {
	store := mockconsensus.NewMockStore(gomock.NewController(t))
	store.EXPECT().GetClusterData(gomock.Any()).Return(nil, nil, nil).AnyTimes()
	c := newChecker(store)
	defer stop(c)
	c.endPollonProxyCh <- errors.New("listener closed")
	if err := c.Start(t.Context()); err == nil {
		t.Fatal("expected the error of the pollon proxy")
	}
}

func TestNewClusterChecker(t *testing.T) {
	cfg := Config{ListenAddress: "127.0.0.1", Port: "0", StopListening: true, KeepAliveIdle: 1}
	cfg.ClusterName = "cluster1"
	cfg.StoreBackend = "etcdv3"
	cfg.StoreEndpoints = "http://127.0.0.1:1"
	cfg.StorePrefix = "orion/cluster"
	cfg.StoreTimeout = time.Second
	c, err := NewClusterChecker(t.Context(), proxyUID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c.uid != proxyUID || c.port != "0" || !c.stopListening || c.cfg.KeepAliveIdle != 1 {
		t.Errorf("unexpected cluster checker %+v", c)
	}
}

func TestRunWithoutAStore(t *testing.T) {
	// the kubernetes backend needs POD_NAME (and a cluster): no store, so no proxy
	t.Setenv("POD_NAME", "")
	cfg := Config{ListenAddress: "127.0.0.1", Port: "0"}
	cfg.ClusterName = "cluster1"
	cfg.StoreBackend = "kubernetes"
	if err := Run(t.Context(), cfg); err == nil {
		t.Fatal("expected an error without a store")
	}
}

func TestRunUntilTheContextIsDone(t *testing.T) {
	cfg := Config{ListenAddress: "127.0.0.1", Port: "0", StopListening: true}
	cfg.ClusterName = "cluster1"
	cfg.StoreBackend = "etcdv3"
	cfg.StoreEndpoints = "http://127.0.0.1:1"
	cfg.StorePrefix = "orion/cluster"
	cfg.StoreTimeout = shortTimeout
	cfg.MetricsListenAddress = "127.0.0.1:0"
	ctx, cancel := context.WithTimeout(t.Context(), 4*shortTimeout)
	defer cancel()
	// the store doesn't answer, so the checks fail; Run stops with ctx
	if err := Run(ctx, cfg); err != nil {
		t.Fatal(err)
	}
}
