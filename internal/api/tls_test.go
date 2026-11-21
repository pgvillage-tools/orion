package api

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
	"sync/atomic"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/auth"
	"github.com/pgvillage-tools/orion/internal/logging"
	mockconsensus "github.com/pgvillage-tools/orion/internal/mock/consensus"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/rs/zerolog"
)

var _ = Describe("TLS", func() {
	It("finds the TLS files in the PKI dir, unless they are overridden", func() {
		home, err := os.UserHomeDir()
		Expect(err).NotTo(HaveOccurred())
		c := Config{PKIDir: "~/.orion/api"}
		files, err := c.tlsFiles()
		Expect(err).NotTo(HaveOccurred())
		Expect(files.CertFile).To(Equal(filepath.Join(home, ".orion/api", pki.APIServerCertFile)))
		c.TLSKeyFile = "/etc/orion/key.pem"
		c.TLSCAFile = "~/ca.pem"
		files, err = c.tlsFiles()
		Expect(err).NotTo(HaveOccurred())
		Expect(files.KeyFile).To(Equal("/etc/orion/key.pem"), "overrides not applied: %+v", files)
		Expect(files.CAFile).To(Equal(filepath.Join(home, "ca.pem")), "overrides not applied: %+v", files)
	})

	It("serves the api to clients with a client certificate only", func(ctx SpecContext) {
		dir := GinkgoT().TempDir()
		opts := pki.InitOptions{CLIDir: filepath.Join(dir, "cli"), APIDir: filepath.Join(dir, "api")}
		_, err := pki.Init(opts)
		Expect(err).NotTo(HaveOccurred())
		serverConfig, err := pki.APIFiles(opts.APIDir).ServerConfig()
		Expect(err).NotTo(HaveOccurred())
		var isReady atomic.Bool
		isReady.Store(true)
		h := NewHandlers(nil, nil, &isReady)

		server := httptest.NewUnstartedServer(auth.Middleware(newMux(ctx, h.Routes()),
			auth.ClientCertAuthenticator{}))
		server.TLS = serverConfig
		server.StartTLS()
		DeferCleanup(server.Close)

		clientConfig, err := pki.CLIFiles(opts.CLIDir).ClientConfig()
		Expect(err).NotTo(HaveOccurred())
		get := func(config *tls.Config) (*http.Response, error) {
			client := http.Client{Transport: &http.Transport{TLSClientConfig: config}, Timeout: 5 * time.Second}
			return client.Get(server.URL + "/" + endpoints.APIVersion + "/" + string(endpoints.WhoAmIEndPoint))
		}

		resp, err := get(clientConfig)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = resp.Body.Close() })
		var identity endpoints.WhoAmIResponse
		Expect(json.NewDecoder(resp.Body).Decode(&identity)).To(Succeed())
		Expect(identity.Name).To(Equal(pki.CLIClientName), "unexpected identity %+v", identity)
		Expect(identity.Method).To(Equal(auth.MethodClientCert), "unexpected identity %+v", identity)

		noCert := clientConfig.Clone()
		noCert.Certificates = nil
		refused, refusedErr := get(noCert)
		if refusedErr == nil {
			_ = refused.Body.Close()
		}
		Expect(refusedErr).To(HaveOccurred(), "expected a client without a certificate to be refused")
	})

	It("serves the probes", func(ctx SpecContext) {
		var isReady atomic.Bool
		store := mockconsensus.NewMockStore(gomock.NewController(GinkgoT()))
		store.EXPECT().Healthy(gomock.Any()).Return(nil).AnyTimes()
		h := NewHandlers(store, nil, &isReady)
		probe := httptest.NewServer(newMux(ctx, h.ProbeRoutes(http.NotFoundHandler())))
		DeferCleanup(probe.Close)

		get := func(ep endpoints.ProbeEndPoint) *http.Response {
			GinkgoHelper()
			resp, err := http.Get(probe.URL + "/" + string(ep))
			Expect(err).NotTo(HaveOccurred())
			return resp
		}

		resp := get(endpoints.ReadyEndPoint)
		_ = resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable),
			"expected readyz to return 503 before the api is ready")
		isReady.Store(true)
		resp = get(endpoints.ReadyEndPoint)
		_ = resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK), "expected readyz to succeed")

		resp = get(endpoints.ModeEndPoint)
		DeferCleanup(func() { _ = resp.Body.Close() })
		var mode endpoints.ModeResponse
		Expect(json.NewDecoder(resp.Body).Decode(&mode)).To(Succeed())
		Expect(mode.Mode).To(Equal(endpoints.ModeNormal))
	})

	It("warns about certificates that expire soon", func(ctx SpecContext) {
		// a logger at the level of the api component, so GetLogComponent keeps it (and its output)
		_, componentLogger := logging.GetLogComponent(ctx, logging.WebApiComponent)
		var out bytes.Buffer
		logger := zerolog.New(&out).Level(componentLogger.GetLevel())
		logCtx := logger.WithContext(ctx)

		soon := &x509.Certificate{Subject: pkix.Name{CommonName: "orion-api"}, NotAfter: time.Now().Add(time.Hour)}
		later := &x509.Certificate{Subject: pkix.Name{CommonName: "orion-ca"},
			NotAfter: time.Now().Add(2 * pki.ExpiryWarning)}
		warnExpiring(logCtx, []*x509.Certificate{soon, later})

		Expect(out.String()).To(ContainSubstring("orion-api expires at"),
			"expected a warning for the certificate that expires soon")
		Expect(out.String()).NotTo(ContainSubstring("orion-ca"),
			"expected no warning for the certificate that expires later")
	})
})
