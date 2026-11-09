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
	"encoding/json"
	"errors"
	"net/http"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	client "github.com/pgvillage-tools/orion/pkg/api_client"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// call is an exported client function, and the request it should send
type call struct {
	name string
	do   func(c client.Connection) (httpCode int, err error)
	want received
	// reply is a valid response body for the call; empty when the call doesn't decode the response
	reply string
}

// mustJSON returns o as JSON. It panics when o can't be marshaled, as it is used to initialize package variables
// (before gomega has a fail handler).
func mustJSON(o any) string {
	data, err := json.Marshal(o)
	if err != nil {
		panic(err)
	}
	return string(data)
}

const (
	etag  = `"7"`
	patch = `{"synchronousReplication":true}`
)

var (
	data = &apiv1.Data{FormatVersion: 1, Keepers: apiv1.Keepers{}}
	spec = &apiv1.Spec{AdditionalWalSenders: new(uint16(3))}
)

// calls are all exported client functions that call an endpoint
var calls = []call{
	{name: "GetCluster", reply: `{}`, want: received{method: http.MethodGet, path: "/v1/cluster"},
		do: func(c client.Connection) (int, error) { _, code, err := c.GetCluster(); return code, err }},
	{name: "GetClusterWithETag", reply: `{}`, want: received{method: http.MethodGet, path: "/v1/cluster"},
		do: func(c client.Connection) (int, error) { _, _, code, err := c.GetClusterWithETag(); return code, err }},
	{name: "PutCluster", want: received{http.MethodPut, "/v1/cluster", "application/json", "", mustJSON(data)},
		do: func(c client.Connection) (int, error) { return c.PutCluster(data) }},
	{name: "PutClusterIfMatch", want: received{http.MethodPut, "/v1/cluster", "application/json", etag, mustJSON(data)},
		do: func(c client.Connection) (int, error) {
			code, _, err := c.PutClusterIfMatch(data, etag)
			return code, err
		}},
	{name: "PostCluster", want: received{http.MethodPost, "/v1/cluster", "application/json", "", mustJSON(spec)},
		do: func(c client.Connection) (int, error) { return c.PostCluster(spec) }},
	{name: "GetClusterSpec", reply: `{}`, want: received{method: http.MethodGet, path: "/v1/cluster/spec"},
		do: func(c client.Connection) (int, error) { _, code, err := c.GetClusterSpec(); return code, err }},
	{name: "GetClusterSpecWithETag", reply: `{}`, want: received{method: http.MethodGet, path: "/v1/cluster/spec"},
		do: func(c client.Connection) (int, error) {
			_, _, code, err := c.GetClusterSpecWithETag()
			return code, err
		}},
	{name: "PutClusterSpec",
		want: received{http.MethodPut, "/v1/cluster/spec", "application/json", "", mustJSON(spec)},
		do:   func(c client.Connection) (int, error) { return c.PutClusterSpec(spec) }},
	{name: "PutClusterSpecIfMatch",
		want: received{http.MethodPut, "/v1/cluster/spec", "application/json", etag, mustJSON(spec)},
		do: func(c client.Connection) (int, error) {
			code, _, err := c.PutClusterSpecIfMatch(spec, etag)
			return code, err
		}},
	{name: "PatchClusterSpec",
		want: received{http.MethodPatch, "/v1/cluster/spec", endpoints.ContentTypeMergePatch, "", patch},
		do:   func(c client.Connection) (int, error) { return c.PatchClusterSpec([]byte(patch)) }},
	{name: "PatchClusterSpecIfMatch",
		want: received{http.MethodPatch, "/v1/cluster/spec", endpoints.ContentTypeMergePatch, etag, patch},
		do: func(c client.Connection) (int, error) {
			code, _, err := c.PatchClusterSpecIfMatch([]byte(patch), etag)
			return code, err
		}},
	{name: "Live", want: received{method: http.MethodGet, path: "/livez"},
		do: func(c client.Connection) (int, error) { return c.Live() }},
	{name: "Ready", want: received{method: http.MethodGet, path: "/readyz"},
		do: func(c client.Connection) (int, error) { return c.Ready() }},
	{name: "Mode", reply: `{"mode":"normal"}`, want: received{method: http.MethodGet, path: "/status"},
		do: func(c client.Connection) (int, error) { _, code, err := c.Mode(); return code, err }},
	{name: "WhoAmI", reply: `{}`, want: received{method: http.MethodGet, path: "/v1/whoami"},
		do: func(c client.Connection) (int, error) { _, code, err := c.WhoAmI(); return code, err }},
	{name: "FailKeeper", want: received{method: http.MethodPost, path: "/v1/cluster/keepers/k1/actions/fail"},
		do: func(c client.Connection) (int, error) { return c.FailKeeper("k1") }},
	{name: "DeleteKeeper", want: received{method: http.MethodDelete, path: "/v1/cluster/keepers/k1"},
		do: func(c client.Connection) (int, error) { return c.DeleteKeeper("k1") }},
	{name: "PromoteCluster", want: received{method: http.MethodPost, path: "/v1/cluster/actions/promote"},
		do: func(c client.Connection) (int, error) { return c.PromoteCluster() }},
	{name: "GetStatus", reply: `{}`, want: received{method: http.MethodGet, path: "/v1/cluster/status"},
		do: func(c client.Connection) (int, error) { _, code, err := c.GetStatus(); return code, err }},
	{name: "GetKeepers", reply: `[]`, want: received{method: http.MethodGet, path: "/v1/cluster/keepers"},
		do: func(c client.Connection) (int, error) { _, code, err := c.GetKeepers(); return code, err }},
	{name: "GetKeeper", reply: `{}`, want: received{method: http.MethodGet, path: "/v1/cluster/keepers/k1"},
		do: func(c client.Connection) (int, error) { _, code, err := c.GetKeeper("k1"); return code, err }},
	{name: "GetProxies", reply: `[]`, want: received{method: http.MethodGet, path: "/v1/cluster/proxies"},
		do: func(c client.Connection) (int, error) { _, code, err := c.GetProxies(); return code, err }},
	{name: "GetProxy", reply: `{}`, want: received{method: http.MethodGet, path: "/v1/cluster/proxies/p1"},
		do: func(c client.Connection) (int, error) { _, code, err := c.GetProxy("p1"); return code, err }},
	{name: "GetSentinels", reply: `[]`, want: received{method: http.MethodGet, path: "/v1/cluster/sentinels"},
		do: func(c client.Connection) (int, error) { _, code, err := c.GetSentinels(); return code, err }},
	{name: "GetSentinel", reply: `{}`, want: received{method: http.MethodGet, path: "/v1/cluster/sentinels/s1"},
		do: func(c client.Connection) (int, error) { _, code, err := c.GetSentinel("s1"); return code, err }},
}

// errorResponse is an error response of the api, and the problem the client should return for it
type errorResponse struct {
	name        string
	status      int
	contentType string
	body        string
	want        endpoints.Problem
}

var errorResponses = []errorResponse{
	{"a 404 problem", http.StatusNotFound, endpoints.ContentTypeProblem, mustJSON(endpoints.NewProblem(404, "no such keeper")),
		endpoints.NewProblem(http.StatusNotFound, "no such keeper")},
	{"a 409 problem", http.StatusConflict, endpoints.ContentTypeProblem, mustJSON(endpoints.NewProblem(409, "exists")),
		endpoints.NewProblem(http.StatusConflict, "exists")},
	{"a 412 problem", http.StatusPreconditionFailed, endpoints.ContentTypeProblem,
		mustJSON(endpoints.NewProblem(412, "changed")), endpoints.NewProblem(http.StatusPreconditionFailed, "changed")},
	{"a 500 problem", http.StatusInternalServerError, endpoints.ContentTypeProblem,
		mustJSON(endpoints.NewProblem(500, "store down")), endpoints.NewProblem(http.StatusInternalServerError, "store down")},
	{"a 503 problem without a detail", http.StatusServiceUnavailable, endpoints.ContentTypeProblem,
		`{"title":"Service Unavailable","status":503}`,
		endpoints.Problem{Title: "Service Unavailable", Status: http.StatusServiceUnavailable}},
	{"a 500 that isn't a problem", http.StatusInternalServerError, "text/plain", "boom\n",
		endpoints.NewProblem(http.StatusInternalServerError, "boom")},
	{"a 502 problem that isn't valid JSON", http.StatusBadGateway, endpoints.ContentTypeProblem, "{not json",
		endpoints.NewProblem(http.StatusBadGateway, "{not json")},
	{"a 400 problem without a status", http.StatusBadRequest, endpoints.ContentTypeProblem, `{"title":"x"}`,
		endpoints.NewProblem(http.StatusBadRequest, `{"title":"x"}`)},
	{"a 404 without a body", http.StatusNotFound, "", "", endpoints.NewProblem(http.StatusNotFound, "")},
}

var _ = Describe("every client call", func() {
	for _, tc := range calls {
		Describe(tc.name, func() {
			It("sends the request, and returns the http code of the response", func() {
				c, requests := record(respond(http.StatusOK, etag, tc.reply))
				httpCode, err := tc.do(c)
				Ω(err).NotTo(HaveOccurred())
				Ω(httpCode).To(Equal(http.StatusOK))
				Ω(*requests).To(Equal([]received{tc.want}))
			})
			for _, er := range errorResponses {
				It("returns the problem for "+er.name, func() {
					c, _ := record(func(w http.ResponseWriter, _ *http.Request) {
						if er.contentType != "" {
							w.Header().Set("Content-Type", er.contentType)
						}
						w.WriteHeader(er.status)
						_, _ = w.Write([]byte(er.body))
					})
					httpCode, err := tc.do(c)
					Ω(httpCode).To(Equal(er.status))
					Ω(err).To(MatchError(ContainSubstring(tc.want.method + " http://")))
					Ω(err).To(MatchError(ContainSubstring(tc.want.path + ": ")))
					var p endpoints.Problem
					Ω(errors.As(err, &p)).To(BeTrue())
					Ω(p).To(Equal(er.want))
				})
			}
			It("returns an error when the server refuses the connection", func() {
				httpCode, err := tc.do(refused())
				Ω(httpCode).To(Equal(-1))
				Ω(errors.Is(err, syscall.ECONNREFUSED)).To(BeTrue(), "%v", err)
			})
			if tc.reply != "" {
				It("returns an error when the response isn't valid JSON", func() {
					c, _ := record(respond(http.StatusOK, "", "not json"))
					httpCode, err := tc.do(c)
					Ω(httpCode).To(Equal(-1))
					Ω(err).To(MatchError(ContainSubstring("failed to decode the response")))
				})
			} else {
				It("ignores the body of the response", func() {
					c, _ := record(respond(http.StatusAccepted, "", "not json"))
					httpCode, err := tc.do(c)
					Ω(err).NotTo(HaveOccurred())
					Ω(httpCode).To(Equal(http.StatusAccepted))
				})
			}
		})
	}
})
