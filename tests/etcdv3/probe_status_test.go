package etcdv3_test

import (
	"context"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

var _ = Describe("GET /status on the probe port", func() {
	It("returns the mode of the api: normal, with certificates", func() {
		mode, httpCode, err := sharedCluster.probeClient.Mode()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(mode.Mode).To(Equal(endpoints.ModeNormal))
	})
	It("returns JSON", func(ctx context.Context) {
		code, body := sharedCluster.probeGet(ctx, endpoints.ModeEndPoint)
		Ω(code).To(Equal(http.StatusOK))
		Ω(body).To(MatchJSON(`{"mode":"normal"}`))
	})
})
