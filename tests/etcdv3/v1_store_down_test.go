package etcdv3_test

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/util"
)

var _ = Describe("the /v1 endpoints while the store doesn't respond", Ordered, func() {
	// these tests pause etcd, so they get their own api
	tc := withOwnAPI()

	It("return 500 within the store timeout, and work again when the store is back", func(ctx context.Context) {
		httpCode, err := tc.apiClient.PostCluster(&apiv1.Spec{InitMode: util.ToPtr(apiv1.New)})
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusCreated))
		cd, _, err := tc.apiClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())

		calls := []struct {
			name string
			call func() (int, error)
		}{
			{"GET /v1/cluster", func() (int, error) { _, code, err := tc.apiClient.GetCluster(); return code, err }},
			{"PUT /v1/cluster", func() (int, error) { return tc.apiClient.PutCluster(cd) }},
			{"POST /v1/cluster", func() (int, error) {
				return tc.apiClient.PostCluster(&apiv1.Spec{InitMode: util.ToPtr(apiv1.New)})
			}},
			{"GET /v1/cluster/spec", func() (int, error) { _, code, err := tc.apiClient.GetClusterSpec(); return code, err }},
			{"PUT /v1/cluster/spec", func() (int, error) { return tc.apiClient.PutClusterSpec(cd.Cluster.Spec) }},
			{"PATCH /v1/cluster/spec", func() (int, error) {
				return tc.apiClient.PatchClusterSpec([]byte(`{"sleepInterval":"6s"}`))
			}},
			{"GET /v1/cluster/status", func() (int, error) { _, code, err := tc.apiClient.GetStatus(); return code, err }},
			{"GET /v1/cluster/keepers", func() (int, error) { _, code, err := tc.apiClient.GetKeepers(); return code, err }},
			{"GET /v1/cluster/keepers/{uid}", func() (int, error) {
				_, code, err := tc.apiClient.GetKeeper("keeper0")
				return code, err
			}},
			{"GET /v1/cluster/proxies", func() (int, error) { _, code, err := tc.apiClient.GetProxies(); return code, err }},
			{"GET /v1/cluster/proxies/{uid}", func() (int, error) {
				_, code, err := tc.apiClient.GetProxy("proxy0")
				return code, err
			}},
			{"GET /v1/cluster/sentinels", func() (int, error) {
				_, code, err := tc.apiClient.GetSentinels()
				return code, err
			}},
			{"GET /v1/cluster/sentinels/{uid}", func() (int, error) {
				_, code, err := tc.apiClient.GetSentinel("sentinel0")
				return code, err
			}},
			{"POST /v1/cluster/keepers/{uid}/actions/fail", func() (int, error) {
				return tc.apiClient.FailKeeper("keeper0")
			}},
			{"DELETE /v1/cluster/keepers/{uid}", func() (int, error) { return tc.apiClient.DeleteKeeper("keeper0") }},
			{"POST /v1/cluster/actions/promote", func() (int, error) { return tc.apiClient.PromoteCluster() }},
		}

		pauseContainer(ctx, tc.etcd)
		for _, c := range calls {
			By(c.name)
			start := time.Now()
			httpCode, err := c.call()
			// the api gives up after its store timeout, well before the client does (apiTimeout)
			Ω(time.Since(start)).To(BeNumerically("<", apiStoreTimeout+2*time.Second), c.name)
			Ω(err).To(HaveOccurred(), c.name)
			Ω(httpCode).To(Equal(http.StatusInternalServerError), c.name)
		}

		unpauseContainer(ctx, tc.etcd)
		Eventually(func(g Gomega) {
			after, httpCode, err := tc.apiClient.GetCluster()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(httpCode).To(Equal(http.StatusOK))
			g.Ω(after.Cluster.UID).To(Equal(cd.Cluster.UID))
		}).WithTimeout(time.Minute).WithPolling(pollInterval).Should(Succeed())
	})
})
