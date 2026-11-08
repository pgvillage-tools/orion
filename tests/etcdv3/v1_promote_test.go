package etcdv3_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
)

var _ = Describe("PUT /v1/cluster/promote", func() {
	// Promoting a primary cluster changes nothing (at most it sets the default role, primary, explicitly), so this
	// test can use the shared cluster. Promoting a standby cluster needs a second cluster that replicates from the
	// first; that is not covered yet.
	It("accepts promoting a cluster that is already primary, and keeps it primary", func() {
		primary := sharedCluster.waitForPrimary()

		httpCode, err := sharedCluster.apiClient.PutPromoteReplicaSet()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(BeElementOf(http.StatusOK, http.StatusAccepted))

		spec, _, err := sharedCluster.apiClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		if spec.Role != nil {
			Ω(*spec.Role).To(Equal(apiv1.Primary))
		}
		Ω(sharedCluster.waitForPrimary()).To(Equal(primary))
	})
	It("is idempotent", func() {
		httpCode, err := sharedCluster.apiClient.PutPromoteReplicaSet()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
	})
})
