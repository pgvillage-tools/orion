package etcdv3_test

import (
	"context"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/tests/helpers"
	"github.com/testcontainers/testcontainers-go"
)

var _ = Describe("GET /v1/cluster/sentinels", func() {
	It("returns the sentinel", func() {
		Eventually(func(g Gomega) {
			sentinels, httpCode, err := sharedCluster.APIClient.GetSentinels()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(httpCode).To(Equal(http.StatusOK))
			g.Ω(sentinels).To(ConsistOf(HaveField("UID", Not(BeEmpty()))))
		}).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
	})
	It("returns the same sentinels as /v1/cluster/status, with the leader", func() {
		sentinels, _, err := sharedCluster.APIClient.GetSentinels()
		Ω(err).NotTo(HaveOccurred())
		status, _, err := sharedCluster.APIClient.GetStatus()
		Ω(err).NotTo(HaveOccurred())
		Ω(sentinels).To(Equal(status.Sentinels))
		Ω(sentinels).To(ContainElement(HaveField("Leader", BeTrue())))
	})
})

var _ = Describe("GET /v1/cluster/sentinels when sentinels stop", Ordered, func() {
	// these tests stop sentinels and pause etcd, so they get their own cluster (with one sentinel, so it is the leader)
	tc := helpers.WithOwnCluster(1)
	var first string
	var second testcontainers.Container

	It("lists a second sentinel", func(ctx context.Context) {
		tc.WaitForPrimary()
		status, _, err := tc.APIClient.GetStatus()
		Ω(err).NotTo(HaveOccurred())
		Ω(status.Sentinels).To(ConsistOf(HaveField("Leader", BeTrue())))
		first = status.Sentinels[0].UID

		second = tc.AddSentinel(ctx)
		Eventually(func(g Gomega) {
			sentinels, httpCode, err := tc.APIClient.GetSentinels()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(httpCode).To(Equal(http.StatusOK))
			g.Ω(sentinels).To(HaveLen(2))
		}).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
	})
	It("drops a stopped sentinel, and the other one takes over as the leader", func(ctx context.Context) {
		Ω(tc.Sentinel.Stop(ctx, nil)).To(Succeed())
		// a sentinel registers itself with a TTL of 2 sleep intervals (10s by default)
		Eventually(func(g Gomega) {
			sentinels, _, err := tc.APIClient.GetSentinels()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(sentinels).To(ConsistOf(HaveField("UID", Not(Equal(first)))))
			status, _, err := tc.APIClient.GetStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(status.Sentinels).To(ConsistOf(SatisfyAll(
				HaveField("UID", sentinels[0].UID),
				HaveField("Leader", BeTrue()),
			)))
		}).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
	})
	It("returns no sentinels when none runs", func(ctx context.Context) {
		Ω(second.Stop(ctx, nil)).To(Succeed())
		Eventually(func(g Gomega) {
			sentinels, httpCode, err := tc.APIClient.GetSentinels()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(httpCode).To(Equal(http.StatusOK))
			g.Ω(sentinels).To(BeEmpty())
		}).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
	})
	It("returns 500 while the store doesn't respond", func(ctx context.Context) {
		helpers.PauseContainer(ctx, tc.Etcd)
		_, httpCode, err := tc.APIClient.GetSentinels()
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusInternalServerError))
	})
})

var _ = Describe("GET /v1/cluster/sentinels without a cluster", Ordered, func() {
	// only etcd and the api: no cluster, no sentinels
	tc := helpers.WithOwnAPI()

	It("returns no sentinels", func() {
		sentinels, httpCode, err := tc.APIClient.GetSentinels()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(sentinels).To(BeEmpty())
	})
})
