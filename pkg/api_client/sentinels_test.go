package api_client_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

var _ = Describe("GetSentinels", func() {
	It("returns the sentinels", func() {
		c := serve(endpoints.SentinelsEndPoint, http.StatusOK, `[{"uid":"s1","leader":true},{"uid":"s2","leader":false}]`)
		sentinels, httpCode, err := c.GetSentinels()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(sentinels).To(Equal(apiv1.InfoSentinels{{UID: "s1", Leader: true}, {UID: "s2"}}))
	})
	It("returns the http code of an error", func() {
		c := serve(endpoints.SentinelsEndPoint, http.StatusInternalServerError, "ERROR")
		_, httpCode, err := c.GetSentinels()
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusInternalServerError))
	})
})
