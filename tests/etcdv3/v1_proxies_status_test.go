package etcdv3_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GET /v1/cluster/proxies/status", func() {
	It("returns the proxy, by uid", func() {
		Eventually(func(g Gomega) {
			proxies, httpCode, err := sharedCluster.apiClient.GetProxiesStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(httpCode).To(Equal(http.StatusOK))
			g.Ω(proxies).To(HaveLen(1))
			for uid, proxy := range proxies {
				g.Ω(proxy.UID).To(Equal(uid))
			}
		}).WithTimeout(primaryTimeout).WithPolling(pollInterval).Should(Succeed())
	})
	It("matches the proxies in /v1/cluster/status", func() {
		proxies, _, err := sharedCluster.apiClient.GetProxiesStatus()
		Ω(err).NotTo(HaveOccurred())
		status, _, err := sharedCluster.apiClient.GetStatus()
		Ω(err).NotTo(HaveOccurred())
		for _, p := range status.Proxies {
			Ω(proxies).To(HaveKey(p.UID))
		}
	})
})
