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
	"io"
	"net"
	"strconv"
	"testing"
	"time"

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
		proxyErrCh:         make(chan error, 1),
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
	c.proxyMutex.Lock()
	ln := c.listener
	c.proxyMutex.Unlock()
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
	c.stopServing(context.Background())
}
