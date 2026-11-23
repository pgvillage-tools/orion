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
	"fmt"
	"net"
	"time"

	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/proxy/tcpproxy"
)

func (c *ClusterChecker) startProxy(ctx context.Context) error {
	_, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	c.proxyMutex.Lock()
	defer c.proxyMutex.Unlock()
	if c.pp != nil {
		return nil
	}

	logger.Info().Msg("Starting proxying")
	addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(c.listenAddress, c.port))
	if err != nil {
		return fmt.Errorf("error resolving tcp addr %q: %v", addr.String(), err)
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return fmt.Errorf("error listening on tcp addr %q: %v", addr.String(), err)
	}

	pp := tcpproxy.New(listener, keepAliveConfig(c.cfg))
	// the proxy outlives the check that starts it: it stops with stopServing
	serveCtx, stopProxy := context.WithCancel(context.WithoutCancel(ctx))
	done := make(chan struct{})
	c.pp = pp
	c.listener = listener
	c.stopProxy = stopProxy
	c.proxyDone = done

	go func() {
		defer close(done)
		if err := pp.Serve(serveCtx); err != nil {
			select {
			case c.proxyErrCh <- err:
			default:
				// an error is pending already: Start returns on that one
			}
		}
	}()

	return nil
}

// keepAliveConfig returns the tcp keepalive of the proxied connections in cfg; a setting of 0 keeps the default of the
// OS
func keepAliveConfig(cfg Config) net.KeepAliveConfig {
	// a negative setting leaves the setting of the OS
	count := cfg.KeepAliveCount
	if count == 0 {
		count = -1
	}
	seconds := func(n int) time.Duration {
		if n == 0 {
			return -1
		}
		return time.Duration(n) * time.Second
	}
	return net.KeepAliveConfig{
		Enable:   true,
		Idle:     seconds(cfg.KeepAliveIdle),
		Interval: seconds(cfg.KeepAliveInterval),
		Count:    count,
	}
}

// stopServing stops the proxy: it closes the listener and all connections
func (c *ClusterChecker) stopServing(ctx context.Context) {
	_, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	c.proxyMutex.Lock()
	defer c.proxyMutex.Unlock()
	if c.pp != nil {
		logger.Info().Msg("Stopping listening")
		c.stopProxy()
		<-c.proxyDone
		c.pp = nil
		c.listener = nil
	}
}

// setDestination sets the address the proxy forwards to (nil: it closes the connections), and returns whether it
// changed
func (c *ClusterChecker) setDestination(addr *net.TCPAddr) bool {
	c.proxyMutex.Lock()
	defer c.proxyMutex.Unlock()
	if c.pp != nil {
		c.pp.SetDestination(addr)
	}
	dest := ""
	if addr != nil {
		dest = addr.String()
	}
	changed := dest != c.dest
	c.dest = dest
	return changed
}
