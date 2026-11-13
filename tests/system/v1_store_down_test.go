package system_test

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/util"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

var _ = Describe("the /v1 endpoints while the store doesn't respond", Ordered, func() {
	// these tests pause etcd, so they get their own api
	tc := helpers.WithOwnAPI()

	It("return 500 within the store timeout, and work again when the store is back", func(ctx context.Context) {
		httpCode, err := tc.APIClient.PostCluster(&apiv1.Spec{InitMode: util.ToPtr(apiv1.New)})
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusCreated))
		cd, _, err := tc.APIClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())

		calls := []struct {
			name string
			call func() (int, error)
		}{
			{"GET /v1/cluster", func() (int, error) { _, code, err := tc.APIClient.GetCluster(); return code, err }},
			{"PUT /v1/cluster", func() (int, error) { return tc.APIClient.PutCluster(cd) }},
			{"POST /v1/cluster", func() (int, error) {
				return tc.APIClient.PostCluster(&apiv1.Spec{InitMode: util.ToPtr(apiv1.New)})
			}},
			{"GET /v1/cluster/spec", func() (int, error) { _, code, err := tc.APIClient.GetClusterSpec(); return code, err }},
			{"PUT /v1/cluster/spec", func() (int, error) { return tc.APIClient.PutClusterSpec(cd.Cluster.Spec) }},
			{"PATCH /v1/cluster/spec", func() (int, error) {
				return tc.APIClient.PatchClusterSpec([]byte(`{"sleepInterval":"6s"}`))
			}},
			{"GET /v1/cluster/status", func() (int, error) { _, code, err := tc.APIClient.GetStatus(); return code, err }},
			{"GET /v1/cluster/keepers", func() (int, error) { _, code, err := tc.APIClient.GetKeepers(); return code, err }},
			{"GET /v1/cluster/keepers/{uid}", func() (int, error) {
				_, code, err := tc.APIClient.GetKeeper("keeper0")
				return code, err
			}},
			{"GET /v1/cluster/proxies", func() (int, error) { _, code, err := tc.APIClient.GetProxies(); return code, err }},
			{"GET /v1/cluster/proxies/{uid}", func() (int, error) {
				_, code, err := tc.APIClient.GetProxy("proxy0")
				return code, err
			}},
			{"GET /v1/cluster/sentinels", func() (int, error) {
				_, code, err := tc.APIClient.GetSentinels()
				return code, err
			}},
			{"GET /v1/cluster/sentinels/{uid}", func() (int, error) {
				_, code, err := tc.APIClient.GetSentinel("sentinel0")
				return code, err
			}},
			{"POST /v1/cluster/keepers/{uid}/actions/fail", func() (int, error) {
				return tc.APIClient.FailKeeper("keeper0")
			}},
			{"DELETE /v1/cluster/keepers/{uid}", func() (int, error) { return tc.APIClient.DeleteKeeper("keeper0") }},
			{"POST /v1/cluster/actions/promote", func() (int, error) { return tc.APIClient.PromoteCluster() }},
		}

		helpers.PauseContainer(ctx, tc.Etcd)
		for _, c := range calls {
			By(c.name)
			start := time.Now()
			httpCode, err := c.call()
			// the api gives up after its store timeout, well before the client does (helpers.APITimeout)
			Ω(time.Since(start)).To(BeNumerically("<", helpers.APIStoreTimeout+2*time.Second), c.name)
			Ω(err).To(HaveOccurred(), c.name)
			Ω(httpCode).To(Equal(http.StatusInternalServerError), c.name)
		}

		helpers.UnpauseContainer(ctx, tc.Etcd)
		Eventually(func(g Gomega) {
			after, httpCode, err := tc.APIClient.GetCluster()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(httpCode).To(Equal(http.StatusOK))
			g.Ω(after.Cluster.UID).To(Equal(cd.Cluster.UID))
		}).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).Should(Succeed())
	})
})
