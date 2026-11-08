package etcdv3_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GET /v1/cluster/status", func() {
	It("returns the keepers, sentinels, proxies and the primary", func() {
		primary := sharedCluster.waitForPrimary()
		Eventually(func(g Gomega) {
			status, httpCode, err := sharedCluster.apiClient.GetStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(httpCode).To(Equal(http.StatusOK))

			g.Ω(status.Keepers).To(HaveLen(defaultKeepers))
			var uids []string
			for _, k := range status.Keepers {
				g.Ω(k.Healthy).To(BeTrue(), "keeper %s", k.UID)
				g.Ω(k.PgHealthy).To(BeTrue(), "postgres of keeper %s", k.UID)
				g.Ω(k.PgCurrentGeneration).To(Equal(k.PgWantedGeneration), "keeper %s", k.UID)
				uids = append(uids, k.UID)
			}
			g.Ω(uids).To(ContainElement(primary))

			g.Ω(status.Sentinels).To(ConsistOf(HaveField("Leader", BeTrue())))
			g.Ω(status.Proxies).To(ConsistOf(HaveField("UID", Not(BeEmpty()))))
			g.Ω(status.Status).To(HaveKeyWithValue("available", true))
			g.Ω(status.Status).To(HaveKeyWithValue("primaryKeeper", primary))
			g.Ω(status.Status).To(HaveKeyWithValue("primaryDB", Not(BeEmpty())))
		}).WithTimeout(primaryTimeout).WithPolling(pollInterval).Should(Succeed())
	})
	It("returns the uids of the keepers in order", func() {
		status, _, err := sharedCluster.apiClient.GetStatus()
		Ω(err).NotTo(HaveOccurred())
		Ω(status.Keepers).To(HaveLen(defaultKeepers))
		Ω(status.Keepers[0].UID < status.Keepers[1].UID).To(BeTrue())
		Ω(status.Keepers).To(HaveEach(HaveField("ListenAddress", MatchRegexp(`^keeper_\d:\d+$`))))
	})
})
