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
	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
)

// pitrArchive is where the keepers mount the volume with the WAL archive (wal) and a base backup (base)
const pitrArchive = "/archive"

// pitrSpec is the spec of a cluster that is restored from the base backup and the WAL archive in pitrArchive, up to
// target (completely without one)
func pitrSpec(target *apiv1.RecoveryTargetSettings) *apiv1.Spec {
	spec := helpers.DefaultSpec()
	spec.InitMode = util.ToPtr(apiv1.PITR)
	spec.PITRConfig = &apiv1.PITRConfig{
		// %d is the data directory of the keeper; postgres wants it 0700, not the mode of the backup directory
		DataRestoreCommand:      "cp -a " + pitrArchive + "/base/. %d && chmod 700 %d",
		ArchiveRecoverySettings: &apiv1.ArchiveRecoverySettings{RestoreCommand: "cp " + pitrArchive + "/wal/%f %p"},
		RecoveryTargetSettings:  target,
	}
	// not the archive settings of the backed up cluster: a restored cluster doesn't archive to the same archive
	spec.MergePgParameters = util.ToPtr(false)
	return spec
}

// Point in time recovery (initMode pitr): a cluster archives its WAL and is backed up to a volume, and new clusters
// are restored from it, completely and up to a restore point.
var _ = Describe("point in time recovery", Ordered, func() {
	volume := fmt.Sprintf("orion-pitr-%d", time.Now().UnixNano())
	mounts := testcontainers.ContainerMounts{testcontainers.VolumeMount(volume, pitrArchive)}

	source := helpers.DefaultSpec()
	source.PGParameters = apiv1.PGParameters{
		"archive_mode":    "on",
		"archive_command": fmt.Sprintf("test ! -f %[1]s/wal/%%f && cp %%p %[1]s/wal/%%f", pitrArchive),
	}
	// withArchive returns a setup for helpers.WithOwnClusterSetup: a cluster with spec and the archive in its keepers
	withArchive := func(spec *apiv1.Spec) func(context.Context, *helpers.Cluster) *apiv1.Spec {
		return func(_ context.Context, c *helpers.Cluster) *apiv1.Spec {
			c.KeeperMounts = mounts
			return spec
		}
	}
	tc := helpers.WithOwnClusterSetup(1, withArchive(source))
	// after the AfterAll of the clusters, which is registered first
	AfterAll(func(ctx context.Context) { helpers.RemoveVolume(ctx, volume) })

	// count returns the number of rows in table pitr of cluster c
	count := func(ctx context.Context, c *helpers.Cluster) (int, error) {
		return helpers.PgQueryInt(ctx, c.KeeperConn(ctx, 0), "SELECT count(*) FROM pitr")
	}

	It("archives its WAL and is backed up", func(ctx context.Context) {
		tc.WaitForConverged(1)
		// a new volume belongs to root; the keepers run as postgres
		for _, cmd := range [][]string{
			{"mkdir", "-p", pitrArchive + "/wal", pitrArchive + "/base"},
			{"chown", "-R", "postgres:", pitrArchive},
		} {
			code, out := tc.KeeperExec(ctx, 0, cmd, tcexec.WithUser("root"))
			Ω(code).To(Equal(0), out)
		}

		conn := tc.KeeperConn(ctx, 0)
		Ω(helpers.PgExec(ctx, conn, "CREATE TABLE pitr (id int)")).To(Succeed())
		Ω(helpers.PgExec(ctx, conn, "INSERT INTO pitr VALUES (1)")).To(Succeed())

		// without WAL (-X none): a restore needs the archive. pg_basebackup waits until the WAL of the backup is
		// archived.
		code, out := tc.KeeperExec(ctx, 0,
			[]string{"pg_basebackup", "-h", "keeper_0", "-U", "repluser", "-D", pitrArchive + "/base", "-X", "none",
				"-c", "fast"},
			tcexec.WithEnv([]string{"PGPASSWORD=test123"}))
		Ω(code).To(Equal(0), out)

		// after the backup: row 2 before the restore point, row 3 after it
		Ω(helpers.PgExec(ctx, conn, "INSERT INTO pitr VALUES (2)")).To(Succeed())
		Ω(helpers.PgExec(ctx, conn, "SELECT pg_create_restore_point('before_3')")).To(Succeed())
		Ω(helpers.PgExec(ctx, conn, "INSERT INTO pitr VALUES (3)")).To(Succeed())

		// close the WAL segment with the rows, and wait until it is archived
		segment, err := helpers.PgQueryString(ctx, conn, "SELECT pg_walfile_name(pg_current_wal_lsn())")
		Ω(err).NotTo(HaveOccurred())
		Ω(helpers.PgExec(ctx, conn, "SELECT pg_switch_wal()")).To(Succeed())
		Eventually(func() (string, error) {
			return helpers.PgQueryString(ctx, conn, "SELECT coalesce(last_archived_wal, '') FROM pg_stat_archiver")
		}).WithContext(ctx).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).
			// WAL file names are fixed-width hex: compared as strings
			Should(Satisfy(func(archived string) bool { return archived >= segment }))
	})

	Context("restored completely", Ordered, func() {
		restored := helpers.WithOwnClusterSetup(1, withArchive(pitrSpec(nil)))

		It("has every row", func(ctx context.Context) {
			restored.WaitForConverged(1)
			Eventually(func() (int, error) { return count(ctx, restored) }).
				WithContext(ctx).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).Should(Equal(3))
		})
		It("is a primary of its own", func(ctx context.Context) {
			Ω(helpers.PgExec(ctx, restored.KeeperConn(ctx, 0), "INSERT INTO pitr VALUES (4)")).To(Succeed())
			Ω(count(ctx, tc)).To(Equal(3), "the backed up cluster has its own rows")
		})
	})

	Context("restored up to a restore point", Ordered, func() {
		restored := helpers.WithOwnClusterSetup(1,
			withArchive(pitrSpec(&apiv1.RecoveryTargetSettings{RecoveryTargetName: "before_3"})))

		It("has the rows before the restore point", func(ctx context.Context) {
			restored.WaitForConverged(1)
			Eventually(func() (int, error) { return count(ctx, restored) }).
				WithContext(ctx).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).Should(Equal(2))
		})
	})
})
