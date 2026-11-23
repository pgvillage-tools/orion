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

var _ = Describe("Proxy", func() {
	var (
		p      *Proxy
		ctx    context.Context
		cancel context.CancelFunc
		served chan error
	)

	// serve starts p with keepAlive
	serve := func(keepAlive net.KeepAliveConfig) {
		ln, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
		Expect(err).NotTo(HaveOccurred())
		p = New(ln, keepAlive)
		ctx, cancel = context.WithCancel(context.Background())
		served = make(chan error, 1)
		// stopped is closed when Serve returned, served gets its result
		stopped := make(chan struct{})
		go func(p *Proxy, ctx context.Context, served chan<- error) {
			defer close(stopped)
			served <- p.Serve(ctx)
		}(p, ctx, served)
		DeferCleanup(func(cancel context.CancelFunc) {
			cancel()
			Eventually(stopped).WithTimeout(waitFor).Should(BeClosed())
		}, cancel)
	}

	BeforeEach(func() {
		serve(net.KeepAliveConfig{Enable: true, Idle: -1, Interval: -1, Count: -1})
	})

	It("forwards connections to the destination", func() {
		p.SetDestination(server("d1"))
		conn := dial(p)
		Expect(greeting(conn)).To(Equal("d1"))
		Expect(echoes(conn)).To(BeTrue())
	})

	It("closes connections without a destination", func() {
		Expect(p.Destination()).To(BeNil())
		_, err := greeting(dial(p))
		Expect(err).To(MatchError(io.EOF))
	})

	It("closes connections when the destination doesn't answer", func() {
		p.SetDestination(closedAddr())
		_, err := greeting(dial(p))
		Expect(err).To(HaveOccurred())
	})

	It("closes the connections to the old destination when the destination changes", func() {
		d1, d2 := server("d1"), server("d2")
		p.SetDestination(d1)
		old := dial(p)
		Expect(greeting(old)).To(Equal("d1"))

		p.SetDestination(d2)
		Expect(p.Destination()).To(Equal(d2))
		Expect(echoes(old)).To(BeFalse(), "a connection to the old destination stays open")
		conn := dial(p)
		Expect(greeting(conn)).To(Equal("d2"))
		Expect(echoes(conn)).To(BeTrue())
	})

	It("keeps the connections when the destination stays the same", func() {
		d1 := server("d1")
		p.SetDestination(d1)
		conn := dial(p)
		Expect(greeting(conn)).To(Equal("d1"))
		// an equal address, not the same pointer
		p.SetDestination(&net.TCPAddr{IP: d1.IP, Port: d1.Port})
		Expect(echoes(conn)).To(BeTrue())
	})

	It("closes all connections when the destination is removed", func() {
		p.SetDestination(server("d1"))
		conn := dial(p)
		Expect(greeting(conn)).To(Equal("d1"))
		p.SetDestination(nil)
		Expect(echoes(conn)).To(BeFalse())
	})

	It("stops when its context is done, closing the listener and the connections", func() {
		p.SetDestination(server("d1"))
		conn := dial(p)
		Expect(greeting(conn)).To(Equal("d1"))
		addr := p.listener.Addr().String()

		cancel()
		Eventually(served).WithTimeout(waitFor).Should(Receive(BeNil()))
		Expect(echoes(conn)).To(BeFalse(), "a connection stays open after the proxy stopped")
		_, err := net.DialTimeout("tcp", addr, waitFor)
		Expect(err).To(HaveOccurred(), "the proxy still listens")
	})

	It("returns an error when accepting fails", func() {
		Expect(p.listener.Close()).To(Succeed())
		Eventually(served).WithTimeout(waitFor).Should(Receive(MatchError(ContainSubstring("accept error"))))
	})

	Context("with tcp keepalive settings", func() {
		BeforeEach(func() {
			serve(net.KeepAliveConfig{Enable: true, Idle: time.Second, Interval: time.Second, Count: 3})
		})

		It("forwards connections", func() {
			p.SetDestination(server("d1"))
			conn := dial(p)
			Expect(greeting(conn)).To(Equal("d1"))
			Expect(echoes(conn)).To(BeTrue())
		})
	})
})
