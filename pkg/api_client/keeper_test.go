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
	client "github.com/pgvillage-tools/orion/pkg/api_client"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

var _ = Describe("keeper", func() {
	DescribeTable("sends the request for the keeper",
		func(do func(c client.Connection, keeper string) (int, error), method, path string) {
			c, requests := record(respond(http.StatusAccepted, "", ""))
			httpCode, err := do(c, "keeper1")
			Ω(err).NotTo(HaveOccurred())
			Ω(httpCode).To(Equal(http.StatusAccepted))
			Ω(*requests).To(Equal([]received{{method: method, path: path}}))
		},
		Entry("FailKeeper", client.Connection.FailKeeper, http.MethodPost, "/v1/cluster/keepers/keeper1/actions/fail"),
		Entry("DeleteKeeper", client.Connection.DeleteKeeper, http.MethodDelete, "/v1/cluster/keepers/keeper1"),
	)
	DescribeTable("returns 404 when the keeper doesn't exist",
		func(do func(c client.Connection, keeper string) (int, error)) {
			c, _ := record(func(w http.ResponseWriter, _ *http.Request) {
				endpoints.WriteProblem(w, http.StatusNotFound, "keeper keeper9 doesn't exist")
			})
			httpCode, err := do(c, "keeper9")
			Ω(err).To(MatchError(HaveSuffix("404 Not Found: keeper keeper9 doesn't exist")))
			Ω(httpCode).To(Equal(http.StatusNotFound))
		},
		Entry("FailKeeper", client.Connection.FailKeeper),
		Entry("DeleteKeeper", client.Connection.DeleteKeeper),
	)
	DescribeTable("sends no request for a keeper id with a placeholder",
		func(do func(c client.Connection, keeper string) (int, error)) {
			c, requests := record(respond(http.StatusAccepted, "", ""))
			httpCode, err := do(c, "{id}")
			Ω(err).To(MatchError(ContainSubstring("not all placeholders")))
			Ω(httpCode).To(Equal(-1))
			Ω(*requests).To(BeEmpty())
		},
		Entry("FailKeeper", client.Connection.FailKeeper),
		Entry("DeleteKeeper", client.Connection.DeleteKeeper),
	)
})
