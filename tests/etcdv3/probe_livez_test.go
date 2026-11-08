package etcdv3_test

import (
	"context"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

var _ = Describe("GET /livez on the probe port", func() {
	It("returns 200 while the api runs", func(ctx context.Context) {
		code, body := sharedCluster.probeGet(ctx, endpoints.LiveEndPoint)
		Ω(code).To(Equal(http.StatusOK))
		Ω(body).To(Equal("OK"))
	})
	It("works with the api client", func() {
		httpCode, err := sharedCluster.probeClient.Live()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
	})
	It("only allows GET", func(ctx context.Context) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			sharedCluster.probeURL+"/"+string(endpoints.LiveEndPoint), nil)
		Ω(err).NotTo(HaveOccurred())
		resp, err := http.DefaultClient.Do(req)
		Ω(err).NotTo(HaveOccurred())
		_ = resp.Body.Close()
		Ω(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed))
	})
})

var _ = Describe("GET /livez when the api or its store is unhealthy", Ordered, func() {
	// these tests pause etcd and stop the api, so they get their own api
	tc := withOwnAPI()

	It("stays 200 while the store doesn't respond: liveness doesn't depend on the store", func(ctx context.Context) {
		pauseContainer(ctx, tc.etcd)
		code, body := tc.probeGet(ctx, endpoints.LiveEndPoint)
		Ω(code).To(Equal(http.StatusOK))
		Ω(body).To(Equal("OK"))
	})
	It("can't be reached when the api doesn't run", func(ctx context.Context) {
		Ω(tc.api.Stop(ctx, nil)).To(Succeed())
		_, err := tc.probeClient.Live()
		Ω(err).To(MatchError(ContainSubstring("connection refused")))
	})
})
