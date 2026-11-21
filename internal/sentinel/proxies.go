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
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/timer"
)

// activeProxiesInfos takes the provided proxyInfo list and returns a list of
// proxiesInfo considered active. We also consider as active the proxies not yet
// in the proxyInfoHistories since only after some time we'll know if they are
// really active (updating their proxyInfo) or stale. This is needed to not
// exclude any possible active proxy from the checks in updateCluster and not
// remove them from the enabled proxies list. At worst a stale proxy will be
// added to the enabled proxies list.
func (s *Sentinel) activeProxiesInfos(proxiesInfo cluster.ProxiesInfo) cluster.ProxiesInfo {
	pihs := s.proxyInfoHistories.DeepCopy()

	// remove missing proxyInfos from the history
	for proxyUID := range pihs {
		if _, ok := proxiesInfo[proxyUID]; !ok {
			delete(pihs, proxyUID)
		}
	}

	activeProxiesInfo := proxiesInfo.DeepCopy()
	// keep only updated proxies info
	for _, pi := range proxiesInfo {
		if pih, ok := pihs[pi.UID]; ok {
			if pih.ProxyInfo.InfoUID == pi.InfoUID {
				if timer.Since(pih.Timer) > 2*pi.ProxyTimeout {
					delete(*activeProxiesInfo, pi.UID)
				}
			} else {
				pihs[pi.UID] = &ProxyInfoHistory{ProxyInfo: pi, Timer: timer.Now()}
			}
		} else {
			// add proxyInfo if not in the history
			pihs[pi.UID] = &ProxyInfoHistory{ProxyInfo: pi, Timer: timer.Now()}
		}
	}

	s.proxyInfoHistories = pihs

	return *activeProxiesInfo
}
