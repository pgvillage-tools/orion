package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/auth"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/prometheus/client_golang/prometheus"
)

var _ = Describe("metrics", func() {
	It("counts the requests, shows the certificates and the metrics of the gatherer", func(ctx SpecContext) {
		dir := GinkgoT().TempDir()
		opts := pki.InitOptions{CLIDir: filepath.Join(dir, "cli"), APIDir: filepath.Join(dir, "api")}
		_, err := pki.Init(opts)
		Expect(err).NotTo(HaveOccurred())
		files := pki.APIFiles(opts.APIDir)
		serverConfig, err := files.ServerConfig()
		Expect(err).NotTo(HaveOccurred())
		certs, err := files.Certificates()
		Expect(err).NotTo(HaveOccurred())

		metrics := newAPIMetrics(prometheus.NewRegistry())
		metrics.setCertificates(certs)
		var isReady atomic.Bool
		isReady.Store(true)
		h := NewHandlers(nil, nil, &isReady)
		routes := h.Routes()
		for i := range routes {
			routes[i].Handler = metrics.instrument(routes[i])
		}
		server := httptest.NewUnstartedServer(auth.Middleware(newMux(ctx, routes), auth.ClientCertAuthenticator{}))
		server.TLS = serverConfig
		server.StartTLS()
		DeferCleanup(server.Close)
		gatherer := prometheus.NewRegistry()
		gatherer.MustRegister(prometheus.NewGauge(prometheus.GaugeOpts{Name: "from_the_gatherer"}))
		probe := httptest.NewServer(newMux(ctx, h.ProbeRoutes(metrics.handler(gatherer))))
		DeferCleanup(probe.Close)

		clientConfig, err := pki.CLIFiles(opts.CLIDir).ClientConfig()
		Expect(err).NotTo(HaveOccurred())
		client := http.Client{Transport: &http.Transport{TLSClientConfig: clientConfig}, Timeout: 5 * time.Second}
		for range 2 {
			resp, getErr := client.Get(server.URL + "/" + endpoints.APIVersion + "/" + string(endpoints.WhoAmIEndPoint))
			Expect(getErr).NotTo(HaveOccurred())
			_ = resp.Body.Close()
		}

		resp, err := http.Get(probe.URL + "/" + string(endpoints.MetricsEndPoint))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = resp.Body.Close() })
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		text := string(body)
		for _, expected := range []string{
			`orion_api_requests_total{code="200",method="get",route="/v1/whoami"} 2`,
			`orion_api_request_duration_seconds_count{method="get",route="/v1/whoami"} 2`,
			`orion_api_certificate_expiry_timestamp_seconds{ca="false",common_name="` + pki.APIServerName + `"`,
			`orion_api_certificate_expiry_timestamp_seconds{ca="true",common_name="` + pki.ServerIntermediate + `"`,
			"from_the_gatherer 0",
		} {
			Expect(text).To(ContainSubstring(expected))
		}
		Expect(strings.Count(text, "\norion_api_certificate_expiry_timestamp_seconds{")).To(Equal(len(certs)),
			"expected a metric per certificate")
	})
})
