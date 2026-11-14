package system_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

// etcd with TLS and client certificates: every orion component (the api, the sentinel, the keepers and the proxy)
// connects with --store-endpoints https://..., --store-cert-file, --store-key and --store-ca-file
var _ = Describe("a store that requires TLS with client certificates", Ordered, func() {
	tc := helpers.WithOwnClusterStoreTLS(2)

	It("only lets in clients with a client certificate", func(ctx context.Context) {
		code, out := tc.Etcdctl(ctx, true, "endpoint", "health")
		Ω(code).To(Equal(0), out)
		code, out = tc.Etcdctl(ctx, false, "endpoint", "health")
		Ω(code).NotTo(Equal(0), out)
	})

	It("runs the cluster: the keepers converge, and the proxy serves the primary", func(ctx context.Context) {
		tc.WaitForConverged(2)
		Eventually(func() error { return helpers.PgPing(ctx, tc.ProxyConn(ctx)) }).
			WithContext(ctx).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).Should(Succeed())
	})

	It("shows the sentinel and the proxy in the status of the api", func() {
		st, _, err := tc.APIClient.GetStatus()
		Ω(err).NotTo(HaveOccurred())
		Ω(st.Sentinels).To(HaveLen(1))
		Ω(st.Proxies).To(HaveLen(1))
	})
})
