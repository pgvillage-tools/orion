package api_client_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

var _ = Describe("GetProxiesStatus", func() {
	It("returns the proxies by uid", func() {
		c := serve(endpoints.ProxyStatusEndPoint, http.StatusOK,
			`{"p1":{"UID":"p1","Generation":3},"p2":{"UID":"p2","Generation":1}}`)
		proxies, httpCode, err := c.GetProxiesStatus()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(proxies).To(HaveLen(2))
		Ω(proxies).To(HaveKeyWithValue("p1", HaveField("Generation", int64(3))))
	})
	It("returns the http code of an error", func() {
		c := serve(endpoints.ProxyStatusEndPoint, http.StatusInternalServerError, "ERROR")
		_, httpCode, err := c.GetProxiesStatus()
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusInternalServerError))
	})
})
