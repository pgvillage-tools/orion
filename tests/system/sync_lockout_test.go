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

// With synchronous replication, a primary that restarts doesn't let normal users in until its synchronous standby is
// in sync again: what they would write could otherwise be lost on a failover. The superuser (for orion itself) and
// the replication user can always connect.
var _ = Describe("a restarted primary with synchronous replication", Ordered, func() {
	spec := helpers.DefaultSpec()
	spec.SynchronousReplication = util.ToPtr(true)
	// normal users connect with a password; the keeper adds the entries for the superuser and replication itself
	spec.PGHBA = []string{"host all all 0.0.0.0/0 scram-sha-256", "host all all ::0/0 scram-sha-256"}
	tc := helpers.WithOwnClusterSpec(2, spec)
	const (
		table    = "lockout"
		rows     = 10
		user     = "e2e_user"
		password = "e2e_password"
	)
	var primary, standby int

	// asUser returns the connection parameters of the primary for the normal user
	asUser := func(ctx context.Context) helpers.PgConnParams {
		return tc.KeeperConn(ctx, primary).SetParam("user", user).SetParam("password", password)
	}

	It("has a normal user that can read the data", func(ctx context.Context) {
		primaryUID := tc.WaitForConverged(2)
		primary = tc.KeeperIndex(primaryUID)
		standby = 1 - primary
		for _, sql := range []string{
			fmt.Sprintf("create role %s login password '%s'", user, password),
			fmt.Sprintf("create table %s (id int primary key)", table),
			fmt.Sprintf("insert into %s select generate_series(1, %d)", table, rows),
			fmt.Sprintf("grant select on %s to %s", table, user),
		} {
			Ω(helpers.PgExec(ctx, tc.ProxyConn(ctx), sql)).To(Succeed(), sql)
		}
		Ω(helpers.PgQueryInt(ctx, asUser(ctx), "select count(*) from "+table)).To(Equal(rows))
	})

	It("locks the normal user out after a restart until the synchronous standby is in sync again", func(ctx context.Context) {
		helpers.PauseContainer(ctx, tc.Keepers[standby])

		// postgres on the primary stops; its keeper starts it again
		tc.KillPostgres(ctx, primary)

		// the superuser gets in again, the normal user doesn't
		Eventually(func() error { return helpers.PgPing(ctx, tc.KeeperConn(ctx, primary)) }).
			WithContext(ctx).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).Should(Succeed())
		Consistently(func() error { return helpers.PgPing(ctx, asUser(ctx)) }).
			WithContext(ctx).WithTimeout(15 * time.Second).WithPolling(helpers.PollInterval).
			Should(MatchError(ContainSubstring("pg_hba.conf")))

		// once the synchronous standby is back in sync, the normal user gets in, with the data (in this spec:
		// PauseContainer would unpause it after the spec)
		helpers.UnpauseContainer(ctx, tc.Keepers[standby])

		Eventually(func() (int, error) { return helpers.PgQueryInt(ctx, asUser(ctx), "select count(*) from "+table) }).
			WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Equal(rows))
	})
})
