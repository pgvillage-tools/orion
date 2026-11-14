package system_test

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

// additionalMasterReplicationSlots (e.g. for logical replication or a backup tool) exist on the primary only, as
// orion_<name>, and move with the primary on a failover. The keeper owns every orion_* slot: it drops the ones that
// aren't wanted, and leaves other slots alone.
var _ = Describe("additional replication slots", Ordered, func() {
	// fails over, so it gets its own cluster
	tc := helpers.WithOwnCluster(2)
	var primary string

	// slots returns the replication slots on a keeper
	slots := func(ctx context.Context, i int) ([]string, error) {
		conn, err := pgx.Connect(ctx, tc.KeeperConn(ctx, i).ConnString())
		if err != nil {
			return nil, err
		}
		defer func() { _ = conn.Close(ctx) }()
		rows, err := conn.Query(ctx, "select slot_name from pg_replication_slots")
		if err != nil {
			return nil, err
		}
		return pgx.CollectRows(rows, pgx.RowTo[string])
	}
	patch := func(body string) {
		httpCode, err := tc.APIClient.PatchClusterSpec([]byte(body))
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
	}
	slotsOf := func(ctx context.Context, i int) func() ([]string, error) {
		return func() ([]string, error) { return slots(ctx, i) }
	}

	It("creates them on the primary only", func(ctx context.Context) {
		primary = tc.WaitForConverged(2)
		patch(`{"additionalMasterReplicationSlots":["e2e_a","e2e_b"]}`)

		p := tc.KeeperIndex(primary)
		Eventually(slotsOf(ctx, p)).WithContext(ctx).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).
			Should(ContainElements("orion_e2e_a", "orion_e2e_b"))
		Consistently(slotsOf(ctx, 1-p)).WithContext(ctx).WithTimeout(10 * time.Second).
			WithPolling(helpers.PollInterval).ShouldNot(ContainElements("orion_e2e_a"))
	})

	It("drops a manual orion_* slot, and keeps other manual slots", func(ctx context.Context) {
		p := tc.KeeperIndex(primary)
		for _, slot := range []string{"e2e_manual", "orion_e2e_manual"} {
			Ω(helpers.PgExec(ctx, tc.KeeperConn(ctx, p), "select pg_create_physical_replication_slot($1)", slot)).
				To(Succeed())
		}
		Eventually(slotsOf(ctx, p)).WithContext(ctx).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).
			ShouldNot(ContainElement("orion_e2e_manual"))
		Ω(slots(ctx, p)).To(ContainElement("e2e_manual"))
	})

	It("drops a slot that is taken out of the spec", func(ctx context.Context) {
		patch(`{"additionalMasterReplicationSlots":["e2e_a"]}`)
		p := tc.KeeperIndex(primary)
		Eventually(slotsOf(ctx, p)).WithContext(ctx).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).
			ShouldNot(ContainElement("orion_e2e_b"))
		Ω(slots(ctx, p)).To(ContainElement("orion_e2e_a"))
	})

	It("creates them on the new primary after a failover", func(ctx context.Context) {
		// look up the index while the keeper is healthy; the standby takes over only once it converged
		oldIndex := tc.KeeperIndex(primary)
		Ω(tc.WaitForConverged(2)).To(Equal(primary))
		httpCode, err := tc.APIClient.FailKeeper(primary)
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusAccepted))

		Eventually(func(g Gomega) {
			status, _, err := tc.APIClient.GetStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(status.Status).To(HaveKeyWithValue("primaryKeeper", And(Not(BeEmpty()), Not(Equal(primary)))))
		}).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
		Eventually(slotsOf(ctx, 1-oldIndex)).WithContext(ctx).WithTimeout(time.Minute).
			WithPolling(helpers.PollInterval).Should(ContainElement("orion_e2e_a"))
	})
})
