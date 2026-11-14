package system_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

// The primary freezes (its container is paused: no answers, but no refused connections either): the sentinel fails
// over to the standby and the proxy follows. When the old primary wakes up, it must not go on as a primary (split
// brain): it steps down and follows the new primary.
var _ = Describe("a frozen primary", Ordered, func() {
	// freezes the primary, so it gets its own cluster
	tc := helpers.WithOwnCluster(2)
	const table = "frozen"
	const rows = 100
	var oldPrimary string

	count := func(ctx context.Context, conn helpers.PgConnParams) func() (int, error) {
		return func() (int, error) {
			return helpers.PgQueryInt(ctx, conn, fmt.Sprintf("select count(*) from %s", table))
		}
	}

	It("has data on the primary and the standby", func(ctx context.Context) {
		oldPrimary = tc.WaitForConverged(2)
		Eventually(func() error {
			return helpers.PgExec(ctx, tc.ProxyConn(ctx), fmt.Sprintf("create table %s (id int primary key)", table))
		}).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
		Ω(helpers.PgExec(ctx, tc.ProxyConn(ctx),
			fmt.Sprintf("insert into %s select generate_series(1, %d)", table, rows))).To(Succeed())
		for i := range tc.Keepers {
			Eventually(count(ctx, tc.KeeperConn(ctx, i))).WithContext(ctx).WithTimeout(30*time.Second).
				WithPolling(time.Second).Should(Equal(rows), "keeper %d", i)
		}
	})

	It("fails over to the standby while the primary is frozen", func(ctx context.Context) {
		helpers.PauseContainer(ctx, tc.Keepers[tc.KeeperIndex(oldPrimary)])

		Eventually(func(g Gomega) {
			status, _, err := tc.APIClient.GetStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(status.Status).To(HaveKeyWithValue("primaryKeeper", And(Not(BeEmpty()), Not(Equal(oldPrimary)))))
		}).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())

		// the proxy follows the new primary, which has every row and takes writes
		Eventually(count(ctx, tc.ProxyConn(ctx))).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).
			WithPolling(helpers.PollInterval).Should(Equal(rows))
		Ω(helpers.PgExec(ctx, tc.ProxyConn(ctx), fmt.Sprintf("insert into %s values (%d)", table, rows+1))).
			To(Succeed())

		// the old primary wakes up (in this spec: PauseContainer would unpause it after the spec)
		helpers.UnpauseContainer(ctx, tc.Keepers[tc.KeeperIndex(oldPrimary)])
	})

	It("steps down and follows the new primary when the old primary wakes up", func(ctx context.Context) {
		i := tc.KeeperIndex(oldPrimary)
		// no split brain: the old primary becomes a standby, with the row written after the failover
		Eventually(func() (int, error) {
			return helpers.PgQueryInt(ctx, tc.KeeperConn(ctx, i), "select pg_is_in_recovery()::int")
		}).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Equal(1))
		Eventually(count(ctx, tc.KeeperConn(ctx, i))).WithContext(ctx).WithTimeout(time.Minute).
			WithPolling(time.Second).Should(Equal(rows + 1))

		newPrimary := tc.WaitForConverged(2)
		Ω(newPrimary).NotTo(Equal(oldPrimary), "the old primary doesn't take over again")
		// the proxy still sends writes to the new primary
		Ω(helpers.PgExec(ctx, tc.ProxyConn(ctx), fmt.Sprintf("insert into %s values (%d)", table, rows+2))).
			To(Succeed())
	})
})
