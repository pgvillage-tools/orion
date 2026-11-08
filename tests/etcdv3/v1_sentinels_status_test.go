package etcdv3_test

import (
	"context"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
)

var _ = Describe("GET /v1/cluster/sentinels/status", func() {
	It("returns the sentinel", func() {
		Eventually(func(g Gomega) {
			sentinels, httpCode, err := sharedCluster.apiClient.GetSentinelsStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(httpCode).To(Equal(http.StatusOK))
			g.Ω(sentinels).To(ConsistOf(HaveField("UID", Not(BeEmpty()))))
		}).WithTimeout(primaryTimeout).WithPolling(pollInterval).Should(Succeed())
	})
	It("matches the sentinels in /v1/cluster/status", func() {
		sentinels, _, err := sharedCluster.apiClient.GetSentinelsStatus()
		Ω(err).NotTo(HaveOccurred())
		status, _, err := sharedCluster.apiClient.GetStatus()
		Ω(err).NotTo(HaveOccurred())
		Ω(sentinels).To(HaveLen(len(status.Sentinels)))
		for i, s := range status.Sentinels {
			Ω(sentinels).To(ContainElement(HaveField("UID", s.UID)), "sentinel %d", i)
		}
	})
})

var _ = Describe("GET /v1/cluster/sentinels/status when sentinels stop", Ordered, func() {
	// these tests stop sentinels and pause etcd, so they get their own cluster (with one sentinel, so it is the leader)
	tc := withOwnCluster(1)
	var first string
	var second testcontainers.Container

	It("lists a second sentinel", func(ctx context.Context) {
		tc.waitForPrimary()
		status, _, err := tc.apiClient.GetStatus()
		Ω(err).NotTo(HaveOccurred())
		Ω(status.Sentinels).To(ConsistOf(HaveField("Leader", BeTrue())))
		first = status.Sentinels[0].UID

		second = tc.addSentinel(ctx)
		Eventually(func(g Gomega) {
			sentinels, httpCode, err := tc.apiClient.GetSentinelsStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(httpCode).To(Equal(http.StatusOK))
			g.Ω(sentinels).To(HaveLen(2))
		}).WithTimeout(primaryTimeout).WithPolling(pollInterval).Should(Succeed())
	})
	It("drops a stopped sentinel, and the other one takes over as the leader", func(ctx context.Context) {
		Ω(tc.sentinel.Stop(ctx, nil)).To(Succeed())
		// a sentinel registers itself with a TTL of 2 sleep intervals (10s by default)
		Eventually(func(g Gomega) {
			sentinels, _, err := tc.apiClient.GetSentinelsStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(sentinels).To(ConsistOf(HaveField("UID", Not(Equal(first)))))
			status, _, err := tc.apiClient.GetStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(status.Sentinels).To(ConsistOf(SatisfyAll(
				HaveField("UID", sentinels[0].UID),
				HaveField("Leader", BeTrue()),
			)))
		}).WithTimeout(primaryTimeout).WithPolling(pollInterval).Should(Succeed())
	})
	It("returns no sentinels when none runs", func(ctx context.Context) {
		Ω(second.Stop(ctx, nil)).To(Succeed())
		Eventually(func(g Gomega) {
			sentinels, httpCode, err := tc.apiClient.GetSentinelsStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(httpCode).To(Equal(http.StatusOK))
			g.Ω(sentinels).To(BeEmpty())
		}).WithTimeout(primaryTimeout).WithPolling(pollInterval).Should(Succeed())
	})
	It("returns 500 while the store doesn't respond", func(ctx context.Context) {
		pauseContainer(ctx, tc.etcd)
		_, httpCode, err := tc.apiClient.GetSentinelsStatus()
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusInternalServerError))
	})
})

var _ = Describe("GET /v1/cluster/sentinels/status without a cluster", Ordered, func() {
	// only etcd and the api: no cluster, no sentinels
	tc := withOwnAPI()

	It("returns no sentinels", func() {
		sentinels, httpCode, err := tc.apiClient.GetSentinelsStatus()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(sentinels).To(BeEmpty())
	})
})
