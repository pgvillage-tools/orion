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
	"net"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("setDestination", func() {
	It("reports whether the address the proxy forwards to changed", func() {
		c := &ClusterChecker{}
		addr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5432}
		other := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 2), Port: 5432}
		Expect(c.setDestination(nil)).To(BeFalse(), "not forwarding at the start")
		Expect(c.setDestination(addr)).To(BeTrue())
		Expect(c.setDestination(addr)).To(BeFalse())
		Expect(c.setDestination(other)).To(BeTrue())
		Expect(c.setDestination(nil)).To(BeTrue())
	})
})

var _ = DescribeTable("keepAliveConfig",
	func(cfg Config, expected net.KeepAliveConfig) {
		Expect(keepAliveConfig(cfg)).To(Equal(expected))
	},
	Entry("keeps the defaults of the OS for settings of 0", Config{},
		net.KeepAliveConfig{Enable: true, Idle: -1, Interval: -1, Count: -1}),
	Entry("takes the settings in seconds", Config{KeepAliveIdle: 30, KeepAliveInterval: 10, KeepAliveCount: 3},
		net.KeepAliveConfig{Enable: true, Idle: 30 * time.Second, Interval: 10 * time.Second, Count: 3}),
)
