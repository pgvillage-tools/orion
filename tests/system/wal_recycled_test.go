package system_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/util"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

// walRecycledSpec keeps little WAL on the primary: no wal_keep_size, and at most 32MB for a replication slot, so a
// standby that is away while the primary writes more loses its slot and the WAL it needs. A write to the paused standby
// fails after 10s (tcp_user_timeout): its walsender, which the checkpoint must end to invalidate its slot, otherwise
// stays blocked on the write while the standby is paused (13 minutes in a run without it, until the standby was
// unpaused; it may never end), and the checkpoint with it. The resync (pg_basebackup) asks for a fast checkpoint
// (resyncFastCheckpoint), instead of waiting for a spread one (4.5 minutes with the default checkpoint_timeout).
func walRecycledSpec() *apiv1.Spec {
	spec := helpers.DefaultSpec()
	spec.PGParameters["wal_keep_size"] = "0"
	spec.PGParameters["max_slot_wal_keep_size"] = "32"
	spec.PGParameters["tcp_user_timeout"] = "10000"
	spec.ResyncFastCheckpoint = util.ToPtr(true)
	return spec
}

// A standby that was away while the primary removed the WAL it needs can't catch up by streaming: it is healthy but
// its WAL position stops. The sentinel removes its db (dbCanSync) and the keeper gets a new one, from a full resync
// of the primary.
var _ = Describe("a standby whose WAL was recycled", Ordered, func() {
	// pauses a keeper and removes WAL, so it gets its own cluster
	tc := helpers.WithOwnClusterSpec(2, walRecycledSpec())
	const table = "recycled"
	var primary, standby string
	var standbyIndex int
	var standbyDB string

	// dbOf returns the uid of the db of keeper uid in the cluster data
	dbOf := func(uid string) (string, error) {
		cd, _, err := tc.APIClient.GetCluster()
		if err != nil {
			return "", err
		}
		for _, db := range cd.DBs {
			if db.Spec.KeeperUID == uid {
				return db.UID, nil
			}
		}
		return "", fmt.Errorf("keeper %s has no db", uid)
	}
	count := func(ctx context.Context, i int) func() (int, error) {
		return func() (int, error) {
			return helpers.PgQueryInt(ctx, tc.KeeperConn(ctx, i), "select count(*) from "+table)
		}
	}

	It("has a streaming standby", func(ctx context.Context) {
		primary = tc.WaitForConverged(2)
		standbyIndex = 1 - tc.KeeperIndex(primary)
		status, _, err := tc.APIClient.GetStatus()
		Ω(err).NotTo(HaveOccurred())
		for _, k := range status.Keepers {
			if k.UID != primary {
				standby = k.UID
			}
		}
		standbyDB, err = dbOf(standby)
		Ω(err).NotTo(HaveOccurred())

		conn := tc.KeeperConn(ctx, tc.KeeperIndex(primary))
		Ω(helpers.PgExec(ctx, conn, "create table "+table+" (id int)")).To(Succeed())
		Ω(helpers.PgExec(ctx, conn, "insert into "+table+" values (1)")).To(Succeed())
		Eventually(count(ctx, standbyIndex)).WithContext(ctx).WithTimeout(time.Minute).
			WithPolling(helpers.PollInterval).Should(Equal(1))
	})

	It("loses its replication slot while it is away and the primary writes", func(ctx context.Context) {
		helpers.PauseContainer(ctx, tc.Keepers[standbyIndex])
		defer helpers.UnpauseContainer(ctx, tc.Keepers[standbyIndex])

		conn := tc.KeeperConn(ctx, tc.KeeperIndex(primary))
		Ω(helpers.PgExec(ctx, conn, "create table filler (t text)")).To(Succeed())
		// over 40MB of WAL per round, each in finished segments that a checkpoint can remove
		for range 3 {
			Ω(helpers.PgExec(ctx, conn, "insert into filler select repeat('x', 1000) from generate_series(1, 40000)")).
				To(Succeed())
			Ω(helpers.PgExec(ctx, conn, "select pg_switch_wal()")).To(Succeed())
			Ω(helpers.PgExec(ctx, conn, "checkpoint")).To(Succeed())
		}
		Eventually(func() (int, error) {
			return helpers.PgQueryInt(ctx, conn, "select count(*) from pg_replication_slots where wal_status = 'lost'")
		}).WithContext(ctx).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).Should(Equal(1))
	})

	It("gets a new db from a full resync, and streams again", func(ctx context.Context) {
		// it can't stream from its lost slot: once its WAL position stopped for a while, the sentinel replaces its db
		Eventually(func() (string, error) { return dbOf(standby) }).WithContext(ctx).
			WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).
			Should(And(Not(BeEmpty()), Not(Equal(standbyDB))))
		Ω(tc.WaitForConverged(2)).To(Equal(primary))

		rows, err := count(ctx, tc.KeeperIndex(primary))()
		Ω(err).NotTo(HaveOccurred())
		Eventually(count(ctx, standbyIndex)).WithContext(ctx).WithTimeout(time.Minute).
			WithPolling(helpers.PollInterval).Should(Equal(rows))
		Ω(helpers.PgExec(ctx, tc.KeeperConn(ctx, tc.KeeperIndex(primary)), "insert into "+table+" values (3)")).
			To(Succeed())
		Eventually(count(ctx, standbyIndex)).WithContext(ctx).WithTimeout(time.Minute).
			WithPolling(helpers.PollInterval).Should(Equal(rows + 1))
	})
})
