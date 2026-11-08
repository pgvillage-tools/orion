package etcdv3_test

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/util"
)

var _ = Describe("/v1/cluster/spec", Ordered, func() {
	// these tests change the spec, so they get their own cluster
	tc := withOwnCluster(1)

	// pgSetting returns the value of a setting of the postgres of the (only) keeper
	pgSetting := func(ctx context.Context, name string) (string, error) {
		conn, err := pgx.Connect(ctx, tc.keeperConn(ctx, 0).connString())
		if err != nil {
			return "", err
		}
		defer func() { _ = conn.Close(ctx) }()
		var value string
		err = conn.QueryRow(ctx, "select current_setting($1)", name).Scan(&value)
		return value, err
	}

	It("GET returns the spec the cluster was created with", func() {
		tc.waitForPrimary()
		spec, httpCode, err := tc.apiClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(spec.InitMode).To(HaveValue(Equal(apiv1.New)))
		Ω(spec.DefaultSUReplAccessMode).To(HaveValue(Equal(apiv1.SUReplAccessAll)))
	})
	It("POST refuses to replace the spec", func() {
		httpCode, err := tc.apiClient.PostClusterSpec(&apiv1.Spec{InitMode: util.ToPtr(apiv1.New)})
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusConflict))
	})
	It("PATCH changes a postgres parameter, and the keeper applies it", func(ctx context.Context) {
		httpCode, err := tc.apiClient.PatchClusterSpec([]byte(`{"pgParameters":{"log_min_duration_statement":"1500"}}`))
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))

		spec, _, err := tc.apiClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		Ω(spec.PGParameters).To(HaveKeyWithValue("log_min_duration_statement", "1500"))
		Ω(spec.InitMode).To(HaveValue(Equal(apiv1.New)), "PATCH keeps the rest of the spec")

		Eventually(func() (string, error) {
			return pgSetting(ctx, "log_min_duration_statement")
		}).WithTimeout(primaryTimeout).WithPolling(pollInterval).Should(Equal("1500ms"))
	})
	It("PATCH refuses an invalid spec", func() {
		httpCode, err := tc.apiClient.PatchClusterSpec([]byte(`{"sleepInterval":"-1s"}`))
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusInternalServerError))
	})
	It("PUT replaces the spec", func(ctx context.Context) {
		spec, _, err := tc.apiClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		delete(spec.PGParameters, "log_min_duration_statement")
		spec.PGParameters["work_mem"] = "8MB"

		httpCode, err := tc.apiClient.PutClusterSpec(spec)
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))

		after, _, err := tc.apiClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		Ω(after.PGParameters).To(HaveKeyWithValue("work_mem", "8MB"))
		Ω(after.PGParameters).NotTo(HaveKey("log_min_duration_statement"))

		Eventually(func() (string, error) {
			return pgSetting(ctx, "work_mem")
		}).WithTimeout(primaryTimeout).WithPolling(pollInterval).Should(Equal("8MB"))
	})
})
