package etcdv3_test

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("PUT /v1/cluster/keepers/{id}/fail", Ordered, func() {
	// these tests fail over the primary, so they get their own cluster
	tc := withOwnCluster(2)

	It("fails over to the other keeper", func(ctx context.Context) {
		primary := tc.waitForPrimary()
		// The standby must be able to take over: healthy, and its db converged (the sentinel only elects a standby
		// whose db is at its wanted generation). A fail is a one-shot signal: the sentinel treats the keeper as failed
		// in one check, and when no standby can take over in that check, the primary just stays.
		Eventually(func(g Gomega) {
			status, _, err := tc.apiClient.GetStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(status.Keepers).To(HaveLen(2))
			for _, k := range status.Keepers {
				g.Ω(k.Healthy && k.PgHealthy).To(BeTrue(), "keeper %s", k.UID)
				g.Ω(k.PgCurrentGeneration).To(Equal(k.PgWantedGeneration), "keeper %s converged", k.UID)
			}
		}).WithTimeout(primaryTimeout).WithPolling(pollInterval).Should(Succeed())
		// and stays so for a while (two sentinel checks), so it isn't between two generations
		Consistently(func(g Gomega) {
			status, _, err := tc.apiClient.GetStatus()
			g.Ω(err).NotTo(HaveOccurred())
			for _, k := range status.Keepers {
				g.Ω(k.Healthy && k.PgHealthy).To(BeTrue(), "keeper %s", k.UID)
				g.Ω(k.PgCurrentGeneration).To(Equal(k.PgWantedGeneration), "keeper %s converged", k.UID)
			}
		}).WithTimeout(10 * time.Second).WithPolling(pollInterval).Should(Succeed())

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
