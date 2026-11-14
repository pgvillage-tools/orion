package system_test

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

// A declarative bootstrap, without the api: the sentinel creates the cluster from --initial-cluster-spec when there
// is no cluster data yet (e.g. for ansible or Kubernetes, with the spec in a file)
var _ = Describe("a sentinel with --initial-cluster-spec", Ordered, func() {
	spec := helpers.DefaultSpec()
	spec.PGParameters["work_mem"] = "7MB"
	tc := helpers.WithOwnClusterInitialSpec(2, spec)

	It("creates the cluster, and the keepers converge", func() {
		tc.WaitForConverged(2)
	})

	It("creates it with the spec of the file", func(ctx context.Context) {
		got, httpCode, err := tc.APIClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(got.PGParameters).To(HaveKeyWithValue("work_mem", "7MB"))

		conn, err := pgx.Connect(ctx, tc.ProxyConn(ctx).ConnString())
		Ω(err).NotTo(HaveOccurred())
		defer func() { _ = conn.Close(ctx) }()
		var workMem string
		Ω(conn.QueryRow(ctx, "select current_setting('work_mem')").Scan(&workMem)).To(Succeed())
		Ω(workMem).To(Equal("7MB"))
	})

	It("leaves the api nothing to create: POST /v1/cluster gets 409", func() {
		httpCode, err := tc.APIClient.PostCluster(helpers.DefaultSpec())
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusConflict))
	})
})
