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

// Package cmd holds all CLI code for the keeper
package v1

import (
	"context"
	"crypto/x509"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	cmdcommon "github.com/pgvillage-tools/orion/cmd"
	"github.com/pgvillage-tools/orion/internal/auth"
	"github.com/pgvillage-tools/orion/internal/flagutil"
	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"
)

const (
	readMaxBytes int64 = 1 << 20

	defaultPort       = uint(8443)
	defaultProbePort  = uint(8080)
	defaultPKIDir     = "~/.orion/api"
	readHeaderTimeout = 10 * time.Second
	// defaultShutdownDelay is the default time /readyz returns 503 before the api stops, on SIGTERM
	defaultShutdownDelay = 5 * time.Second
	// shutdownTimeout is the time the servers get to finish their requests when the api stops
	shutdownTimeout   = 10 * time.Second
	expiryCheckPeriod = 24 * time.Hour
	keyError          = "error"
	decimal           = 10
)

type config struct {
	cmdcommon.CommonConfig

	baseURL     string
	port        uint
	probePort   uint
	debug       bool
	pkiDir      string
	tlsCertFile string
	tlsKeyFile  string
	tlsCAFile   string
	// shutdownDelay is the time between not being ready anymore and stopping the servers, on SIGTERM
	shutdownDelay time.Duration
}

var cfg config

func init() {
	cmdcommon.AddCommonFlags(CmdWebApi, &cfg.CommonConfig)
	// revive:disable
	CmdWebApi.PersistentFlags().StringVar(&cfg.baseURL, "baseurl", "0.0.0.0",
		"address for the api and probe ports to listen on")
	CmdWebApi.PersistentFlags().UintVar(&cfg.port, "port", defaultPort,
		"port for the api (TLS, a client certificate is required)")
	CmdWebApi.PersistentFlags().UintVar(&cfg.probePort, "probe-port", defaultProbePort,
		"port for /livez, /readyz, /status and /metrics (plain HTTP, no authentication)")
	CmdWebApi.PersistentFlags().DurationVar(&cfg.shutdownDelay, "shutdown-delay", defaultShutdownDelay,
		"on SIGTERM, the time /readyz returns 503 before the api stops, so load balancers stop sending requests")
	CmdWebApi.PersistentFlags().BoolVar(&cfg.debug, "debug", false,
		"enable debug logging")
	CmdWebApi.PersistentFlags().StringVar(&cfg.pkiDir, "pki-dir", defaultPKIDir,
		"directory with the certificates of the api, as created by orion-cli pki init")
	CmdWebApi.PersistentFlags().StringVar(&cfg.tlsCertFile, "tls-cert-file", "",
		"server certificate of the api, followed by its chain (default <pki-dir>/"+pki.APIServerCertFile+")")
	CmdWebApi.PersistentFlags().StringVar(&cfg.tlsKeyFile, "tls-key-file", "",
		"private key of the server certificate (default <pki-dir>/"+pki.APIServerKeyFile+")")
	CmdWebApi.PersistentFlags().StringVar(&cfg.tlsCAFile, "tls-ca-file", "",
		"CA that signs the client certificates the api accepts (default <pki-dir>/"+pki.APIClientCAFile+")")
	// revive:enable
}

var ready atomic.Bool

// CmdWebApi exports the main webapi process
var CmdWebApi = &cobra.Command{
	Use:     "api",
	Run:     api,
	Version: cmdcommon.Version,
}

// tlsFiles returns the files of the api: the files in the pki directory, unless a file is set explicitly
func (c config) tlsFiles() (pki.TLSFiles, error) {
	files := pki.APIFiles(c.pkiDir)
	for _, override := range []struct {
		value  string
		target *string
	}{
		{c.tlsCertFile, &files.CertFile},
		{c.tlsKeyFile, &files.KeyFile},
		{c.tlsCAFile, &files.CAFile},
	} {
		if override.value != "" {
			*override.target = override.value
		}
	}
	return files.Expand()
}

// warnExpiring warns about the certificates (as loaded at start) that expire soon. The api reads its certificates
// at start only, so new files need a restart.
func warnExpiring(ctx context.Context, certs []*x509.Certificate) {
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	for _, e := range pki.ExpiringCertificates(certs, time.Now(), pki.ExpiryWarning) {
		logger.Warn().Str("certificate", e).Msg("certificate expires soon; renew it with the cli and restart the api")
	}
}

func newMux(ctx context.Context, routes []Route) *http.ServeMux {
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	mux := http.NewServeMux()
	for _, route := range routes {
		mux.HandleFunc(route.Pattern, route.Handler)
		logger.Debug().Str("pattern", route.Pattern).Msg("registered route")
	}
	return mux
}

func listen(ctx context.Context, port uint) net.Listener {
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	addr := net.JoinHostPort(cfg.baseURL, strconv.FormatUint(uint64(port), decimal))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Fatal().AnErr(keyError, err).Str("address", addr).Msg("binding failed")
	}
	return ln
}

func api(c *cobra.Command, _ []string) {
	ctx, logger := logging.GetLogComponent(context.Background(), logging.WebApiComponent)

	if cfg.debug {
		logging.SetStaticLevel("debug")
	}
	if cmdcommon.IsColorLoggerEnable(c, &cfg.CommonConfig) {
		logging.EnableColor()
	}

	if err := cmdcommon.CheckCommonConfig(&cfg.CommonConfig); err != nil {
		logger.Fatal().AnErr(keyError, err).Msg("")
	}

	files, err := cfg.tlsFiles()
	if err != nil {
		logger.Fatal().AnErr(keyError, err).Msg("")
	}
	if !files.Exist() {
		logger.Fatal().
			Str("cert", files.CertFile).
			Str("key", files.KeyFile).
			Str("ca", files.CAFile).
			Msg("certificates not found: create them with `orion-cli pki init`, and copy the api files to " +
				"--pki-dir (starting without certificates is not supported yet)")
	}
	tlsConfig, err := files.ServerConfig()
	if err != nil {
		logger.Fatal().AnErr(keyError, err).Msg("failed to load the certificates")
	}
	certs, err := files.Certificates()
	if err != nil {
		logger.Fatal().AnErr(keyError, err).Msg("failed to load the certificates")
	}
	metrics := newAPIMetrics(prometheus.NewRegistry())
	metrics.setCertificates(certs)
	cmdcommon.SetMetrics(&cfg.CommonConfig, "api")
	warnExpiring(ctx, certs)
	go func() {
		for range time.Tick(expiryCheckPeriod) {
			warnExpiring(ctx, certs)
		}
	}()

	// one store connection for the whole api, shared by the store and the election client
	cl, election, err := cmdcommon.NewStoreAndElection(ctx, &cfg.CommonConfig, "")
	if err != nil {
		logger.Fatal().AnErr(keyError, err).Msg("")
	}

	h := NewHandlers(cl, election, &ready)
	logger.Debug().Msg("registering routes")
	routes := h.Routes()
	for i := range routes {
		routes[i].Handler = metrics.instrument(routes[i])
	}
	srv := &http.Server{
		Handler:           auth.Middleware(newMux(ctx, routes), auth.ClientCertAuthenticator{}),
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	probe := &http.Server{
		Handler:           newMux(ctx, h.ProbeRoutes(metrics.handler(prometheus.DefaultGatherer))),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	logger.Info().Str("host", cfg.baseURL).Uint("port", cfg.port).Uint("probe-port", cfg.probePort).
		Msg("starting server")
	ln := listen(ctx, cfg.port)
	probeLn := listen(ctx, cfg.probePort)

	stopCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt)
	defer stop()
	errs := make(chan error, 2)
	go func() { errs <- probe.Serve(probeLn) }()
	go func() { errs <- srv.ServeTLS(ln, "", "") }()

	ready.Store(true)
	logger.Info().Str("host", cfg.baseURL).Uint("port", cfg.port).Uint("probe-port", cfg.probePort).
		Str("mode", endpoints.ModeNormal).Msg("server ready")

	select {
	case err = <-errs:
		logger.Fatal().AnErr(keyError, err).Msg("server failing")
	case <-stopCtx.Done():
		shutdown(ctx, srv, probe)
	}
}

// shutdown stops the api gracefully: /readyz returns 503 first, and after cfg.shutdownDelay (so load balancers and
// Kubernetes stop sending requests) the servers finish their requests and stop.
func shutdown(ctx context.Context, srv, probe *http.Server) {
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	ready.Store(false)
	logger.Info().Dur("delay", cfg.shutdownDelay).Msg("shutting down: not ready anymore")
	time.Sleep(cfg.shutdownDelay)

	shutdownCtx, cancelFunc := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelFunc()
	for name, s := range map[string]*http.Server{"api": srv, "probe": probe} {
		if err := s.Shutdown(shutdownCtx); err != nil {
			logger.Error().AnErr(keyError, err).Str("server", name).Msg("graceful shutdown failed")
		}
	}
	logger.Info().Msg("stopped")
}

// Execute can be used to start and run the API
func Execute() {
	_, logger := logging.GetLogComponent(context.Background(), logging.WebApiComponent)
	if err := flagutil.SetFlagsFromEnv(CmdWebApi.PersistentFlags(), "ORIONAPI"); err != nil {
		logger.Fatal().AnErr("err", err).Msg("")
	}

	if err := CmdWebApi.Execute(); err != nil {
		logger.Fatal().AnErr("err", err).Msg("")
	}
}

// handleError logs err with msg, and (when w is set) responds with a 500 problem with msg as the detail (or the error,
// when msg is empty)
func handleError(ctx context.Context, err error, w http.ResponseWriter, msg string) {
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	if err != nil {
		logger.Error().AnErr("error", err).Msg(msg)
	}
	if w == nil {
		return
	}
	detail := msg
	if detail == "" && err != nil {
		detail = err.Error()
	}
	endpoints.WriteProblem(w, http.StatusInternalServerError, detail)
}
