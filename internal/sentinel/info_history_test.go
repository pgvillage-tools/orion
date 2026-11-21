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

package sentinel

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cluster "github.com/pgvillage-tools/orion/api/v1"
)

var _ = Describe("DeepCopy of info histories", func() {
	It("copies nil to nil", func() {
		Ω(KeeperInfoHistories(nil).DeepCopy()).To(BeNil(), "a copy of nil isn't nil")
		Ω(ProxyInfoHistories(nil).DeepCopy()).To(BeNil(), "a copy of nil isn't nil")
	})

	It("copies keeper info histories without sharing their keeper info", func() {
		kih := KeeperInfoHistories{"keeper1": {KeeperInfo: &cluster.KeeperInfo{UID: "keeper1"}, Seen: true, Timer: 5}}
		kc := kih.DeepCopy()
		Ω(kc).To(Equal(kih))
		kc["keeper1"].KeeperInfo.UID = "changed"
		Ω(kih["keeper1"].KeeperInfo.UID).To(Equal("keeper1"), "the keeper info history copy shares its keeper info")
	})

	It("copies proxy info histories without sharing their proxy info", func() {
		pih := ProxyInfoHistories{"proxy1": {ProxyInfo: &cluster.ProxyInfo{UID: "proxy1"}, Timer: 5}}
		pc := pih.DeepCopy()
		Ω(pc).To(Equal(pih))
		pc["proxy1"].ProxyInfo.UID = "changed"
		Ω(pih["proxy1"].ProxyInfo.UID).To(Equal("proxy1"), "the proxy info history copy shares its proxy info")
	})
})
