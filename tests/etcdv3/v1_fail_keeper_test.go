package etcdv3_test

import (
	"context"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("PUT /v1/cluster/keepers/{id}/fail", Ordered, func() {
	// these tests fail over the primary, so they get their own cluster
	tc := withOwnCluster(2)

	It("fails over to the other keeper", func(ctx context.Context) {
		primary := tc.waitForPrimary()
		// the standby must be in sync before it can take over
		Eventually(func(g Gomega) {
			status, _, err := tc.apiClient.GetStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(status.Keepers).To(HaveLen(2))
			for _, k := range status.Keepers {
				g.Ω(k.Healthy && k.PgHealthy).To(BeTrue(), "keeper %s", k.UID)
			}
		}).WithTimeout(primaryTimeout).WithPolling(pollInterval).Should(Succeed())

		httpCode, err := tc.apiClient.PutFailKeeper(primary)
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusAccepted))

		Eventually(func(g Gomega) {
			status, _, statusErr := tc.apiClient.GetStatus()
			g.Ω(statusErr).NotTo(HaveOccurred())
			g.Ω(status.Status).To(HaveKeyWithValue("primaryKeeper", And(Not(BeEmpty()), Not(Equal(primary)))))
		}).WithTimeout(primaryTimeout).WithPolling(pollInterval).Should(Succeed())

		Eventually(func() error {
			return pgPing(ctx, tc.proxyConn(ctx))
		}).WithTimeout(primaryTimeout).WithPolling(pollInterval).Should(Succeed(), "the proxy follows the new primary")
	})
	It("returns 404 for an unknown keeper", func() {
		httpCode, err := tc.apiClient.PutFailKeeper("unknown")
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusNotFound))
	})
})
