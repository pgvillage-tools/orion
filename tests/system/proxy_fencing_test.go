package system_test

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

// fencingTimeout is how long the proxy may take to stop serving once the store doesn't answer: the proxy timeout
// (15s by default) after its last successful check, plus a check interval and the store timeout
const fencingTimeout = time.Minute

// Without the store the proxy can't know whether its primary is still the primary, so it stops serving (fencing):
// it closes the open connections and stops listening, until it can check the store again.
var _ = Describe("the proxy while the store doesn't respond", Ordered, func() {
	// pauses etcd, so it gets its own cluster
	tc := helpers.WithOwnCluster(2)

	It("closes its connections and stops serving, and serves again when the store is back", func(ctx context.Context) {
		tc.WaitForPrimary()
		var conn *pgx.Conn
		Eventually(func() (err error) {
			conn, err = pgx.Connect(ctx, tc.ProxyConn(ctx).ConnString())
			return err
		}).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
		defer func() { _ = conn.Close(context.Background()) }()
		Ω(conn.Ping(ctx)).To(Succeed())

		helpers.PauseContainer(ctx, tc.Etcd)

		// the open connection is closed, and new connections fail
		Eventually(func() error { return conn.Ping(ctx) }).
			WithContext(ctx).WithTimeout(fencingTimeout).WithPolling(time.Second).ShouldNot(Succeed())
		Eventually(func() error { return helpers.PgPing(ctx, tc.ProxyConn(ctx)) }).
			WithContext(ctx).WithTimeout(fencingTimeout).WithPolling(time.Second).ShouldNot(Succeed())
		// and it stays so while the store is away
		Consistently(func() error { return helpers.PgPing(ctx, tc.ProxyConn(ctx)) }).
			WithContext(ctx).WithTimeout(10 * time.Second).WithPolling(time.Second).ShouldNot(Succeed())

		helpers.UnpauseContainer(ctx, tc.Etcd)

		Eventually(func() error { return helpers.PgPing(ctx, tc.ProxyConn(ctx)) }).
			WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
	})
})
