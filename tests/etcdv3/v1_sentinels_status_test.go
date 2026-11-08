package etcdv3_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
