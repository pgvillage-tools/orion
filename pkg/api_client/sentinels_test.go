package api_client_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

var _ = Describe("GetSentinelsStatus", func() {
	It("returns the sentinels", func() {
		c := serve(endpoints.SentinelStatusEndPoint, http.StatusOK, `[{"UID":"s1"},{"UID":"s2"}]`)
		sentinels, httpCode, err := c.GetSentinelsStatus()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(sentinels).To(ConsistOf(HaveField("UID", "s1"), HaveField("UID", "s2")))
	})
	It("returns the http code of an error", func() {
		c := serve(endpoints.SentinelStatusEndPoint, http.StatusInternalServerError, "ERROR")
		_, httpCode, err := c.GetSentinelsStatus()
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusInternalServerError))
	})
})
