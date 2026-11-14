package v1

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/pgvillage-tools/orion/internal/auth"
	"github.com/pgvillage-tools/orion/internal/logging"
	mockconsensus "github.com/pgvillage-tools/orion/internal/mock/consensus"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/rs/zerolog"
)

func TestTLSFiles(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	c := config{pkiDir: "~/.orion/api"}
	files, err := c.tlsFiles()
	if err != nil {
		t.Fatal(err)
	}
	if files.CertFile != filepath.Join(home, ".orion/api", pki.APIServerCertFile) {
		t.Errorf("unexpected cert file %s", files.CertFile)
	}
	c.tlsKeyFile = "/etc/orion/key.pem"
	c.tlsCAFile = "~/ca.pem"
	files, err = c.tlsFiles()
	if err != nil {
		t.Fatal(err)
	}
	if files.KeyFile != "/etc/orion/key.pem" || files.CAFile != filepath.Join(home, "ca.pem") {
		t.Errorf("overrides not applied: %+v", files)
	}
}

func TestAPIWithClientCertificates(t *testing.T) {
	dir := t.TempDir()
	opts := pki.InitOptions{CLIDir: filepath.Join(dir, "cli"), APIDir: filepath.Join(dir, "api")}
	if _, err := pki.Init(opts); err != nil {
		t.Fatal(err)
	}
	serverConfig, err := pki.APIFiles(opts.APIDir).ServerConfig()
	if err != nil {
		t.Fatal(err)
	}
	var isReady atomic.Bool
	isReady.Store(true)
	h := NewHandlers(nil, nil, &isReady)

	server := httptest.NewUnstartedServer(auth.Middleware(newMux(t.Context(), h.Routes()),
		auth.ClientCertAuthenticator{}))
	server.TLS = serverConfig
	server.StartTLS()
	defer server.Close()

	clientConfig, err := pki.CLIFiles(opts.CLIDir).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	get := func(config *tls.Config) (*http.Response, error) {
		client := http.Client{Transport: &http.Transport{TLSClientConfig: config}, Timeout: 5 * time.Second}
		return client.Get(server.URL + "/" + endpoints.APIVersion + "/" + string(endpoints.WhoAmIEndPoint))
	}

	resp, err := get(clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var identity endpoints.WhoAmIResponse
	if err = json.NewDecoder(resp.Body).Decode(&identity); err != nil {
		t.Fatal(err)
	}
	if identity.Name != pki.CLIClientName || identity.Method != auth.MethodClientCert {
		t.Errorf("unexpected identity %+v", identity)
	}

	noCert := clientConfig.Clone()
	noCert.Certificates = nil
	if refused, refusedErr := get(noCert); refusedErr == nil {
		_ = refused.Body.Close()
		t.Error("expected a client without a certificate to be refused")
	}
}

func TestProbeRoutes(t *testing.T) {
	var isReady atomic.Bool
	store := mockconsensus.NewMockStore(gomock.NewController(t))
	store.EXPECT().Healthy(gomock.Any()).Return(nil).AnyTimes()
	h := NewHandlers(store, nil, &isReady)
	probe := httptest.NewServer(newMux(t.Context(), h.ProbeRoutes(http.NotFoundHandler())))
	defer probe.Close()

	get := func(ep endpoints.ProbeEndPoint) *http.Response {
		resp, err := http.Get(probe.URL + "/" + string(ep))
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	resp := get(endpoints.ReadyEndPoint)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected readyz to return 503 before the api is ready, got %d", resp.StatusCode)
	}
	isReady.Store(true)
	resp = get(endpoints.ReadyEndPoint)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected readyz to succeed, got %d", resp.StatusCode)
	}

	resp = get(endpoints.ModeEndPoint)
	defer func() { _ = resp.Body.Close() }()
	var mode endpoints.ModeResponse
	if err := json.NewDecoder(resp.Body).Decode(&mode); err != nil {
		t.Fatal(err)
	}
	if mode.Mode != endpoints.ModeNormal {
		t.Errorf("unexpected mode %q", mode.Mode)
	}
}

func TestWarnExpiring(t *testing.T) {
	// a logger at the level of the api component, so GetLogComponent keeps it (and its output)
	_, componentLogger := logging.GetLogComponent(t.Context(), logging.WebApiComponent)
	var out bytes.Buffer
	logger := zerolog.New(&out).Level(componentLogger.GetLevel())
	ctx := logger.WithContext(t.Context())

	soon := &x509.Certificate{Subject: pkix.Name{CommonName: "orion-api"}, NotAfter: time.Now().Add(time.Hour)}
	later := &x509.Certificate{Subject: pkix.Name{CommonName: "orion-ca"},
		NotAfter: time.Now().Add(2 * pki.ExpiryWarning)}
	warnExpiring(ctx, []*x509.Certificate{soon, later})

	if !strings.Contains(out.String(), "orion-api expires at") {
		t.Errorf("expected a warning for the certificate that expires soon, got %q", out.String())
	}
	if strings.Contains(out.String(), "orion-ca") {
		t.Errorf("expected no warning for the certificate that expires later, got %q", out.String())
	}
}
