package etcdv3_test

import (
	"context"
	"net/http"

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
