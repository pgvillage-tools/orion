package etcdv3_test

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

var _ = Describe("GET /readyz on the probe port", func() {
	It("returns 200 when the api serves and its store answers", func(ctx context.Context) {
		code, body := sharedCluster.probeGet(ctx, endpoints.ReadyEndPoint)
		Ω(code).To(Equal(http.StatusOK))
		Ω(body).To(Equal("Ready"))
	})
	It("works with the api client", func() {
		httpCode, err := sharedCluster.probeClient.Ready()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
	})
})

var _ = Describe("GET /readyz when the api or its store is unhealthy", Ordered, func() {
	// these tests pause etcd and stop the api, so they get their own api
	tc := withOwnAPI()

	It("returns 503 while the store doesn't respond, and 200 again when it is back", func(ctx context.Context) {
		pauseContainer(ctx, tc.etcd)
		code, body := tc.probeGet(ctx, endpoints.ReadyEndPoint)
		Ω(code).To(Equal(http.StatusServiceUnavailable))
		Ω(body).To(Equal("Not Ready"))

		unpauseContainer(ctx, tc.etcd)
		Eventually(func(g Gomega) {
			httpCode, err := tc.probeClient.Ready()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(httpCode).To(Equal(http.StatusOK))
		}).WithTimeout(time.Minute).WithPolling(pollInterval).Should(Succeed())
	})
	It("returns 503 on SIGTERM, before the api stops", func(ctx context.Context) {
		signalContainer(ctx, tc.api, "SIGTERM")
		// the api waits --shutdown-delay (5s by default) before it stops
		Eventually(func(g Gomega) {
			httpCode, err := tc.probeClient.Ready()
			g.Ω(err).To(HaveOccurred())
			g.Ω(httpCode).To(Equal(http.StatusServiceUnavailable))
		}).WithTimeout(3 * time.Second).WithPolling(200 * time.Millisecond).Should(Succeed())

		Eventually(func(g Gomega) {
			_, err := tc.probeClient.Ready()
			g.Ω(err).To(MatchError(ContainSubstring("connection refused")))
		}).WithTimeout(30 * time.Second).WithPolling(time.Second).Should(Succeed())
	})
})
