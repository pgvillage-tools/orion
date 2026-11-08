package etcdv3_test

import (
	"context"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

var _ = Describe("GET /readyz on the probe port", func() {
	It("returns 200 when the api serves", func(ctx context.Context) {
		code, body := sharedCluster.probeGet(ctx, endpoints.ReadyEndPoint)
		Ω(code).To(Equal(http.StatusOK))
		Ω(body).To(Equal("Ready"))
	})
})

var _ = Describe("GET /readyz when the api or its store is unhealthy", Ordered, func() {
	// these tests pause etcd and stop the api, so they get their own api
	tc := withOwnAPI()

	It("stays ready while the store doesn't respond: readiness doesn't depend on the store", func(ctx context.Context) {
		pauseContainer(ctx, tc.etcd)
		code, body := tc.probeGet(ctx, endpoints.ReadyEndPoint)
		Ω(code).To(Equal(http.StatusOK))
		Ω(body).To(Equal("Ready"))
	})
	It("can't be reached when the api doesn't run", func(ctx context.Context) {
		Ω(tc.api.Stop(ctx, nil)).To(Succeed())
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			tc.probeURL+"/"+string(endpoints.ReadyEndPoint), nil)
		Ω(err).NotTo(HaveOccurred())
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
		Ω(err).To(MatchError(ContainSubstring("connection refused")))
	})
})
