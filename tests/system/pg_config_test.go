package system_test

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/util"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

// restartTimeout is the time for a keeper to apply a changed spec and restart postgres (a few keeper checks)
const restartTimeout = 2 * time.Minute

// How the keeper applies the postgres parameters of the cluster spec: restarts, wal_level, a bad parameter, ALTER
// SYSTEM, and which parameters of initdb end up in the spec (mergePgParameters)
var _ = Describe("postgres parameters", Ordered, func() {
	// changes and breaks the configuration, so it gets its own cluster; one keeper is enough
	tc := helpers.WithOwnCluster(1)

	// setting returns the value of a parameter on the keeper
	setting := func(ctx context.Context, name string) (string, error) {
		conn, err := pgx.Connect(ctx, tc.KeeperConn(ctx, 0).ConnString())
		if err != nil {
			return "", err
		}
		defer func() { _ = conn.Close(ctx) }()
		var value string
		err = conn.QueryRow(ctx, "select current_setting($1)", name).Scan(&value)
		return value, err
	}
	// startTime returns when postgres started, as unix time
	startTime := func(ctx context.Context) (int, error) {
		return helpers.PgQueryInt(ctx, tc.KeeperConn(ctx, 0), "select extract(epoch from pg_postmaster_start_time())::int")
	}
	patch := func(body string) {
		httpCode, err := tc.APIClient.PatchClusterSpec([]byte(body))
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
	}
	var started int

	It("merges the parameters of initdb into the spec (mergePgParameters, the default)", func(ctx context.Context) {
		tc.WaitForConverged(1)
		Eventually(func(g Gomega) {
			spec, _, err := tc.APIClient.GetClusterSpec()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(spec.PGParameters).To(HaveKey("max_connections"))
		}).WithContext(ctx).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).Should(Succeed())
	})

	It("doesn't restart postgres for a parameter that needs it, without automaticPgRestart", func(ctx context.Context) {
		var err error
		started, err = startTime(ctx)
		Ω(err).NotTo(HaveOccurred())
		patch(`{"pgParameters":{"max_connections":"150"}}`)

		Eventually(func() (int, error) {
			return helpers.PgQueryInt(ctx, tc.KeeperConn(ctx, 0),
				"select pending_restart::int from pg_settings where name = 'max_connections'")
		}).WithContext(ctx).WithTimeout(restartTimeout).WithPolling(helpers.PollInterval).Should(Equal(1))
		Ω(setting(ctx, "max_connections")).To(Equal("100"))
		Ω(startTime(ctx)).To(Equal(started))
	})

	It("restarts postgres with automaticPgRestart", func(ctx context.Context) {
		patch(`{"automaticPgRestart":true}`)

		Eventually(func() (string, error) { return setting(ctx, "max_connections") }).
			WithContext(ctx).WithTimeout(restartTimeout).WithPolling(helpers.PollInterval).Should(Equal("150"))
		Ω(startTime(ctx)).To(BeNumerically(">", started))
	})

	It("only applies wal_level logical, and keeps replica for anything else", func(ctx context.Context) {
		patch(`{"pgParameters":{"wal_level":"minimal"}}`)
		tc.WaitForConverged(1)
		Consistently(func() (string, error) { return setting(ctx, "wal_level") }).
			WithContext(ctx).WithTimeout(15 * time.Second).WithPolling(helpers.PollInterval).Should(Equal("replica"))

		patch(`{"pgParameters":{"wal_level":"logical"}}`)
		Eventually(func() (string, error) { return setting(ctx, "wal_level") }).
			WithContext(ctx).WithTimeout(restartTimeout).WithPolling(helpers.PollInterval).Should(Equal("logical"))
	})

	It("keeps postgres running on its old configuration while the spec has a parameter postgres doesn't know",
		func(ctx context.Context) {
			// postgres refuses the whole configuration file on reload, so the change of max_connections isn't seen
			// either, and the keeper doesn't restart postgres into a configuration it can't start with
			patch(`{"pgParameters":{"orion_e2e_unknown":"1","max_connections":"160"}}`)
			Consistently(func() (string, error) { return setting(ctx, "max_connections") }).
				WithContext(ctx).WithTimeout(30 * time.Second).WithPolling(helpers.PollInterval).Should(Equal("150"))

			// without it, the change is applied
			patch(`{"pgParameters":{"orion_e2e_unknown":null}}`)
			Eventually(func() (string, error) { return setting(ctx, "max_connections") }).
				WithContext(ctx).WithTimeout(restartTimeout).WithPolling(helpers.PollInterval).Should(Equal("160"))
		})

	It("ignores ALTER SYSTEM: postgresql.auto.conf is /dev/null", func(ctx context.Context) {
		before, err := setting(ctx, "work_mem")
		Ω(err).NotTo(HaveOccurred())
		// whether ALTER SYSTEM itself fails depends on the postgres version; it must not change the setting
		_ = helpers.PgExec(ctx, tc.KeeperConn(ctx, 0), "alter system set work_mem = '42MB'")
		Ω(helpers.PgExec(ctx, tc.KeeperConn(ctx, 0), "select pg_reload_conf()")).To(Succeed())
		Consistently(func() (string, error) { return setting(ctx, "work_mem") }).
			WithContext(ctx).WithTimeout(10 * time.Second).WithPolling(helpers.PollInterval).Should(Equal(before))
	})
})

// With mergePgParameters false the spec only holds what the user set
var _ = Describe("postgres parameters without mergePgParameters", Ordered, func() {
	spec := helpers.DefaultSpec()
	spec.MergePgParameters = util.ToPtr(false)
	spec.PGParameters = apiv1.PGParameters{"work_mem": "9MB"}
	tc := helpers.WithOwnClusterSpec(1, spec)

	It("keeps the parameters of initdb out of the spec", func() {
		tc.WaitForConverged(1)
		got, _, err := tc.APIClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		Ω(got.PGParameters).To(Equal(apiv1.PGParameters{"work_mem": "9MB"}))
	})
})
