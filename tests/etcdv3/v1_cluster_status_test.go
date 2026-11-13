package etcdv3_test

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

var _ = Describe("GET /v1/cluster/status", func() {
	It("returns the keepers, sentinels, proxies and the primary", func() {
		primary := sharedCluster.WaitForPrimary()
		Eventually(func(g Gomega) {
			status, httpCode, err := sharedCluster.APIClient.GetStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(httpCode).To(Equal(http.StatusOK))

			g.Ω(status.Keepers).To(HaveLen(helpers.DefaultKeepers))
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
			g.Ω(status.Status).To(HaveKeyWithValue("role", string(apiv1.Primary)))
		}).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
	})
	It("returns the uids of the keepers in order", func() {
		status, _, err := sharedCluster.APIClient.GetStatus()
		Ω(err).NotTo(HaveOccurred())
		Ω(status.Keepers).To(HaveLen(helpers.DefaultKeepers))
		Ω(status.Keepers[0].UID < status.Keepers[1].UID).To(BeTrue())
		Ω(status.Keepers).To(HaveEach(HaveField("ListenAddress", MatchRegexp(`^keeper_\d:\d+$`))))
	})
})

var _ = Describe("GET /v1/cluster/status when the cluster is unhealthy", Ordered, func() {
	// these tests stop a keeper and pause etcd, so they get their own cluster
	const numKeepers = 2
	tc := helpers.WithOwnCluster(numKeepers)

	It("shows a stopped standby as unhealthy, and keeps the primary", func(ctx context.Context) {
		primary := tc.WaitForPrimary()
		// wait until both keepers run and reported their listen address, so the stopped one was healthy before
		var standby, listenAddress string
		Eventually(func(g Gomega) {
			status, _, err := tc.APIClient.GetStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(status.Keepers).To(HaveLen(numKeepers))
			for _, k := range status.Keepers {
				g.Ω(k.Healthy).To(BeTrue(), "keeper %s", k.UID)
				// it is "(unknown)" until the db of the keeper reports it
				g.Ω(k.ListenAddress).To(MatchRegexp(`^keeper_\d+:\d+$`), "keeper %s", k.UID)
				if k.UID != primary {
					standby, listenAddress = k.UID, k.ListenAddress
				}
			}
		}).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())

		// stop the standby; its listen address is keeper_<index>:<port>
		alias, _, found := strings.Cut(listenAddress, ":")
		Ω(found).To(BeTrue(), "listen address %q", listenAddress)
		index, err := strconv.Atoi(strings.TrimPrefix(alias, "keeper_"))
		Ω(err).NotTo(HaveOccurred())
		Ω(tc.Keepers[index].Stop(ctx, nil)).To(Succeed())

		Eventually(func(g Gomega) {
			status, httpCode, err := tc.APIClient.GetStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(httpCode).To(Equal(http.StatusOK))
			g.Ω(status.Keepers).To(ContainElement(SatisfyAll(
				HaveField("UID", standby),
				HaveField("Healthy", BeFalse()),
			)))
			g.Ω(status.Keepers).To(ContainElement(SatisfyAll(
				HaveField("UID", primary),
				HaveField("Healthy", BeTrue()),
				HaveField("PgHealthy", BeTrue()),
			)))
			g.Ω(status.Status).To(HaveKeyWithValue("available", true))
			g.Ω(status.Status).To(HaveKeyWithValue("primaryKeeper", primary))
		}).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
	})
	It("returns 500 while the store doesn't respond", func(ctx context.Context) {
		helpers.PauseContainer(ctx, tc.Etcd)
		_, httpCode, err := tc.APIClient.GetStatus()
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusInternalServerError))
	})
})

var _ = Describe("GET /v1/cluster/status without a cluster", Ordered, func() {
	// only etcd and the api: no cluster data yet
	tc := helpers.WithOwnAPI()

	It("returns an unavailable cluster without keepers, sentinels or proxies", func() {
		status, httpCode, err := tc.APIClient.GetStatus()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(status.Status).To(Equal(apiv1.InfoGenericStatus{"available": false}))
		Ω(status.Keepers).To(BeEmpty())
		Ω(status.Sentinels).To(BeEmpty())
		Ω(status.Proxies).To(BeEmpty())
	})
})
