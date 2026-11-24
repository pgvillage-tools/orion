package system_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

// The data directory of the primary disappears (a lost or wrongly mounted volume): the keeper must not initialize a
// new, empty primary in its place. The standby takes over with the data, and the old keeper comes back as a standby.
var _ = Describe("a primary whose data directory disappears", Ordered, func() {
	// destroys the primary, so it gets its own cluster
	tc := helpers.WithOwnCluster(2)
	const table = "wiped"
	const rows = 100
	var oldPrimary string
	var oldIndex int

	count := func(ctx context.Context, conn helpers.PgConnParams) func() (int, error) {
		return func() (int, error) {
			return helpers.PgQueryInt(ctx, conn, fmt.Sprintf("select count(*) from %s", table))
		}
	}

	It("has data on the primary and the standby", func(ctx context.Context) {
		oldPrimary = tc.WaitForConverged(2)
		oldIndex = tc.KeeperIndex(oldPrimary)
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

	It("fails over to the standby with the data, without a new empty primary", func(ctx context.Context) {
		// stop postgres hard and remove its data directory, while the keeper keeps running
		tc.KillPostgres(ctx, oldIndex)
		tc.RemoveDataDir(ctx, oldIndex)

		Eventually(func(g Gomega) {
			status, _, err := tc.APIClient.GetStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(status.Status).To(HaveKeyWithValue("primaryKeeper", And(Not(BeEmpty()), Not(Equal(oldPrimary)))))
		}).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())

		// the proxy serves the new primary, with every row
		Eventually(count(ctx, tc.ProxyConn(ctx))).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).
			WithPolling(helpers.PollInterval).Should(Equal(rows))
		Ω(helpers.PgExec(ctx, tc.ProxyConn(ctx), fmt.Sprintf("insert into %s values (%d)", table, rows+1))).
			To(Succeed())
	})

	It("brings the old keeper back as a standby of the new primary, with the data", func(ctx context.Context) {
		newPrimary := tc.WaitForConverged(2)
		Ω(newPrimary).NotTo(Equal(oldPrimary))
		Eventually(count(ctx, tc.KeeperConn(ctx, oldIndex))).WithContext(ctx).WithTimeout(time.Minute).
			WithPolling(time.Second).Should(Equal(rows + 1))
		inRecovery, err := helpers.PgQueryInt(ctx, tc.KeeperConn(ctx, oldIndex), "select pg_is_in_recovery()::int")
		Ω(err).NotTo(HaveOccurred())
		Ω(inRecovery).To(Equal(1))
	})
})
