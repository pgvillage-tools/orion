package system_test

import (
	"context"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

var _ = Describe("POST /v1/cluster/actions/promote", func() {
	// Promoting a primary cluster changes nothing (also when its spec has no role: primary is the default), so this
	// test can use the shared cluster. Promoting a standby cluster needs a second cluster that replicates from the
	// first; that is not covered yet.
	It("does nothing for a cluster that is already primary", func() {
		primary := sharedCluster.WaitForPrimary()
		before, _, err := sharedCluster.APIClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())

		httpCode, err := sharedCluster.APIClient.PromoteCluster()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))

		after, _, err := sharedCluster.APIClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		Ω(after.Role).To(Equal(before.Role), "the spec doesn't change (the role stays unset: primary is the default)")
		Ω(sharedCluster.WaitForPrimary()).To(Equal(primary))
	})
	It("is idempotent", func() {
		httpCode, err := sharedCluster.APIClient.PromoteCluster()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
	})
})

var _ = Describe("POST /v1/cluster/actions/promote without a cluster, or without the store", Ordered, func() {
	// only etcd and the api: no cluster data yet
	tc := helpers.WithOwnAPI()

	It("returns 404 without a cluster", func() {
		httpCode, err := tc.APIClient.PromoteCluster()
		Ω(err).To(MatchError(ContainSubstring("no cluster yet")))
		Ω(httpCode).To(Equal(http.StatusNotFound))
	})
	It("returns 500 while the store doesn't respond", func(ctx context.Context) {
		helpers.PauseContainer(ctx, tc.Etcd)
		httpCode, err := tc.APIClient.PromoteCluster()
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusInternalServerError))
	})
})
