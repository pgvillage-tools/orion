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
	"reflect"
	"testing"

	cluster "github.com/pgvillage-tools/orion/api/v1"

	"github.com/pgvillage-tools/orion/internal/timer"
)

func TestActiveProxiesInfos(t *testing.T) {
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
	tests := []struct {
		name                       string
		proxyInfoHistories         ProxyInfoHistories
		proxiesInfos               cluster.ProxiesInfo
		expectedActiveProxies      cluster.ProxiesInfo
		expectedProxyInfoHistories ProxyInfoHistories
	}{
		{
			name:                       "should do nothing when called with empty proxyInfos",
			proxyInfoHistories:         nil,
			proxiesInfos:               cluster.ProxiesInfo{},
			expectedActiveProxies:      cluster.ProxiesInfo{},
			expectedProxyInfoHistories: nil,
		},
		{
			name:                  "should append to histories when called with proxyInfos",
			proxyInfoHistories:    make(ProxyInfoHistories),
			proxiesInfos:          cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfo2},
			expectedActiveProxies: cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfo2},
			expectedProxyInfoHistories: ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2},
			},
		},
		{
			name: "should update to histories if infoUID is different",
			proxyInfoHistories: ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1, Timer: timer.Now()},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2, Timer: timer.Now()},
			},
			proxiesInfos:          cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfoWithDifferentInfoUID},
			expectedActiveProxies: cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfoWithDifferentInfoUID},
			expectedProxyInfoHistories: ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfoWithDifferentInfoUID},
			},
		},
		{
			name: "should remove from active proxies if is not updated for twice the DefaultProxyTimeout",
			proxyInfoHistories: ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1, Timer: timer.Now() - (td45ns)},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2, Timer: timer.Now() - (td15ns)},
			},
			proxiesInfos:          cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfo2},
			expectedActiveProxies: cluster.ProxiesInfo{pr2: &proxyInfo2},
			expectedProxyInfoHistories: ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2},
			},
		},
		{
			name: "should remove proxy from sentinel's local history if the proxy is removed in store",
			proxyInfoHistories: ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1, Timer: timer.Now()},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2, Timer: timer.Now()},
			},
			proxiesInfos:               cluster.ProxiesInfo{pr2: &proxyInfo2},
			expectedActiveProxies:      cluster.ProxiesInfo{pr2: &proxyInfo2},
			expectedProxyInfoHistories: ProxyInfoHistories{pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := &Sentinel{
				uid:                "sentinel01",
				UIDFn:              testUIDFn,
				RandFn:             testRandFn,
				dbConvergenceInfos: map[string]*DBConvergenceInfo{},
				proxyInfoHistories: test.proxyInfoHistories,
			}
			actualActiveProxies := s.activeProxiesInfos(test.proxiesInfos)

			if !reflect.DeepEqual(actualActiveProxies, test.expectedActiveProxies) {
				t.Errorf("Expected proxiesInfos to be %v but got %v", test.expectedActiveProxies, actualActiveProxies)
			}
			if !isProxyInfoHistoriesEqual(s.proxyInfoHistories, test.expectedProxyInfoHistories) {
				t.Errorf("Expected proxyInfoHistories to be %v but got %v",
					test.expectedProxyInfoHistories, s.proxyInfoHistories)
			}
		})
	}
}

func isProxyInfoHistoriesEqual(actualProxyInfoHistories ProxyInfoHistories,
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
