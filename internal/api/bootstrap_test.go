// Copyright 2026 PgVillage
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied
// See the License for the specific language governing permissions and
// limitations under the License.

package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mockconsensus "github.com/pgvillage-tools/orion/internal/mock/consensus"
	"github.com/pgvillage-tools/orion/internal/pki"
	apiclient "github.com/pgvillage-tools/orion/pkg/api_client"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/pgvillage-tools/orion/pkg/bootstrap"
	"github.com/prometheus/client_golang/prometheus"
)

// freePort returns a tcp port on 127.0.0.1 that was free a moment ago
func freePort() uint {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = ln.Close() }()
	return uint(ln.Addr().(*net.TCPAddr).Port)
}

// initPKI creates the files of the cli and the api in dir/cli and dir/api, as orion-cli pki init does
func initPKI(dir string) pki.InitOptions {
	opts := pki.InitOptions{CLIDir: filepath.Join(dir, "cli"), APIDir: filepath.Join(dir, "api")}
	_, err := pki.Init(opts)
	Expect(err).NotTo(HaveOccurred())
	return opts
}

var _ = Describe("startsInBootstrap", func() {
	var dir string

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
	})

	// check returns the result of startsInBootstrap for cfg, with the files in its pki dir
	check := func(cfg Config) (bool, error) {
		files, err := cfg.tlsFiles()
		Expect(err).NotTo(HaveOccurred())
		return startsInBootstrap(cfg, files)
	}

	It("starts in normal mode with certificates", func() {
		Expect(check(Config{PKIDir: initPKI(dir).APIDir})).To(BeFalse())
	})

	It("starts in bootstrap mode without any certificates", func() {
		Expect(check(Config{PKIDir: dir})).To(BeTrue())
	})

	It("fails with a part of the certificates", func() {
		Expect(os.WriteFile(filepath.Join(dir, pki.APIServerCertFile), []byte("x"), 0o600)).To(Succeed())
		_, err := check(Config{PKIDir: dir})
		Expect(err).To(MatchError(pki.ErrFilesNotFound))
	})

	It("fails when a certificate file is set explicitly, but missing", func() {
		_, err := check(Config{PKIDir: dir, TLSCertFile: filepath.Join(dir, "elsewhere.pem")})
		Expect(err).To(MatchError(pki.ErrFilesNotFound))
	})

	It("fails with unreadable certificates", func() {
		if os.Getuid() == 0 {
			Skip("root reads every file")
		}
		for _, f := range []string{pki.APIServerCertFile, pki.APIServerKeyFile, pki.APIClientCAFile} {
			Expect(os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o000)).To(Succeed())
		}
		_, err := check(Config{PKIDir: dir})
		Expect(err).To(MatchError(ContainSubstring("readable")))
	})
})

var _ = Describe("selfSignedCert", func() {
	It("is a self-signed server certificate for localhost", func() {
		now := time.Now()
		cert, err := selfSignedCert(now)
		Expect(err).NotTo(HaveOccurred())
		x, err := x509.ParseCertificate(cert.Certificate[0])
		Expect(err).NotTo(HaveOccurred())
		Expect(x.NotBefore).To(BeTemporally("~", now.Add(-bootstrapCertBackdate), time.Second))
		Expect(x.NotAfter).To(BeTemporally("~", now.Add(bootstrapCertValidity), time.Second))

		roots := x509.NewCertPool()
		roots.AddCert(x)
		for _, name := range []string{"localhost", "127.0.0.1", "::1"} {
			_, err = x.Verify(x509.VerifyOptions{Roots: roots, DNSName: name})
			Expect(err).NotTo(HaveOccurred(), "not valid for %s", name)
		}
	})
})

var _ = Describe("run", func() {
	var (
		opts   pki.InitOptions
		cfg    Config
		h      *Handlers
		ctx    context.Context
		cancel context.CancelFunc
		done   chan error
		// stopped is closed when run returned
		stopped  chan struct{}
		apiURL   string
		probeURL string
	)

	BeforeEach(func() {
		dir := GinkgoT().TempDir()
		// the cli files exist (the cli made them while bootstrapping), the api files come at the end of the bootstrap
		opts = pki.InitOptions{CLIDir: filepath.Join(dir, "cli"), APIDir: filepath.Join(dir, "api")}
		cfg = Config{PKIDir: opts.APIDir, BaseURL: "127.0.0.1", Port: freePort(), ProbePort: freePort(),
			ShutdownDelay: time.Millisecond}
		apiURL = "https://127.0.0.1:" + strconv.FormatUint(uint64(cfg.Port), 10)
		probeURL = "http://127.0.0.1:" + strconv.FormatUint(uint64(cfg.ProbePort), 10)

		store := mockconsensus.NewMockStore(gomock.NewController(GinkgoT()))
		store.EXPECT().Healthy(gomock.Any()).Return(nil).AnyTimes()
		h = NewHandlers(store, nil, &atomic.Bool{})
		ctx, cancel = context.WithCancel(context.Background())
		done = make(chan error, 1)
		stopped = make(chan struct{})
		DeferCleanup(func() {
			cancel()
			Eventually(stopped).WithTimeout(5 * time.Second).Should(BeClosed())
		})
	})

	start := func() {
		files, err := cfg.tlsFiles()
		Expect(err).NotTo(HaveOccurred())
		bootstrap, err := startsInBootstrap(cfg, files)
		Expect(err).NotTo(HaveOccurred())
		go func() {
			defer close(stopped)
			done <- run(ctx, cfg, files, bootstrap, h, newAPIMetrics(prometheus.NewRegistry()))
		}()
	}

	// mode returns the mode on the probe port, "" when it doesn't answer
	mode := func() string {
		resp, err := http.Get(probeURL + "/" + string(endpoints.ModeEndPoint))
		if err != nil {
			return ""
		}
		defer func() { _ = resp.Body.Close() }()
		var m endpoints.ModeResponse
		if json.NewDecoder(resp.Body).Decode(&m) != nil {
			return ""
		}
		return m.Mode
	}

	// get requests path on the api port with config
	get := func(config *tls.Config, path string) (*http.Response, error) {
		client := http.Client{Transport: &http.Transport{TLSClientConfig: config}, Timeout: 5 * time.Second}
		return client.Get(apiURL + path)
	}
	whoami := "/" + endpoints.APIVersion + "/" + string(endpoints.WhoAmIEndPoint)

	It("serves bootstrap mode without certificates, and switches to normal mode once the bootstrap is done", func() {
		start()
		Eventually(mode).WithTimeout(5 * time.Second).Should(Equal(endpoints.ModeBootstrap))
		// /status answers as soon as the probe port serves, a moment before run marks the api ready
		Eventually(h.Ready.Load).WithTimeout(5*time.Second).Should(BeTrue(), "ready in bootstrap mode")

		// a client without a certificate connects (it can't verify the self-signed certificate), but gets no api
		insecure := &tls.Config{InsecureSkipVerify: true} //nolint:gosec // the self-signed bootstrap certificate
		resp, err := get(insecure, whoami)
		Expect(err).NotTo(HaveOccurred())
		_ = resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound), "the api is served in bootstrap mode")

		// a bootstrap connection that stays open
		conn, err := tls.Dial("tcp", "127.0.0.1:"+strconv.FormatUint(uint64(cfg.Port), 10), insecure)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(conn.Close)

		// the bootstrap writes the certificates of the api, and is done
		initPKI(filepath.Dir(opts.APIDir))
		h.finishBootstrap()
		Eventually(mode).WithTimeout(5 * time.Second).Should(Equal(endpoints.ModeNormal))

		Expect(conn.SetReadDeadline(time.Now().Add(5 * time.Second))).To(Succeed())
		_, err = conn.Read(make([]byte, 1))
		Expect(err).To(MatchError(io.EOF), "a bootstrap connection stays open into normal mode")

		clientConfig, err := pki.CLIFiles(opts.CLIDir).ClientConfig()
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() (int, error) {
			resp, err := get(clientConfig, whoami)
			if err != nil {
				return 0, err
			}
			_ = resp.Body.Close()
			return resp.StatusCode, nil
		}).WithTimeout(5*time.Second).Should(Equal(http.StatusOK), "the api in normal mode")
		noCert := clientConfig.Clone()
		noCert.Certificates = nil
		if resp, err = get(noCert, whoami); err == nil {
			_ = resp.Body.Close()
		}
		Expect(err).To(HaveOccurred(), "a client without a certificate in normal mode")

		cancel()
		Eventually(done).WithTimeout(5 * time.Second).Should(Receive(BeNil()))
	})

	It("bootstraps with the cli: archive, stored, normal mode, and the confirmation over mTLS", func() {
		h.bootstrap = newBootstrapper([]byte(testSecret), time.Hour)
		h.bootstrap.dir = opts.APIDir
		h.pkiDir = opts.APIDir
		archiveFile := filepath.Join(opts.APIDir, pki.APIBootstrapArchiveFile)
		start()
		Eventually(mode).WithTimeout(5 * time.Second).Should(Equal(endpoints.ModeBootstrap))
		Eventually(h.Ready.Load).WithTimeout(5 * time.Second).Should(BeTrue())

		priv, err := bootstrap.GenerateKey()
		Expect(err).NotTo(HaveOccurred())
		session, err := bootstrap.Start(ctx, bootstrap.NewHTTPClient(5*time.Second), apiURL, []byte(testSecret))
		Expect(err).NotTo(HaveOccurred())
		subject := pki.SubjectOptions{Organisation: "Example", Country: "NL"}
		certs, keys, err := session.Archive(ctx, priv, []string{"lb.example.org"}, subject.Bootstrap())
		Expect(err).NotTo(HaveOccurred())
		Expect(pki.VerifyArchive(certs, keys, time.Now())).To(Succeed())
		Expect(pki.CheckArchiveSubject(certs, subject)).To(Succeed())
		Expect(pki.StoreArchive(opts.CLIDir, certs, keys, []byte("the CA passphrase"))).To(Succeed())
		Expect(session.Stored(ctx)).To(Succeed())
		Eventually(mode).WithTimeout(5 * time.Second).Should(Equal(endpoints.ModeNormal))
		Expect(archiveFile).To(BeAnExistingFile())

		// the cli verifies the api with the new root, and logs in with its new certificate
		clientConfig, err := pki.CLIFiles(opts.CLIDir).ClientConfig()
		Expect(err).NotTo(HaveOccurred())
		conn := apiclient.NewTLSConnection("127.0.0.1", uint16(cfg.Port), 5*time.Second, clientConfig) //nolint:gosec
		for range 2 {
			Eventually(conn.ConfirmBootstrap).WithTimeout(5 * time.Second).Should(Equal(http.StatusOK))
		}
		Expect(archiveFile).NotTo(BeAnExistingFile(), "the api deleted the archive with the CA keys")
		Expect(readBootstrapState(opts.APIDir)).To(Equal(bootstrapState{State: stateConfirmed}))

		// a spare connection the client dialed but never used would hold up the shutdown for 5 seconds
		conn.CloseIdleConnections()
		cancel()
		Eventually(done).WithTimeout(5 * time.Second).Should(Receive(BeNil()))
	})

	It("fails when the certificates aren't there once the bootstrap is done", func() {
		start()
		Eventually(mode).WithTimeout(5 * time.Second).Should(Equal(endpoints.ModeBootstrap))
		h.finishBootstrap()
		h.finishBootstrap()
		Eventually(done).WithTimeout(5 * time.Second).Should(Receive(MatchError(ContainSubstring(
			"failed to load the certificates"))))
	})

	It("serves normal mode with certificates", func() {
		initPKI(filepath.Dir(opts.APIDir))
		start()
		Eventually(mode).WithTimeout(5 * time.Second).Should(Equal(endpoints.ModeNormal))
	})
})

var _ = Describe("Run in bootstrap mode", func() {
	It("fails with an invalid bootstrap secret", func(ctx SpecContext) {
		cfg := Config{PKIDir: GinkgoT().TempDir(), BaseURL: "127.0.0.1", BootstrapSecret: "too short"}
		cfg.ClusterName = "cluster1"
		cfg.StoreBackend = "etcdv3"
		// nothing listens there: the store client connects on its first call
		cfg.StoreEndpoints = "http://127.0.0.1:1"
		cfg.StorePrefix = "orion/cluster"
		Expect(Run(ctx, cfg)).To(MatchError(ContainSubstring("at least")))
	})
})
