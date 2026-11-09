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

var _ = Describe("cluster spec", func() {
	const body = `{"synchronousReplication":true,"additionalWalSenders":3}`

	It("GetClusterSpec returns the cluster spec", func() {
		c, _ := record(respond(http.StatusOK, "", body))
		s, httpCode, err := c.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(s.SynchronousReplication).To(HaveValue(BeTrue()))
		Ω(s.AdditionalWalSenders).To(HaveValue(BeEquivalentTo(3)))
	})
	It("GetClusterSpec returns no cluster spec on an error", func() {
		c, _ := record(func(w http.ResponseWriter, _ *http.Request) {
			endpoints.WriteProblem(w, http.StatusNotFound, "no cluster")
		})
		s, httpCode, err := c.GetClusterSpec()
		Ω(err).To(MatchError(ContainSubstring("no cluster")))
		Ω(httpCode).To(Equal(http.StatusNotFound))
		Ω(s).To(BeNil())
	})
	It("GetClusterSpecWithETag returns the cluster spec and its ETag", func() {
		c, _ := record(respond(http.StatusOK, `"9"`, body))
		s, etag, httpCode, err := c.GetClusterSpecWithETag()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(etag).To(Equal(`"9"`))
		Ω(s.AdditionalWalSenders).To(HaveValue(BeEquivalentTo(3)))
	})
	It("GetClusterSpecWithETag returns no cluster spec on an error", func() {
		c, _ := record(respond(http.StatusOK, `"9"`, "not json"))
		s, _, httpCode, err := c.GetClusterSpecWithETag()
		Ω(err).To(MatchError(ContainSubstring("failed to decode the response")))
		Ω(httpCode).To(Equal(-1))
		Ω(s).To(BeNil())
	})
	It("PutClusterSpec and PatchClusterSpec return the http code (e.g. 202)", func() {
		c, requests := record(respond(http.StatusAccepted, "", ""))
		httpCode, err := c.PutClusterSpec(&apiv1.Spec{})
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusAccepted))
		httpCode, err = c.PatchClusterSpec([]byte(`{}`))
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusAccepted))
		Ω(*requests).To(HaveLen(2))
	})

	// ifMatchCalls are the cluster spec calls that send If-Match, and return the new ETag
	ifMatchCalls := map[string]func(etag string) (int, string, error){}
	BeforeEach(func() {
		c, _ := record(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("ETag", `"10"`)
			if r.Header.Get("If-Match") == `"8"` {
				endpoints.WriteProblem(w, http.StatusPreconditionFailed, "the cluster data changed")
				return
			}
			w.WriteHeader(http.StatusAccepted)
		})
		ifMatchCalls["PutClusterSpecIfMatch"] = func(etag string) (int, string, error) {
			return c.PutClusterSpecIfMatch(&apiv1.Spec{}, etag)
		}
		ifMatchCalls["PatchClusterSpecIfMatch"] = func(etag string) (int, string, error) {
			return c.PatchClusterSpecIfMatch([]byte(`{}`), etag)
		}
	})
	for _, name := range []string{"PutClusterSpecIfMatch", "PatchClusterSpecIfMatch"} {
		It(name+" returns the new ETag", func() {
			httpCode, newETag, err := ifMatchCalls[name](`"9"`)
			Ω(err).NotTo(HaveOccurred())
			Ω(httpCode).To(Equal(http.StatusAccepted))
			Ω(newETag).To(Equal(`"10"`))
		})
		It(name+" returns 412 when the cluster data changed", func() {
			httpCode, _, err := ifMatchCalls[name](`"8"`)
			Ω(err).To(MatchError(HaveSuffix("412 Precondition Failed: the cluster data changed")))
			Ω(httpCode).To(Equal(http.StatusPreconditionFailed))
		})
		It(name+" changes the spec unconditionally without an etag", func() {
			httpCode, _, err := ifMatchCalls[name]("")
			Ω(err).NotTo(HaveOccurred())
			Ω(httpCode).To(Equal(http.StatusAccepted))
		})
	}
})
