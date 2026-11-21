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

package sentinel

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/timer"
)

var _ = Describe("activeProxiesInfos", func() {
	const (
		pr1 = "proxy01"
		pr2 = "proxy02"
	)
	proxyInfo1 := cluster.ProxyInfo{UID: pr1, InfoUID: "infoUID1", ProxyTimeout: cluster.DefaultProxyTimeout}
	proxyInfo2 := cluster.ProxyInfo{UID: pr2, InfoUID: "infoUID2", ProxyTimeout: cluster.DefaultProxyTimeout}
	proxyInfoWithDifferentInfoUID := cluster.ProxyInfo{UID: pr2, InfoUID: "differentInfoUID"}
	var (
		secToNanoSecondMultiplier int64 = 1000000000

		td15ns = 15 * secToNanoSecondMultiplier
		td45ns = 45 * secToNanoSecondMultiplier
	)

	isProxyInfoHistoriesEqual := func(actualProxyInfoHistories ProxyInfoHistories,
		expectedProxyInfoHistories ProxyInfoHistories) bool {
		if len(actualProxyInfoHistories) != len(expectedProxyInfoHistories) {
			return false
		}
		for k, expectedProxyInfoHistory := range expectedProxyInfoHistories {
			actualProxyInfoHistory, ok := actualProxyInfoHistories[k]
			if !ok {
				return false
			}
			if actualProxyInfoHistory.ProxyInfo.InfoUID != expectedProxyInfoHistory.ProxyInfo.InfoUID ||
				actualProxyInfoHistory.ProxyInfo.UID != expectedProxyInfoHistory.ProxyInfo.UID {
				return false
			}
		}
		return true
	}

	DescribeTable("keeps the proxy info histories and returns the active proxies",
		// proxyInfoHistories is a func, so the timers are set when the spec runs
		func(proxyInfoHistories func() ProxyInfoHistories, proxiesInfos cluster.ProxiesInfo,
			expectedActiveProxies cluster.ProxiesInfo, expectedProxyInfoHistories ProxyInfoHistories) {
			s := &Sentinel{
				uid:                "sentinel01",
				UIDFn:              testUIDFn,
				RandFn:             testRandFn,
				dbConvergenceInfos: map[string]*DBConvergenceInfo{},
				proxyInfoHistories: proxyInfoHistories(),
			}
			actualActiveProxies := s.activeProxiesInfos(proxiesInfos)

			Ω(actualActiveProxies).To(Equal(expectedActiveProxies), "proxiesInfos")
			Ω(isProxyInfoHistoriesEqual(s.proxyInfoHistories, expectedProxyInfoHistories)).To(BeTrue(),
				"Expected proxyInfoHistories to be %v but got %v", expectedProxyInfoHistories, s.proxyInfoHistories)
		},
		Entry("does nothing when called with empty proxyInfos",
			func() ProxyInfoHistories { return nil },
			cluster.ProxiesInfo{},
			cluster.ProxiesInfo{},
			nil,
		),
		Entry("appends to histories when called with proxyInfos",
			func() ProxyInfoHistories { return make(ProxyInfoHistories) },
			cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfo2},
			cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfo2},
			ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2},
			},
		),
		Entry("updates the histories if infoUID is different",
			func() ProxyInfoHistories {
				return ProxyInfoHistories{
					pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1, Timer: timer.Now()},
					pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2, Timer: timer.Now()},
				}
			},
			cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfoWithDifferentInfoUID},
			cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfoWithDifferentInfoUID},
			ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfoWithDifferentInfoUID},
			},
		),
		Entry("removes from active proxies if is not updated for twice the DefaultProxyTimeout",
			func() ProxyInfoHistories {
				return ProxyInfoHistories{
					pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1, Timer: timer.Now() - (td45ns)},
					pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2, Timer: timer.Now() - (td15ns)},
				}
			},
			cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfo2},
			cluster.ProxiesInfo{pr2: &proxyInfo2},
			ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2},
			},
		),
		Entry("removes a proxy from sentinel's local history if the proxy is removed in store",
			func() ProxyInfoHistories {
				return ProxyInfoHistories{
					pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1, Timer: timer.Now()},
					pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2, Timer: timer.Now()},
				}
			},
			cluster.ProxiesInfo{pr2: &proxyInfo2},
			cluster.ProxiesInfo{pr2: &proxyInfo2},
			ProxyInfoHistories{pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2}},
		),
	)
})
