package system_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

// The primary crashes (its container is killed): the sentinel fails over to the standby, which has all the data, and
// the proxy follows. When the old primary comes back, it rejoins as a standby of the new one.
var _ = Describe("a crashing primary", Ordered, func() {
	// kills the primary, so it gets its own cluster
	tc := helpers.WithOwnCluster(2)
	const table = "crash"
	const rows = 100
	var oldPrimary string
	// oldIndex is the index of the old primary in tc.Keepers: KeeperIndex needs its listen address, which the
	// sentinel removes once it fails the keeper over
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

	It("fails over to the standby when the primary is killed, with all the data", func(ctx context.Context) {
		helpers.SignalContainer(ctx, tc.Keepers[oldIndex], "SIGKILL")

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
	})

	It("rejoins as a standby of the new primary when the old primary comes back", func(ctx context.Context) {
		i := oldIndex
		Ω(tc.Keepers[i].Start(ctx)).To(Succeed())

		newPrimary := tc.WaitForConverged(2)
		Ω(newPrimary).NotTo(Equal(oldPrimary), "the old primary doesn't take over again")
		// it follows the new primary: it has the row written after the failover, and refuses writes
		Eventually(count(ctx, tc.KeeperConn(ctx, i))).WithContext(ctx).WithTimeout(30 * time.Second).
			WithPolling(time.Second).Should(Equal(rows + 1))
		inRecovery, err := helpers.PgQueryInt(ctx, tc.KeeperConn(ctx, i), "select pg_is_in_recovery()::int")
		Ω(err).NotTo(HaveOccurred())
		Ω(inRecovery).To(Equal(1))
	})
})
