package system_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

// replicationTimeout is how long a row written on the primary may take to reach every standby
const replicationTimeout = 30 * time.Second

var _ = Describe("Replication", func() {
	It("replicates what is written through the proxy to every keeper", func(ctx context.Context) {
		sharedCluster.WaitForPrimary()
		// a table of its own: the shared cluster is used by other specs too
		table := fmt.Sprintf("replication_%d", time.Now().UnixNano())
		proxy := sharedCluster.ProxyConn(ctx)
		Eventually(func() error {
			return helpers.PgExec(ctx, proxy, fmt.Sprintf("create table %s (id int primary key)", table))
		}).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
		Ω(helpers.PgExec(ctx, proxy, fmt.Sprintf("insert into %s select generate_series(1, 100)", table))).
			To(Succeed())

		// the primary and the standbys (hot standbys: they answer read queries) all have the rows
		for i := range sharedCluster.Keepers {
			Eventually(func() (int, error) {
				return helpers.PgQueryInt(ctx, sharedCluster.KeeperConn(ctx, i),
					fmt.Sprintf("select count(*) from %s", table))
			}).WithContext(ctx).WithTimeout(replicationTimeout).WithPolling(time.Second).
				Should(Equal(100), "keeper %d", i)
		}

		// a standby refuses writes: only the primary takes them
		var readOnly int
		for i := range sharedCluster.Keepers {
			inRecovery, err := helpers.PgQueryInt(ctx, sharedCluster.KeeperConn(ctx, i),
				"select pg_is_in_recovery()::int")
			Ω(err).NotTo(HaveOccurred())
			if inRecovery == 1 {
				readOnly++
				Ω(helpers.PgExec(ctx, sharedCluster.KeeperConn(ctx, i),
					fmt.Sprintf("insert into %s values (1000)", table))).
					To(MatchError(ContainSubstring("read-only transaction")))
			}
		}
		Ω(readOnly).To(Equal(len(sharedCluster.Keepers)-1), "one primary, the other keepers are standbys")
	})
})
