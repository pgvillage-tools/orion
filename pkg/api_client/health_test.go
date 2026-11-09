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

var _ = Describe("health", func() {
	It("Live and Ready return 200 when the API is live and ready", func() {
		c, _ := record(respond(http.StatusOK, "", "ok"))
		httpCode, err := c.Live()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		httpCode, err = c.Ready()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
	})
	It("Ready returns 503 when the API isn't ready", func() {
		c, _ := record(func(w http.ResponseWriter, _ *http.Request) {
			endpoints.WriteProblem(w, http.StatusServiceUnavailable, "the store doesn't answer")
		})
		httpCode, err := c.Ready()
		Ω(err).To(MatchError(HaveSuffix("503 Service Unavailable: the store doesn't answer")))
		Ω(httpCode).To(Equal(http.StatusServiceUnavailable))
	})
	DescribeTable("Mode returns the mode of the API",
		func(body, mode string) {
			c, _ := record(respond(http.StatusOK, "", body))
			got, httpCode, err := c.Mode()
			Ω(err).NotTo(HaveOccurred())
			Ω(httpCode).To(Equal(http.StatusOK))
			Ω(got).To(Equal(endpoints.ModeResponse{Mode: mode}))
		},
		Entry("normal", `{"mode":"normal"}`, endpoints.ModeNormal),
		Entry("bootstrap", `{"mode":"bootstrap"}`, endpoints.ModeBootstrap),
	)
	It("WhoAmI returns the identity of the client", func() {
		c, _ := record(respond(http.StatusOK, "", `{"name":"admin","groups":["orion-admins"],"method":"mtls"}`))
		identity, httpCode, err := c.WhoAmI()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(identity).To(Equal(endpoints.WhoAmIResponse{Name: "admin", Groups: []string{"orion-admins"}, Method: "mtls"}))
	})
	It("WhoAmI returns no identity on an error", func() {
		c, _ := record(func(w http.ResponseWriter, _ *http.Request) {
			endpoints.WriteProblem(w, http.StatusUnauthorized, "no client certificate")
		})
		identity, httpCode, err := c.WhoAmI()
		Ω(err).To(MatchError(ContainSubstring("no client certificate")))
		Ω(httpCode).To(Equal(http.StatusUnauthorized))
		Ω(identity).To(BeZero())
	})
})
