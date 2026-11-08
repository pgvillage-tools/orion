package api_client_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

var _ = Describe("GetProxies", func() {
	It("returns the proxies", func() {
		c := serve(endpoints.ProxiesEndPoint, http.StatusOK, `[{"uid":"p1","generation":3},{"uid":"p2","generation":1}]`)
		proxies, httpCode, err := c.GetProxies()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(proxies).To(Equal(apiv1.InfoProxies{{UID: "p1", Generation: 3}, {UID: "p2", Generation: 1}}))
	})
	It("returns the http code of an error", func() {
		c := serve(endpoints.ProxiesEndPoint, http.StatusInternalServerError, "ERROR")
		_, httpCode, err := c.GetProxies()
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusInternalServerError))
	})
})
