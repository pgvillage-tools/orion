package v1

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pgvillage-tools/orion/internal/auth"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
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
	h := NewHandlers(nil, &isReady)

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
	h := NewHandlers(nil, &isReady)
	probe := httptest.NewServer(newMux(t.Context(), h.ProbeRoutes()))
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
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected readyz to fail before the api is ready, got %d", resp.StatusCode)
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
