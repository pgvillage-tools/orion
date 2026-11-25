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

package tcpproxy

import (
	"context"
	"io"
	"net"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	waitFor  = 2 * time.Second
	nameSize = 2
)

// server plays a destination: on every connection it first sends its name (nameSize bytes), then echoes
func server(name string) *net.TCPAddr {
	ln, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(ln.Close)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				if _, err := conn.Write([]byte(name)); err != nil {
					return
				}
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr)
}

// closedAddr returns an address nothing listens on
func closedAddr() *net.TCPAddr {
	ln, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	Expect(err).NotTo(HaveOccurred())
	addr := ln.Addr().(*net.TCPAddr)
	Expect(ln.Close()).To(Succeed())
	return addr
}

// dial connects to the proxy
func dial(p *Proxy) net.Conn {
	conn, err := net.DialTimeout("tcp", p.listener.Addr().String(), waitFor)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(conn.Close)
	Expect(conn.SetDeadline(time.Now().Add(waitFor))).To(Succeed())
	return conn
}

// greeting returns the name of the destination conn reaches, and an error when the proxy closed it
func greeting(conn net.Conn) (string, error) {
	buf := make([]byte, nameSize)
	_, err := io.ReadFull(conn, buf)
	return string(buf), err
}

// echoes returns whether conn still reaches its destination
func echoes(conn net.Conn) bool {
	if _, err := conn.Write([]byte("ping")); err != nil {
		return false
	}
	buf := make([]byte, 4)
	_, err := io.ReadFull(conn, buf)
	return err == nil && string(buf) == "ping"
}

// eventuallyClosed waits until the proxy closed conn: SetDestination cancels the connections, and the proxy closes
// them in their own goroutines, so a connection can still echo right after SetDestination returns
func eventuallyClosed(conn net.Conn, description string) {
	GinkgoHelper()
	Eventually(func() bool { return echoes(conn) }).WithTimeout(waitFor).Should(BeFalse(), description)
}

// osKeepAlive keeps the tcp keepalive settings of the OS
var osKeepAlive = net.KeepAliveConfig{Enable: true, Idle: -1, Interval: -1, Count: -1}

// serve starts a proxy with keepAlive on a free local port, and returns it, the function that stops it, and the
// channel that gets the result of Serve; the proxy is stopped at the end of the spec
func serve(keepAlive net.KeepAliveConfig) (*Proxy, context.CancelFunc, chan error) {
	ln, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	Expect(err).NotTo(HaveOccurred())
	p := New(ln, keepAlive)
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	// stopped is closed when Serve returned
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		served <- p.Serve(ctx)
	}()
	DeferCleanup(func() {
		cancel()
		Eventually(stopped).WithTimeout(waitFor).Should(BeClosed())
	})
	return p, cancel, served
}
