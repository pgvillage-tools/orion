package etcdv3_test

import (
	"net/http"
	"sync"

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

// createConcurrently calls create n times at the same time, and returns the status codes of the calls
func createConcurrently(n int, create func() (int, error)) []int {
	codes := make([]int, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer GinkgoRecover()
			defer wg.Done()
			<-start
			codes[i], _ = create()
		}()
	}
	close(start)
	wg.Wait()
	return codes
}

var _ = Describe("POST /v1/cluster from several clients at the same time", Ordered, func() {
	// only etcd and the api: no cluster data yet
	tc := withOwnAPI()

	It("creates the cluster once: one client gets 202, the others 409", func() {
		codes := createConcurrently(10, func() (int, error) {
			return tc.apiClient.PostCluster(&apiv1.Spec{InitMode: util.ToPtr(apiv1.New)})
		})
		Ω(codes).To(HaveEach(BeElementOf(http.StatusAccepted, http.StatusConflict)), "codes %v", codes)
		accepted := 0
		for _, c := range codes {
			if c == http.StatusAccepted {
				accepted++
			}
		}
		Ω(accepted).To(Equal(1), "codes %v", codes)

		cd, _, err := tc.apiClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())
		Ω(cd.Cluster).NotTo(BeNil())
	})
})

var _ = Describe("POST /v1/cluster/spec from several clients at the same time", Ordered, func() {
	// only etcd and the api: no cluster data yet
	tc := withOwnAPI()

	It("sets the spec once: one client gets 202, the others 409", func() {
		codes := createConcurrently(10, func() (int, error) {
			return tc.apiClient.PostClusterSpec(&apiv1.Spec{InitMode: util.ToPtr(apiv1.New)})
		})
		Ω(codes).To(HaveEach(BeElementOf(http.StatusAccepted, http.StatusConflict)), "codes %v", codes)
		accepted := 0
		for _, c := range codes {
			if c == http.StatusAccepted {
				accepted++
			}
		}
		Ω(accepted).To(Equal(1), "codes %v", codes)

		spec, _, err := tc.apiClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		Ω(spec.InitMode).To(HaveValue(Equal(apiv1.New)))
	})
})
