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

package api_client_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

var _ = Describe("members", func() {
	It("GetKeepers returns the keepers", func() {
		c := serve(endpoints.KeepersEndPoint, http.StatusOK, `[{"uid":"k1","healthy":true}]`)
		keepers, httpCode, err := c.GetKeepers()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(keepers).To(Equal(apiv1.InfoKeepers{{UID: "k1", Healthy: true}}))
	})
	It("GetKeeper, GetProxy and GetSentinel get one by uid", func() {
		keeper, _, err := serve("cluster/keepers/k1", http.StatusOK, `{"uid":"k1"}`).GetKeeper("k1")
		Ω(err).NotTo(HaveOccurred())
		Ω(keeper.UID).To(Equal("k1"))
		proxy, _, err := serve("cluster/proxies/p1", http.StatusOK, `{"uid":"p1","generation":2}`).GetProxy("p1")
		Ω(err).NotTo(HaveOccurred())
		Ω(proxy).To(Equal(apiv1.InfoProxy{UID: "p1", Generation: 2}))
		sentinel, _, err := serve("cluster/sentinels/s1", http.StatusOK, `{"uid":"s1","leader":true}`).GetSentinel("s1")
		Ω(err).NotTo(HaveOccurred())
		Ω(sentinel).To(Equal(apiv1.InfoSentinel{UID: "s1", Leader: true}))
	})
	It("returns the http code when one doesn't exist", func() {
		_, httpCode, err := serve("cluster/keepers/k9", http.StatusNotFound, "").GetKeeper("k9")
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusNotFound))
	})
})
