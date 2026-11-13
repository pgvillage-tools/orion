package system_test

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/util"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

var _ = Describe("/v1/cluster/spec", Ordered, func() {
	// these tests change the spec, so they get their own cluster
	tc := helpers.WithOwnCluster(1)

	// pgSetting returns the value of a setting of the postgres of the (only) keeper
	pgSetting := func(ctx context.Context, name string) (string, error) {
		conn, err := pgx.Connect(ctx, tc.KeeperConn(ctx, 0).ConnString())
		if err != nil {
			return "", err
		}
		defer func() { _ = conn.Close(ctx) }()
		var value string
		err = conn.QueryRow(ctx, "select current_setting($1)", name).Scan(&value)
		return value, err
	}

	It("GET returns the spec the cluster was created with", func() {
		tc.WaitForPrimary()
		spec, httpCode, err := tc.APIClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(spec.InitMode).To(HaveValue(Equal(apiv1.New)))
		Ω(spec.DefaultSUReplAccessMode).To(HaveValue(Equal(apiv1.SUReplAccessAll)))
	})
	It("PATCH changes a postgres parameter, and the keeper applies it", func(ctx context.Context) {
		httpCode, err := tc.APIClient.PatchClusterSpec([]byte(`{"pgParameters":{"log_min_duration_statement":"1500"}}`))
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))

		spec, _, err := tc.APIClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		Ω(spec.PGParameters).To(HaveKeyWithValue("log_min_duration_statement", "1500"))
		Ω(spec.InitMode).To(HaveValue(Equal(apiv1.New)), "PATCH keeps the rest of the spec")

		Eventually(func() (string, error) {
			return pgSetting(ctx, "log_min_duration_statement")
		}).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Equal("1500ms"))
	})
	It("PATCH is a JSON merge patch (RFC 7386): null removes a parameter", func() {
		httpCode, err := tc.APIClient.PatchClusterSpec([]byte(`{"pgParameters":{"log_min_duration_statement":null}}`))
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))

		spec, _, err := tc.APIClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		Ω(spec.PGParameters).NotTo(HaveKey("log_min_duration_statement"))
		Ω(spec.InitMode).To(HaveValue(Equal(apiv1.New)), "PATCH keeps the rest of the spec")
	})
	It("PATCH refuses an invalid spec, invalid JSON and a change of the init mode", func() {
		for patch, expected := range map[string]int{
			`{"sleepInterval":"-1s"}`: http.StatusBadRequest,
			`{"sleepInterval":`:       http.StatusBadRequest,
			`{"initMode":"existing","existingConfig":{"keeperUID":"keeper0"}}`: http.StatusConflict,
		} {
			httpCode, err := tc.APIClient.PatchClusterSpec([]byte(patch))
			Ω(err).To(HaveOccurred(), patch)
			Ω(httpCode).To(Equal(expected), patch)
		}
	})
	It("PUT refuses an invalid spec and null", func() {
		spec, _, err := tc.APIClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		spec.SleepInterval = &apiv1.Duration{Duration: -time.Second}
		httpCode, err := tc.APIClient.PutClusterSpec(spec)
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusBadRequest))

		httpCode, err = tc.APIClient.PutClusterSpec(nil)
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusBadRequest))
	})
	It("PUT replaces the spec", func(ctx context.Context) {
		spec, _, err := tc.APIClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		delete(spec.PGParameters, "log_min_duration_statement")
		spec.PGParameters["work_mem"] = "8MB"

		httpCode, err := tc.APIClient.PutClusterSpec(spec)
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))

		after, _, err := tc.APIClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		Ω(after.PGParameters).To(HaveKeyWithValue("work_mem", "8MB"))
		Ω(after.PGParameters).NotTo(HaveKey("log_min_duration_statement"))

		Eventually(func() (string, error) {
			return pgSetting(ctx, "work_mem")
		}).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Equal("8MB"))
	})
	It("refuses a write with an old ETag (412), and accepts the current one", func() {
		_, etag, _, err := tc.APIClient.GetClusterSpecWithETag()
		Ω(err).NotTo(HaveOccurred())
		Ω(etag).To(MatchRegexp(`^"[0-9a-f]+"$`))

		// a write in between makes etag old
		_, newETag, err := tc.APIClient.PatchClusterSpecIfMatch([]byte(`{"pgParameters":{"work_mem":"16MB"}}`), etag)
		Ω(err).NotTo(HaveOccurred())
		Ω(newETag).NotTo(Equal(etag))

		httpCode, _, err := tc.APIClient.PatchClusterSpecIfMatch([]byte(`{"pgParameters":{"work_mem":"32MB"}}`), etag)
		Ω(err).To(MatchError(ContainSubstring("412 Precondition Failed")))
		Ω(httpCode).To(Equal(http.StatusPreconditionFailed))
		spec, _, err := tc.APIClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		Ω(spec.PGParameters).To(HaveKeyWithValue("work_mem", "16MB"), "the refused patch changed nothing")

	})
	It("refuses to replace the cluster data with an old ETag (412)", func() {
		// the ETag of all cluster data is its store revision, which changes with every write (also the sentinel's)
		cd, old, _, err := tc.APIClient.GetClusterWithETag()
		Ω(err).NotTo(HaveOccurred())
		_, _, err = tc.APIClient.PatchClusterSpecIfMatch([]byte(`{"pgParameters":{"work_mem":"24MB"}}`), "")
		Ω(err).NotTo(HaveOccurred())
		httpCode, _, err := tc.APIClient.PutClusterIfMatch(cd, old)
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusPreconditionFailed))

		// with the current ETag it works (the sentinel may write in between: then read it again)
		Eventually(func(g Gomega) {
			cd, current, _, err := tc.APIClient.GetClusterWithETag()
			g.Ω(err).NotTo(HaveOccurred())
			httpCode, _, err := tc.APIClient.PutClusterIfMatch(cd, current)
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(httpCode).To(Equal(http.StatusOK))
		}).WithTimeout(30 * time.Second).WithPolling(time.Second).Should(Succeed())
	})
})

var _ = Describe("/v1/cluster/spec without a cluster", Ordered, func() {
	// only etcd and the api: no cluster data yet
	tc := helpers.WithOwnAPI()

	It("GET, PATCH and PUT return 404", func() {
		_, httpCode, err := tc.APIClient.GetClusterSpec()
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusNotFound))
		var p endpoints.Problem
		Ω(errors.As(err, &p)).To(BeTrue())
		Ω(p.Detail).To(ContainSubstring("no cluster spec"))

		httpCode, err = tc.APIClient.PatchClusterSpec([]byte(`{"pgParameters":{"work_mem":"8MB"}}`))
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusNotFound))

		httpCode, err = tc.APIClient.PutClusterSpec(&apiv1.Spec{InitMode: util.ToPtr(apiv1.New)})
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusNotFound))
	})
	It("returns 500 while the store doesn't respond", func(ctx context.Context) {
		helpers.PauseContainer(ctx, tc.Etcd)
		_, httpCode, err := tc.APIClient.GetClusterSpec()
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusInternalServerError))
	})
})
