// Package etcdv3_test will run integration tests for using etcd as backend with v3 api
package etcdv3_test

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/pki"
)

var _ = Describe("Smoke", func() {
	Context("when connecting to the keepers", func() {
		It("should work properly", func(ctx context.Context) {
			for i := range sharedCluster.keepers {
				Ω(pgPing(ctx, sharedCluster.keeperConn(ctx, i))).To(Succeed())
			}
		})
	})
	Context("when connecting through proxy", func() {
		It("should work properly", func(ctx context.Context) {
			// This does not work directly after starting the container but does after 5s.
			// So, we will try this for 10 seconds
			isReadyCtx, cancelFunc := context.WithTimeout(ctx, 10*time.Second)
			defer cancelFunc()
			Ω(isReady(isReadyCtx, sharedCluster.proxyConn(ctx), time.Second)).To(Succeed())
		})
	})
	Context("when using the api", func() {
		It("identifies the client by its certificate", func() {
			identity, httpCode, err := sharedCluster.apiClient.WhoAmI()
			Ω(err).NotTo(HaveOccurred())
			Ω(httpCode).To(Equal(http.StatusOK))
			Ω(identity.Name).To(Equal(pki.CLIClientName))
		})
		It("status return expected result", func() {
			sharedCluster.waitForPrimary()
			myStatus, httpCode, statusErr := sharedCluster.apiClient.GetStatus()
			Ω(statusErr).NotTo(HaveOccurred())
			Ω(httpCode).To(Equal(http.StatusOK))

			Ω(myStatus.Keepers).To(HaveLen(defaultKeepers))
			Ω(myStatus.Sentinels).To(HaveLen(1))
			Ω(myStatus.Proxies).To(HaveLen(1))
		})
	})
})
