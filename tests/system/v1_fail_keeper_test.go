package system_test

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

var _ = Describe("POST /v1/cluster/keepers/{uid}/actions/fail", Ordered, func() {
	// these tests fail over the primary, so they get their own cluster
	tc := helpers.WithOwnCluster(2)

	// waitForConverged waits until both keepers are healthy with their db at its wanted generation, and stay so for a
	// while (two sentinel checks), so they aren't between two generations
	waitForConverged := func() {
		converged := func(g Gomega) {
			status, _, err := tc.APIClient.GetStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(status.Keepers).To(HaveLen(2))
			for _, k := range status.Keepers {
				g.Ω(k.Healthy && k.PgHealthy).To(BeTrue(), "keeper %s", k.UID)
				g.Ω(k.PgCurrentGeneration).To(Equal(k.PgWantedGeneration), "keeper %s converged", k.UID)
			}
		}
		Eventually(converged).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
		Consistently(converged).WithTimeout(10 * time.Second).WithPolling(helpers.PollInterval).Should(Succeed())
	}

	It("accepts failing a standby: the primary stays, and the standby comes back", func(ctx context.Context) {
		primary := tc.WaitForPrimary()
		waitForConverged()
		cd, _, err := tc.APIClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())
		var standby string
		for uid := range cd.Keepers {
			if uid != primary {
				standby = uid
			}
		}
		Ω(standby).NotTo(BeEmpty())

		httpCode, err := tc.APIClient.FailKeeper(standby)
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusAccepted))

		// the sentinel doesn't fail over for a standby: the primary stays, and keeps serving through the proxy
		Consistently(func(g Gomega) {
			status, _, statusErr := tc.APIClient.GetStatus()
			g.Ω(statusErr).NotTo(HaveOccurred())
			g.Ω(status.Status).To(HaveKeyWithValue("primaryKeeper", primary))
		}).WithTimeout(15 * time.Second).WithPolling(helpers.PollInterval).Should(Succeed())
		Ω(helpers.PgPing(ctx, tc.ProxyConn(ctx))).To(Succeed())
		// and the standby is healthy and converged again
		waitForConverged()
	})

	It("fails over to the other keeper", func(ctx context.Context) {
		primary := tc.WaitForPrimary()
		// The standby must be able to take over: healthy, and its db converged (the sentinel only elects a standby
		// whose db is at its wanted generation). A fail is a one-shot signal: the sentinel treats the keeper as failed
		// in one check, and when no standby can take over in that check, the primary just stays.
		waitForConverged()

		httpCode, err := tc.APIClient.FailKeeper(primary)
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusAccepted))

		Eventually(func(g Gomega) {
			status, _, statusErr := tc.APIClient.GetStatus()
			g.Ω(statusErr).NotTo(HaveOccurred())
			g.Ω(status.Status).To(HaveKeyWithValue("primaryKeeper", And(Not(BeEmpty()), Not(Equal(primary)))))
		}).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())

		Eventually(func() error {
			return helpers.PgPing(ctx, tc.ProxyConn(ctx))
		}).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed(), "the proxy follows the new primary")
	})
	It("returns 404 for an unknown keeper", func() {
		httpCode, err := tc.APIClient.FailKeeper("unknown")
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusNotFound))
	})
})

var _ = Describe("POST /v1/cluster/keepers/{uid}/actions/fail without a cluster, or without the store", Ordered, func() {
	// only etcd and the api: no cluster data yet
	tc := helpers.WithOwnAPI()

	It("returns 404 without a cluster", func() {
		httpCode, err := tc.APIClient.FailKeeper("keeper0")
		Ω(err).To(MatchError(ContainSubstring("no cluster yet")))
		Ω(httpCode).To(Equal(http.StatusNotFound))
	})
	It("returns 500 while the store doesn't respond", func(ctx context.Context) {
		helpers.PauseContainer(ctx, tc.Etcd)
		httpCode, err := tc.APIClient.FailKeeper("keeper0")
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusInternalServerError))
	})
})

var _ = Describe("POST /v1/cluster/keepers/{uid}/actions/fail on the only keeper", Ordered, func() {
	// one keeper: it runs the primary, and nothing can take over
	tc := helpers.WithOwnCluster(1)

	It("refuses to fail it (409), and the primary stays", func() {
		primary := tc.WaitForPrimary()
		httpCode, err := tc.APIClient.FailKeeper(primary)
		Ω(err).To(MatchError(ContainSubstring("no other keeper can take over")))
		Ω(httpCode).To(Equal(http.StatusConflict))

		cd, _, err := tc.APIClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())
		Ω(cd.Keepers[primary].Status.ForceFail).To(BeFalse(), "the keeper isn't marked as failed")
		Ω(tc.WaitForPrimary()).To(Equal(primary))
	})
})
