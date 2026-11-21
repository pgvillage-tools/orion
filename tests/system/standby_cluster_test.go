package system_test

import (
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/util"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

// A standby cluster (role standby): a second cluster replicates from the primary of another cluster, from a base
// backup (initMode pitr), and becomes a primary cluster of its own when its role changes to master.
var _ = Describe("a standby cluster", Ordered, func() {
	source := helpers.WithOwnCluster(1)
	// nested, so the standby cluster, on the network of the source cluster too, is removed before the source cluster
	// and its network
	Context("replicating from it", Ordered, func() {
		standby := helpers.WithOwnClusterSetup(1, func(ctx context.Context, c *helpers.Cluster) *apiv1.Spec {
			// the keepers of the standby cluster reach the primary of the source cluster on its network, by address:
			// keeper_0 is the alias of their own keeper too
			c.KeeperNetworks = []string{source.Network.Name}
			primary := source.KeeperIP(ctx, 0)
			spec := helpers.DefaultSpec()
			spec.InitMode = util.ToPtr(apiv1.PITR)
			spec.Role = util.ToPtr(apiv1.Replica)
			spec.PITRConfig = &apiv1.PITRConfig{
				// %d is the data directory of the keeper
				DataRestoreCommand: fmt.Sprintf("PGPASSWORD=test123 pg_basebackup -h %s -U repluser -D %%d -X stream",
					primary),
			}
			spec.StandbyConfig = &apiv1.StandbyConfig{StandbySettings: &apiv1.StandbySettings{
				PrimaryConninfo: fmt.Sprintf("host=%s port=5432 user=repluser password=test123 sslmode=disable", primary),
			}}
			return spec
		})

		// count returns the number of rows in table standby of cluster c
		count := func(ctx context.Context, c *helpers.Cluster) (int, error) {
			return helpers.PgQueryInt(ctx, c.KeeperConn(ctx, 0), "SELECT count(*) FROM standby")
		}

		It("replicates from the primary of the source cluster", func(ctx context.Context) {
			source.WaitForConverged(1)
			standby.WaitForConverged(1)
			conn := source.KeeperConn(ctx, 0)
			Ω(helpers.PgExec(ctx, conn, "CREATE TABLE standby (id int)")).To(Succeed())
			Ω(helpers.PgExec(ctx, conn, "INSERT INTO standby VALUES (1), (2)")).To(Succeed())
			Eventually(func() (int, error) { return count(ctx, standby) }).
				WithContext(ctx).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).Should(Equal(2))
		})

		It("is read only", func(ctx context.Context) {
			err := helpers.PgExec(ctx, standby.KeeperConn(ctx, 0), "INSERT INTO standby VALUES (3)")
			Ω(err).To(MatchError(ContainSubstring("read-only transaction")))
		})

		It("becomes a primary cluster when its role changes to master", func(ctx context.Context) {
			httpCode, err := standby.APIClient.PatchClusterSpec([]byte(`{"role":"master"}`))
			Ω(err).NotTo(HaveOccurred())
			Ω(httpCode).To(Equal(http.StatusOK))

			Eventually(func() error {
				return helpers.PgExec(ctx, standby.KeeperConn(ctx, 0), "INSERT INTO standby VALUES (3)")
			}).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
			Ω(count(ctx, standby)).To(Equal(3))
		})

		It("no longer follows the source cluster", func(ctx context.Context) {
			Ω(helpers.PgExec(ctx, source.KeeperConn(ctx, 0), "INSERT INTO standby VALUES (10), (11)")).To(Succeed())
			Consistently(func() (int, error) { return count(ctx, standby) }).
				WithContext(ctx).WithTimeout(10 * time.Second).WithPolling(helpers.PollInterval).Should(Equal(3))
			Ω(count(ctx, source)).To(Equal(4), "the source cluster has its own rows")
		})
	})
})
