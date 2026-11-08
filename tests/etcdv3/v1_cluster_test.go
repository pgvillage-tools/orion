package etcdv3_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/util"
)

var _ = Describe("GET /v1/cluster", func() {
	It("returns the cluster data", func() {
		sharedCluster.waitForPrimary()
		cd, httpCode, err := sharedCluster.apiClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(cd.Cluster).NotTo(BeNil())
		Ω(cd.Cluster.UID).NotTo(BeEmpty())
		Ω(cd.Cluster.Spec).NotTo(BeNil())
		Ω(cd.Cluster.Status.Master).NotTo(BeEmpty())
		Ω(cd.Keepers).To(HaveLen(defaultKeepers))
		Ω(cd.DBs).To(HaveKey(cd.Cluster.Status.Master))
	})
})

var _ = Describe("PUT and POST /v1/cluster", Ordered, func() {
	// these tests change the cluster data, so they get their own cluster
	tc := withOwnCluster(1)

	It("POST refuses to replace an existing cluster", func() {
		tc.waitForPrimary()
		before, _, err := tc.apiClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())

		httpCode, err := tc.apiClient.PostCluster(&apiv1.Spec{InitMode: util.ToPtr(apiv1.New)})
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusConflict))

		after, _, err := tc.apiClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())
		Ω(after.Cluster.UID).To(Equal(before.Cluster.UID))
	})
	It("PUT replaces the cluster data", func() {
		cd, _, err := tc.apiClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())
		if cd.Cluster.Spec.PGParameters == nil {
			cd.Cluster.Spec.PGParameters = apiv1.PGParameters{}
		}
		cd.Cluster.Spec.PGParameters["application_name"] = "put-by-e2e"

		httpCode, err := tc.apiClient.PutCluster(cd)
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))

		after, _, err := tc.apiClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())
		Ω(after.Cluster.UID).To(Equal(cd.Cluster.UID))
		Ω(after.Cluster.Spec.PGParameters).To(HaveKeyWithValue("application_name", "put-by-e2e"))
	})
	It("PUT refuses cluster data without a cluster", func() {
		httpCode, err := tc.apiClient.PutCluster(nil)
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusBadRequest))

		after, _, err := tc.apiClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())
		Ω(after.Cluster).NotTo(BeNil())
	})
})
