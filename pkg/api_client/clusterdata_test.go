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

var _ = Describe("cluster data", func() {
	const body = `{"formatVersion":1,"cluster":{"uid":"c1","generation":4}}`

	It("GetCluster returns the cluster data", func() {
		c, _ := record(respond(http.StatusOK, "", body))
		cd, httpCode, err := c.GetCluster()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(cd.FormatVersion).To(BeEquivalentTo(1))
		Ω(cd.Cluster.UID).To(Equal("c1"))
		Ω(cd.Cluster.Generation).To(BeEquivalentTo(4))
	})
	It("GetCluster returns no cluster data on an error", func() {
		c, _ := record(func(w http.ResponseWriter, _ *http.Request) {
			endpoints.WriteProblem(w, http.StatusInternalServerError, "store down")
		})
		cd, httpCode, err := c.GetCluster()
		Ω(err).To(MatchError(ContainSubstring("store down")))
		Ω(httpCode).To(Equal(http.StatusInternalServerError))
		Ω(cd).To(BeNil())
	})
	It("GetClusterWithETag returns the cluster data and its ETag", func() {
		c, _ := record(respond(http.StatusOK, `"4"`, body))
		cd, etag, httpCode, err := c.GetClusterWithETag()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(etag).To(Equal(`"4"`))
		Ω(cd.Cluster.UID).To(Equal("c1"))
	})
	It("GetClusterWithETag returns no cluster data, but the ETag, on an error", func() {
		c, _ := record(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("ETag", `"4"`)
			endpoints.WriteProblem(w, http.StatusNotFound, "no cluster")
		})
		cd, etag, httpCode, err := c.GetClusterWithETag()
		Ω(err).To(MatchError(ContainSubstring("no cluster")))
		Ω(httpCode).To(Equal(http.StatusNotFound))
		Ω(etag).To(Equal(`"4"`))
		Ω(cd).To(BeNil())
	})
	It("GetClusterWithETag returns no ETag when the server refuses the connection", func() {
		cd, etag, httpCode, err := refused().GetClusterWithETag()
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(-1))
		Ω(etag).To(BeEmpty())
		Ω(cd).To(BeNil())
	})
	It("PutCluster returns the http code (e.g. 202)", func() {
		c, _ := record(respond(http.StatusAccepted, "", ""))
		httpCode, err := c.PutCluster(&apiv1.Data{})
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusAccepted))
	})
	It("PostCluster returns 201 when it created the cluster", func() {
		c, requests := record(respond(http.StatusCreated, "", ""))
		httpCode, err := c.PostCluster(&apiv1.Spec{})
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusCreated))
		Ω(*requests).To(HaveLen(1))
	})
	It("PutClusterIfMatch returns the new ETag", func() {
		c, requests := record(respond(http.StatusAccepted, `"5"`, ""))
		httpCode, newETag, err := c.PutClusterIfMatch(&apiv1.Data{}, `"4"`)
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusAccepted))
		Ω(newETag).To(Equal(`"5"`))
		Ω((*requests)[0].ifMatch).To(Equal(`"4"`))
	})
	It("PutClusterIfMatch sends no If-Match without an etag", func() {
		c, requests := record(respond(http.StatusAccepted, `"5"`, ""))
		_, _, err := c.PutClusterIfMatch(&apiv1.Data{}, "")
		Ω(err).NotTo(HaveOccurred())
		Ω((*requests)[0].ifMatch).To(BeEmpty())
	})
	It("PutClusterIfMatch returns no ETag when the server refuses the connection", func() {
		httpCode, newETag, err := refused().PutClusterIfMatch(&apiv1.Data{}, `"4"`)
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(-1))
		Ω(newETag).To(BeEmpty())
	})
})
