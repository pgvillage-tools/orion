package etcdv3_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

var _ = Describe("GET /v1/cluster/keepers, and a keeper, proxy or sentinel by uid", func() {
	uids := func(g Gomega) (keepers, proxies, sentinels []string) {
		status, _, err := sharedCluster.APIClient.GetStatus()
		g.Ω(err).NotTo(HaveOccurred())
		for _, k := range status.Keepers {
			keepers = append(keepers, k.UID)
		}
		for _, p := range status.Proxies {
			proxies = append(proxies, p.UID)
		}
		for _, s := range status.Sentinels {
			sentinels = append(sentinels, s.UID)
		}
		return keepers, proxies, sentinels
	}

	It("lists the keepers of the cluster status, sorted on uid", func() {
		sharedCluster.WaitForPrimary()
		keepers, httpCode, err := sharedCluster.APIClient.GetKeepers()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		statusKeepers, _, _ := uids(Default)
		Ω(keepers).To(HaveLen(helpers.DefaultKeepers))
		for i, k := range keepers {
			Ω(k.UID).To(Equal(statusKeepers[i]), "keeper %d", i)
		}
	})
	It("returns every keeper, proxy and sentinel by uid", func() {
		keepers, proxies, sentinels := uids(Default)
		Ω(proxies).NotTo(BeEmpty())
		Ω(sentinels).NotTo(BeEmpty())
		for _, uid := range keepers {
			k, httpCode, err := sharedCluster.APIClient.GetKeeper(uid)
			Ω(err).NotTo(HaveOccurred())
			Ω(httpCode).To(Equal(http.StatusOK))
			Ω(k.UID).To(Equal(uid))
		}
		for _, uid := range proxies {
			p, _, err := sharedCluster.APIClient.GetProxy(uid)
			Ω(err).NotTo(HaveOccurred())
			Ω(p.UID).To(Equal(uid))
		}
		for _, uid := range sentinels {
			s, _, err := sharedCluster.APIClient.GetSentinel(uid)
			Ω(err).NotTo(HaveOccurred())
			Ω(s.UID).To(Equal(uid))
		}
	})
	It("returns 404 for an unknown keeper, proxy or sentinel", func() {
		_, httpCode, err := sharedCluster.APIClient.GetKeeper("unknown")
		Ω(err).To(MatchError(ContainSubstring("keeper unknown doesn't exist")))
		Ω(httpCode).To(Equal(http.StatusNotFound))
		_, httpCode, _ = sharedCluster.APIClient.GetProxy("unknown")
		Ω(httpCode).To(Equal(http.StatusNotFound))
		_, httpCode, _ = sharedCluster.APIClient.GetSentinel("unknown")
		Ω(httpCode).To(Equal(http.StatusNotFound))
	})
})

var _ = Describe("GET /v1/cluster/keepers without a cluster", Ordered, func() {
	// only etcd and the api: no cluster data yet
	tc := helpers.WithOwnAPI()

	It("returns no keepers, and 404 for a keeper", func() {
		keepers, httpCode, err := tc.APIClient.GetKeepers()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(keepers).To(BeEmpty())

		_, httpCode, err = tc.APIClient.GetKeeper("keeper0")
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusNotFound))
	})
})
