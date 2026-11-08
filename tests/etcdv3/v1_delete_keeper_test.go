package etcdv3_test

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DELETE /v1/cluster/keepers/{id}", Ordered, func() {
	// these tests remove a keeper, so they get their own cluster
	const numKeepers = 2
	tc := withOwnCluster(numKeepers)

	It("removes a stopped keeper from the cluster data", func(ctx context.Context) {
		primary := tc.waitForPrimary()
		status, _, err := tc.apiClient.GetStatus()
		Ω(err).NotTo(HaveOccurred())
		Ω(status.Keepers).To(HaveLen(numKeepers))

		// stop the standby; its listen address is keeper_<index>:<port>
		var standby string
		for _, k := range status.Keepers {
			if k.UID == primary {
				continue
			}
			standby = k.UID
			alias, _, found := strings.Cut(k.ListenAddress, ":")
			Ω(found).To(BeTrue(), "listen address %q", k.ListenAddress)
			index, convErr := strconv.Atoi(strings.TrimPrefix(alias, "keeper_"))
			Ω(convErr).NotTo(HaveOccurred())
			Ω(tc.keepers[index].Stop(ctx, nil)).To(Succeed())
		}
		Ω(standby).NotTo(BeEmpty())

		httpCode, err := tc.apiClient.DeleteKeeper(standby)
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusAccepted))

		cd, _, err := tc.apiClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())
		Ω(cd.Keepers).NotTo(HaveKey(standby))
		Ω(cd.Keepers).To(HaveKey(primary))
	})
	It("returns 404 for an unknown keeper", func() {
		httpCode, err := tc.apiClient.DeleteKeeper("unknown")
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusNotFound))
	})
})
