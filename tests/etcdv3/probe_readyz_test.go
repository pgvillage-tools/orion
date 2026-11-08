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
