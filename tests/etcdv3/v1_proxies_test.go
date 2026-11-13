package etcdv3_test

import (
	"context"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

var _ = Describe("GET /v1/cluster/proxies", func() {
	It("returns the proxy", func() {
		Eventually(func(g Gomega) {
			proxies, httpCode, err := sharedCluster.APIClient.GetProxies()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(httpCode).To(Equal(http.StatusOK))
			g.Ω(proxies).To(ConsistOf(HaveField("UID", Not(BeEmpty()))))
		}).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
	})
	It("returns the same proxies as /v1/cluster/status", func() {
		proxies, _, err := sharedCluster.APIClient.GetProxies()
		Ω(err).NotTo(HaveOccurred())
		status, _, err := sharedCluster.APIClient.GetStatus()
		Ω(err).NotTo(HaveOccurred())
		Ω(proxies).To(Equal(status.Proxies))
	})
})

var _ = Describe("GET /v1/cluster/proxies when the proxy stops", Ordered, func() {
	// these tests stop the proxy and pause etcd, so they get their own cluster
	tc := helpers.WithOwnCluster(1)

	It("drops a stopped proxy", func(ctx context.Context) {
		Eventually(func(g Gomega) {
			proxies, _, err := tc.APIClient.GetProxies()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(proxies).To(HaveLen(1))
		}).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())

		Ω(tc.Proxy.Stop(ctx, nil)).To(Succeed())
		// a proxy registers itself with a TTL of 2 proxy timeouts (30s by default)
		Eventually(func(g Gomega) {
			proxies, httpCode, err := tc.APIClient.GetProxies()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(httpCode).To(Equal(http.StatusOK))
			g.Ω(proxies).To(BeEmpty())
		}).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
	})
	It("returns 500 while the store doesn't respond", func(ctx context.Context) {
		helpers.PauseContainer(ctx, tc.Etcd)
		_, httpCode, err := tc.APIClient.GetProxies()
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusInternalServerError))
	})
})

var _ = Describe("GET /v1/cluster/proxies without a cluster", Ordered, func() {
	// only etcd and the api: no cluster, no proxies
	tc := helpers.WithOwnAPI()

	It("returns no proxies", func() {
		proxies, httpCode, err := tc.APIClient.GetProxies()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(proxies).To(BeEmpty())
	})
})
