package v1

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pgvillage-tools/orion/internal/auth"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/prometheus/client_golang/prometheus"
)

func TestMetrics(t *testing.T) {
	dir := t.TempDir()
	opts := pki.InitOptions{CLIDir: filepath.Join(dir, "cli"), APIDir: filepath.Join(dir, "api")}
	if _, err := pki.Init(opts); err != nil {
		t.Fatal(err)
	}
	files := pki.APIFiles(opts.APIDir)
	serverConfig, err := files.ServerConfig()
	if err != nil {
		t.Fatal(err)
	}
	certs, err := files.Certificates()
	if err != nil {
		t.Fatal(err)
	}

	metrics := newAPIMetrics(prometheus.NewRegistry())
	metrics.setCertificates(certs)
	var isReady atomic.Bool
	isReady.Store(true)
	h := NewHandlers(nil, nil, &isReady)
	routes := h.Routes()
	for i := range routes {
		routes[i].Handler = metrics.instrument(routes[i])
	}
	server := httptest.NewUnstartedServer(auth.Middleware(newMux(t.Context(), routes), auth.ClientCertAuthenticator{}))
	server.TLS = serverConfig
	server.StartTLS()
	defer server.Close()
	gatherer := prometheus.NewRegistry()
	gatherer.MustRegister(prometheus.NewGauge(prometheus.GaugeOpts{Name: "from_the_gatherer"}))
	probe := httptest.NewServer(newMux(t.Context(), h.ProbeRoutes(metrics.handler(gatherer))))
	defer probe.Close()

	clientConfig, err := pki.CLIFiles(opts.CLIDir).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	client := http.Client{Transport: &http.Transport{TLSClientConfig: clientConfig}, Timeout: 5 * time.Second}
	for range 2 {
		resp, getErr := client.Get(server.URL + "/" + endpoints.APIVersion + "/" + string(endpoints.WhoAmIEndPoint))
		if getErr != nil {
			t.Fatal(getErr)
		}
		_ = resp.Body.Close()
	}

	resp, err := http.Get(probe.URL + "/" + string(endpoints.MetricsEndPoint))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, expected := range []string{
		`orion_api_requests_total{code="200",method="get",route="/v1/whoami"} 2`,
		`orion_api_request_duration_seconds_count{method="get",route="/v1/whoami"} 2`,
		`orion_api_certificate_expiry_timestamp_seconds{ca="false",common_name="` + pki.APIServerName + `"`,
		`orion_api_certificate_expiry_timestamp_seconds{ca="true",common_name="` + pki.ServerIntermediate + `"`,
		"from_the_gatherer 0",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("expected %q in the metrics:\n%s", expected, text)
		}
	}
	if count := strings.Count(text, "\norion_api_certificate_expiry_timestamp_seconds{"); count != len(certs) {
		t.Errorf("expected %d certificates in the metrics, got %d", len(certs), count)
	}
}
