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
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	client "github.com/pgvillage-tools/orion/pkg/api_client"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

var _ = Describe("Connection", func() {
	// request is the last request the test server received
	type request struct {
		method, path, contentType, body string
	}
	var last request
	// connect starts a test server that answers every request with handler, and returns a connection to it
	connect := func(handler http.HandlerFunc) client.Connection {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			last = request{r.Method, r.URL.Path, r.Header.Get("Content-Type"), string(body)}
			handler(w, r)
		}))
		DeferCleanup(srv.Close)
		u, err := url.Parse(srv.URL)
		Ω(err).NotTo(HaveOccurred())
		port, err := strconv.ParseUint(u.Port(), 10, 16)
		Ω(err).NotTo(HaveOccurred())
		return client.NewConnection(endpoints.HTTP, u.Hostname(), uint16(port), time.Second)
	}
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) }

	It("returns the problem of an error response as the error", func() {
		c := connect(func(w http.ResponseWriter, _ *http.Request) {
			endpoints.WriteProblem(w, http.StatusNotFound, "keeper keeper1 doesn't exist")
		})
		httpCode, err := c.Delete(endpoints.KeeperEndPoint)
		Ω(httpCode).To(Equal(http.StatusNotFound))
		Ω(err).To(MatchError(ContainSubstring("DELETE ")))
		Ω(err).To(MatchError(HaveSuffix("404 Not Found: keeper keeper1 doesn't exist")))
		var p endpoints.Problem
		Ω(errors.As(err, &p)).To(BeTrue())
		Ω(p.Status).To(Equal(http.StatusNotFound))
		Ω(p.Detail).To(Equal("keeper keeper1 doesn't exist"))
	})
	It("makes a problem of an error response that isn't one", func() {
		c := connect(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		})
		httpCode, err := c.Get(endpoints.StatusEndPoint, nil)
		Ω(httpCode).To(Equal(http.StatusMethodNotAllowed))
		var p endpoints.Problem
		Ω(errors.As(err, &p)).To(BeTrue())
		Ω(p).To(Equal(endpoints.NewProblem(http.StatusMethodNotAllowed, "Method Not Allowed")))
	})
	It("sends JSON with PUT and POST, and the patch with PATCH", func() {
		c := connect(ok)
		httpCode, err := c.Put(endpoints.ClusterEndPoint, map[string]int{"a": 1})
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusAccepted))
		Ω(last).To(Equal(request{http.MethodPut, "/v1/cluster", "application/json", `{"a":1}`}))

		_, err = c.Post(endpoints.ClusterEndPoint, map[string]int{"b": 2})
		Ω(err).NotTo(HaveOccurred())
		Ω(last).To(Equal(request{http.MethodPost, "/v1/cluster", "application/json", `{"b":2}`}))

		_, err = c.Patch(endpoints.ClusterSpecEndPoint, []byte(`{"c":3}`))
		Ω(err).NotTo(HaveOccurred())
		Ω(last).To(Equal(request{http.MethodPatch, "/v1/cluster/spec", "application/json", `{"c":3}`}))
	})
	It("decodes the response of a GET", func() {
		c := connect(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"mode":"normal"}`)) })
		var mode endpoints.ModeResponse
		httpCode, err := c.Get(endpoints.ModeEndPoint, &mode)
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(mode.Mode).To(Equal(endpoints.ModeNormal))
		Ω(last.method).To(Equal(http.MethodGet))
	})
	It("returns an error when the response can't be decoded", func() {
		c := connect(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`not json`)) })
		var mode endpoints.ModeResponse
		_, err := c.Get(endpoints.ModeEndPoint, &mode)
		Ω(err).To(MatchError(ContainSubstring("failed to decode the response")))
	})
	It("sends an action as a POST without a body", func() {
		c := connect(ok)
		httpCode, err := c.Action(endpoints.PromoteClusterEndPoint)
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusAccepted))
		Ω(last).To(Equal(request{http.MethodPost, "/v1/cluster/actions/promote", "", ""}))
	})
})
