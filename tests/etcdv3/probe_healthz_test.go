package etcdv3_test

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

var _ = Describe("GET /healthz on the probe port", func() {
	It("returns 200 when the api and its store are healthy", func(ctx context.Context) {
		code, body := sharedCluster.probeGet(ctx, endpoints.HealthEndPoint)
		Ω(code).To(Equal(http.StatusOK))
		Ω(body).To(Equal("OK"))
	})
	It("works with the api client", func() {
		_, err := sharedCluster.probeClient.Healthy()
		Ω(err).NotTo(HaveOccurred())
	})
	It("only allows GET", func(ctx context.Context) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			sharedCluster.probeURL+"/"+string(endpoints.HealthEndPoint), nil)
		Ω(err).NotTo(HaveOccurred())
		resp, err := http.DefaultClient.Do(req)
		Ω(err).NotTo(HaveOccurred())
		_ = resp.Body.Close()
		Ω(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed))
	})
})

var _ = Describe("GET /healthz when the api or its store is unhealthy", Ordered, func() {
	// these tests pause etcd and stop the api, so they get their own api
	tc := withOwnAPI()

	It("returns 500 while the store doesn't respond, and 200 again when it is back", func(ctx context.Context) {
		pauseContainer(ctx, tc.etcd)
		code, body := tc.probeGet(ctx, endpoints.HealthEndPoint)
		Ω(code).To(Equal(http.StatusInternalServerError))
		Ω(body).To(Equal("ERROR"))

		unpauseContainer(ctx, tc.etcd)
		Eventually(func(g Gomega) {
			_, err := tc.probeClient.Healthy()
			g.Ω(err).NotTo(HaveOccurred())
		}).WithTimeout(time.Minute).WithPolling(pollInterval).Should(Succeed())
	})
	It("can't be reached when the api doesn't run", func(ctx context.Context) {
		Ω(tc.api.Stop(ctx, nil)).To(Succeed())
		_, err := tc.probeClient.Healthy()
		Ω(err).To(MatchError(ContainSubstring("connection refused")))
	})
})
