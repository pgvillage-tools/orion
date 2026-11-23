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
	"net"
	"sync"
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
