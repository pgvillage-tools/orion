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
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
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

var _ = Describe("GetStatus", func() {
	It("returns the status of the cluster", func() {
		c := serve(endpoints.StatusEndPoint, http.StatusOK,
			`{"keepers":[{"uid":"k1","healthy":true}],"proxies":[{"uid":"p1"}],"sentinels":[{"uid":"s1","leader":true}],`+
				`"status":{"phase":"normal"}}`)
		status, httpCode, err := c.GetStatus()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(status).To(Equal(&apiv1.InfoCluster{
			Keepers:   apiv1.InfoKeepers{{UID: "k1", Healthy: true}},
			Proxies:   apiv1.InfoProxies{{UID: "p1"}},
			Sentinels: apiv1.InfoSentinels{{UID: "s1", Leader: true}},
			Status:    apiv1.InfoGenericStatus{"phase": "normal"},
		}))
	})
	It("returns no status on an error", func() {
		status, httpCode, err := serve(endpoints.StatusEndPoint, http.StatusInternalServerError, "ERROR").GetStatus()
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusInternalServerError))
		Ω(status).To(BeNil())
	})
})
