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
	"net"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Proxy Serve", func() {
	var (
		p      *Proxy
		cancel context.CancelFunc
		served chan error
	)

	BeforeEach(func() {
		p, cancel, served = serve(osKeepAlive)
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
			cancel()
			Eventually(served).WithTimeout(waitFor).Should(Receive(BeNil()))
			p, cancel, served = serve(net.KeepAliveConfig{Enable: true, Idle: time.Second, Interval: time.Second,
				Count: 3})
		})

		It("forwards connections", func() {
			p.SetDestination(server("d1"))
			conn := dial(p)
			Expect(greeting(conn)).To(Equal("d1"))
			Expect(echoes(conn)).To(BeTrue())
		})
	})
})
