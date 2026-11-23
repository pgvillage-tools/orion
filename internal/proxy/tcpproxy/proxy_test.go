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
	"io"
	"net"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Proxy destination", func() {
	var p *Proxy

	BeforeEach(func() {
		p, _, _ = serve(osKeepAlive)
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
})
