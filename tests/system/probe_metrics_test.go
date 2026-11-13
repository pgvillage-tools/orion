package system_test

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

var _ = Describe("GET /metrics on the probe port", func() {
	// scrape returns the metrics of the shared cluster, in the Prometheus text format
	scrape := func(ctx context.Context) string {
		code, body := sharedCluster.ProbeGet(ctx, endpoints.MetricsEndPoint)
		Ω(code).To(Equal(http.StatusOK))
		return body
	}
	// value returns the value of the series that matches the regular expression series (name and labels), and
	// fails when there is none
	value := func(metrics, series string) float64 {
		m := regexp.MustCompile(`(?m)^` + series + ` (\S+)$`).FindStringSubmatch(metrics)
		Ω(m).NotTo(BeNil(), "no series %s in the metrics", series)
		v, err := strconv.ParseFloat(m[1], 64)
		Ω(err).NotTo(HaveOccurred())
		return v
	}
	whoamiRequests := `orion_api_requests_total\{code="200",method="get",route="/v1/whoami"\}`
	whoamiDuration := `orion_api_request_duration_seconds_count\{method="get",route="/v1/whoami"\}`

	It("counts and times the requests on the api port, by route", func(ctx context.Context) {
		// other tests call whoami too, so compare before and after
		before := scrape(ctx)
		requestsBefore, durationBefore := 0.0, 0.0
		if regexp.MustCompile(`(?m)^` + whoamiRequests + ` `).MatchString(before) {
			requestsBefore = value(before, whoamiRequests)
			durationBefore = value(before, whoamiDuration)
		}
		for range 2 {
			_, httpCode, err := sharedCluster.APIClient.WhoAmI()
			Ω(err).NotTo(HaveOccurred())
			Ω(httpCode).To(Equal(http.StatusOK))
		}
		after := scrape(ctx)
		Ω(value(after, whoamiRequests)).To(Equal(requestsBefore + 2))
		Ω(value(after, whoamiDuration)).To(Equal(durationBefore + 2))
	})
	It("shows when the certificates of the api expire", func(ctx context.Context) {
		metrics := scrape(ctx)
		expiry := func(ca bool, commonName string) time.Time {
			series := `orion_api_certificate_expiry_timestamp_seconds\{ca="` + strconv.FormatBool(ca) +
				`",common_name="` + regexp.QuoteMeta(commonName) + `",serial="[0-9a-f]+"\}`
			return time.Unix(int64(value(metrics, series)), 0)
		}
		// the server certificate of the api is valid for a year by default (pki init), its intermediates longer
		Ω(expiry(false, pki.APIServerName)).To(BeTemporally("~", time.Now().Add(365*24*time.Hour), 48*time.Hour))
		Ω(expiry(true, pki.ServerIntermediate)).To(BeTemporally(">", time.Now().Add(365*24*time.Hour)))
		Ω(expiry(true, pki.APIClientIntermediate)).To(BeTemporally(">", time.Now().Add(365*24*time.Hour)))
	})
	It("also serves the go, process and cluster metrics", func(ctx context.Context) {
		metrics := scrape(ctx)
		Ω(metrics).To(MatchRegexp(`(?m)^go_goroutines \d+`))
		Ω(metrics).To(MatchRegexp(`(?m)^process_start_time_seconds \S+`))
		Ω(metrics).To(MatchRegexp(`(?m)^orion_cluster_identifier\{`))
	})
	It("only allows GET", func(ctx context.Context) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			sharedCluster.ProbeURL+"/"+string(endpoints.MetricsEndPoint), nil)
		Ω(err).NotTo(HaveOccurred())
		resp, err := http.DefaultClient.Do(req)
		Ω(err).NotTo(HaveOccurred())
		_ = resp.Body.Close()
		Ω(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed))
	})
})
