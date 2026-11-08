package etcdv3_test

import (
	"context"
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

var _ = Describe("GET /v1/cluster/proxies/status when the proxy stops", Ordered, func() {
	// these tests stop the proxy and pause etcd, so they get their own cluster
	tc := withOwnCluster(1)

	It("drops a stopped proxy", func(ctx context.Context) {
		Eventually(func(g Gomega) {
			proxies, _, err := tc.apiClient.GetProxiesStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(proxies).To(HaveLen(1))
		}).WithTimeout(primaryTimeout).WithPolling(pollInterval).Should(Succeed())

		Ω(tc.proxy.Stop(ctx, nil)).To(Succeed())
		// a proxy registers itself with a TTL of 2 proxy timeouts (30s by default)
		Eventually(func(g Gomega) {
			proxies, httpCode, err := tc.apiClient.GetProxiesStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(httpCode).To(Equal(http.StatusOK))
			g.Ω(proxies).To(BeEmpty())
		}).WithTimeout(primaryTimeout).WithPolling(pollInterval).Should(Succeed())
	})
	It("returns 500 while the store doesn't respond", func(ctx context.Context) {
		pauseContainer(ctx, tc.etcd)
		_, httpCode, err := tc.apiClient.GetProxiesStatus()
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusInternalServerError))
	})
})

var _ = Describe("GET /v1/cluster/proxies/status without a cluster", Ordered, func() {
	// only etcd and the api: no cluster, no proxies
	tc := withOwnAPI()

	It("returns no proxies", func() {
		proxies, httpCode, err := tc.apiClient.GetProxiesStatus()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(proxies).To(BeEmpty())
	})
})
