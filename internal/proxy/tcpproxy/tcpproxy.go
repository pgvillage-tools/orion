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

// Package tcpproxy forwards the TCP connections it accepts to a destination that can change: a change closes the
// connections to the old destination, and without a destination it closes the connections it accepts. It started as
// github.com/sorintlab/pollon (Apache 2.0, by Sorint.lab).
package tcpproxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/rs/zerolog"
)

// Proxy forwards the connections of a listener to a destination
type Proxy struct {
	listener  *net.TCPListener
	keepAlive net.KeepAliveConfig

	mutex sync.Mutex
	dest  *net.TCPAddr
	// connCtx ends the connections to dest: it is canceled when dest changes, or when the proxy stops
	connCtx    context.Context
	connCancel context.CancelFunc
	// conns counts the connections that are still open
	conns sync.WaitGroup
}

// New returns a proxy on listener without a destination; keepAlive is the tcp keepalive of the connections it accepts
func New(listener *net.TCPListener, keepAlive net.KeepAliveConfig) *Proxy {
	connCtx, connCancel := context.WithCancel(context.Background())
	return &Proxy{
		listener:   listener,
		keepAlive:  keepAlive,
		connCtx:    connCtx,
		connCancel: connCancel,
	}
}

// SetDestination sets the address the proxy forwards new connections to (nil: it closes them). When the address
// changes, it closes the connections to the old one.
func (p *Proxy) SetDestination(addr *net.TCPAddr) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if addr.String() == p.dest.String() {
		return
	}
	p.connCancel()
	p.connCtx, p.connCancel = context.WithCancel(context.Background())
	p.dest = addr
}

// Destination returns the address the proxy forwards new connections to, nil when it closes them
func (p *Proxy) Destination() *net.TCPAddr {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.dest
}

// Serve accepts connections until ctx is done (it returns nil then) or accepting fails. It closes the listener, and
// all connections before it returns.
func (p *Proxy) Serve(ctx context.Context) error {
	_, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	stopListening := context.AfterFunc(ctx, func() { _ = p.listener.Close() })
	defer func() {
		stopListening()
		_ = p.listener.Close()
		p.mutex.Lock()
		p.connCancel()
		p.mutex.Unlock()
		p.conns.Wait()
	}()

	for {
		conn, err := p.listener.AcceptTCP()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept error: %w", err)
		}
		connLogger := logger.With().Stringer("client", conn.RemoteAddr()).Logger()
		if err = conn.SetKeepAliveConfig(p.keepAlive); err != nil {
			// a client that is gone already, most likely: it shouldn't stop the proxy for the other clients
			connLogger.Warn().AnErr(logging.KeyErr, err).Msg("cannot set the tcp keepalive, closing the connection")
			_ = conn.Close()
			continue
		}
		p.mutex.Lock()
		dest, connCtx := p.dest, p.connCtx
		p.conns.Add(1)
		p.mutex.Unlock()
		go func() {
			defer p.conns.Done()
			proxyConn(connCtx, &connLogger, conn, dest)
		}()
	}
}

// proxyConn forwards conn to dest until either side closes, or connCtx is done; it closes conn
func proxyConn(connCtx context.Context, logger *zerolog.Logger, conn net.Conn, dest *net.TCPAddr) {
	defer func() { _ = conn.Close() }()
	if dest == nil {
		logger.Debug().Msg("no destination, closing the connection")
		return
	}

	var dialer net.Dialer
	destConn, err := dialer.DialContext(connCtx, "tcp", dest.String())
	if err != nil {
		event := logger.Warn()
		if connCtx.Err() != nil {
			// the destination changed while connecting: not a failure of the destination
			event = logger.Debug()
		}
		event.AnErr(logging.KeyErr, err).Stringer("destination", dest).
			Msg("cannot connect to the destination, closing the connection")
		return
	}
	logger.Debug().Stringer("destination", dest).Msg("proxying connection")

	var toDest, toClient int64
	var wg sync.WaitGroup
	// copied has room for both copies, so the second one doesn't block
	copied := make(chan struct{}, 2)
	pipe := func(dst, src net.Conn, n *int64) {
		defer wg.Done()
		*n, _ = io.Copy(dst, src)
		copied <- struct{}{}
	}
	wg.Add(2)
	go pipe(destConn, conn, &toDest)
	go pipe(conn, destConn, &toClient)

	reason := "connection ended"
	select {
	case <-copied:
	case <-connCtx.Done():
		reason = "destination changed or proxy stopped"
	}
	// closing both sides ends the other copy too
	_ = conn.Close()
	_ = destConn.Close()
	wg.Wait()
	logger.Debug().Stringer("destination", dest).Str("reason", reason).
		Int64("bytes to destination", toDest).Int64("bytes to client", toClient).
		Msg("closed the connection")
}
