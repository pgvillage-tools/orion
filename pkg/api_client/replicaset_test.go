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
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

var _ = Describe("PromoteCluster", func() {
	It("posts the promote action without a body", func() {
		c, requests := record(respond(http.StatusAccepted, "", ""))
		httpCode, err := c.PromoteCluster()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusAccepted))
		Ω(*requests).To(Equal([]received{{method: http.MethodPost, path: "/v1/cluster/actions/promote"}}))
	})
	It("returns 409 when the API refuses it", func() {
		c, _ := record(func(w http.ResponseWriter, _ *http.Request) {
			endpoints.WriteProblem(w, http.StatusConflict, "the cluster isn't initialized")
		})
		httpCode, err := c.PromoteCluster()
		Ω(err).To(MatchError(HaveSuffix("409 Conflict: the cluster isn't initialized")))
		Ω(httpCode).To(Equal(http.StatusConflict))
	})
})
