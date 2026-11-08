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

var _ = Describe("GET /status when the store is unhealthy", Ordered, func() {
	// this test pauses etcd, so it gets its own api
	tc := withOwnAPI()

	It("still returns mode normal while the store doesn't respond: the mode doesn't depend on the store",
		func(ctx context.Context) {
			pauseContainer(ctx, tc.etcd)
			mode, httpCode, err := tc.probeClient.Mode()
			Ω(err).NotTo(HaveOccurred())
			Ω(httpCode).To(Equal(http.StatusOK))
			Ω(mode.Mode).To(Equal(endpoints.ModeNormal))
		})
	// The bootstrap mode (an api without certificates, which only serves the bootstrap port) is designed in
	// docs/api/bootstrap-design.md, but not implemented yet
	PIt("returns mode bootstrap when the api runs without certificates")
})
