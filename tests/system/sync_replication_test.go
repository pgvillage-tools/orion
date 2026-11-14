package system_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/util"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

// blockedFor is how long a commit must hang while the synchronous standby is frozen
const blockedFor = 5 * time.Second

// With synchronous replication a commit returns only once the synchronous standby has the transaction: when the standby
// freezes, writes wait (and nothing is lost on a failover) instead of going on without it.
var _ = Describe("synchronous replication", Ordered, func() {
	spec := helpers.DefaultSpec()
	spec.SynchronousReplication = util.ToPtr(true)
	tc := helpers.WithOwnClusterSpec(2, spec)
	const table = "sync"
	var primary, standby int

	It("has the standby as synchronous standby of the primary", func(ctx context.Context) {
		primaryUID := tc.WaitForConverged(2)
		primary = tc.KeeperIndex(primaryUID)
		standby = 1 - primary
		Eventually(func() (int, error) {
			return helpers.PgQueryInt(ctx, tc.KeeperConn(ctx, primary),
				"select count(*) from pg_stat_replication where sync_state = 'sync'")
		}).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Equal(1))
	})

	It("waits with a commit while the synchronous standby is frozen", func(ctx context.Context) {
		Ω(helpers.PgExec(ctx, tc.ProxyConn(ctx), fmt.Sprintf("create table %s (id int primary key)", table))).
			To(Succeed())

		helpers.PauseContainer(ctx, tc.Keepers[standby])
		blockedCtx, cancel := context.WithTimeout(ctx, blockedFor)
		defer cancel()
		err := helpers.PgExec(blockedCtx, tc.ProxyConn(ctx), fmt.Sprintf("insert into %s values (1)", table))
		Ω(err).To(HaveOccurred(), "the commit returned while the synchronous standby was frozen")
		Ω(blockedCtx.Err()).To(HaveOccurred(), "the commit failed instead of waiting: %v", err)

		// once the standby is back, it has the row (it was committed locally on the primary while waiting)
		helpers.UnpauseContainer(ctx, tc.Keepers[standby])
		Eventually(func() (int, error) {
			return helpers.PgQueryInt(ctx, tc.KeeperConn(ctx, standby), fmt.Sprintf("select count(*) from %s", table))
		}).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Equal(1))
		// and commits return again
		Eventually(func() error {
			return helpers.PgExec(ctx, tc.ProxyConn(ctx), fmt.Sprintf("insert into %s values (2)", table))
		}).WithContext(ctx).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).Should(Succeed())
	})
})
