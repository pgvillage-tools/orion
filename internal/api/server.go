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
package api

import (
	"context"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"sync/atomic"

	"github.com/pgvillage-tools/orion/internal/auth"
	"github.com/pgvillage-tools/orion/internal/consensus"
	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/pki"
	"github.com/pgvillage-tools/orion/internal/store"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	readMaxBytes      int64 = 1 << 20
	readHeaderTimeout       = 10 * time.Second
	// shutdownTimeout is the time the servers get to finish their requests when the api stops
	shutdownTimeout   = 10 * time.Second
	expiryCheckPeriod = 24 * time.Hour
	keyError          = "error"
	decimal           = 10
)

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

// listen listens on host:port
func listen(host string, port uint) (net.Listener, error) {
	addr := net.JoinHostPort(host, strconv.FormatUint(uint64(port), decimal))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("binding %s failed: %w", addr, err)
	}
	return ln, nil
}

// Run runs the api with cfg until it gets SIGTERM or an interrupt (then it shuts down gracefully), or a server fails.
// Without certificates (when supported) the api port starts in bootstrap mode, and switches to normal mode once the
// bootstrap is done.
func Run(ctx context.Context, cfg Config) error {
	ctx, _ = logging.GetLogComponent(ctx, logging.WebApiComponent)

	files, err := cfg.tlsFiles()
	if err != nil {
		return err
	}
	// the bootstrap stores its files in the api directory: only when no file is set explicitly
	dir := filepath.Dir(files.CertFile)
	var state bootstrapState
	if cfg.explicitTLSFiles() {
		if cfg.Rebootstrap {
			return errRebootstrapExplicit
		}
	} else if state, err = prepareBootstrap(ctx, dir, cfg.Rebootstrap); err != nil {
		return err
	}
	bootstrap, err := startsInBootstrap(cfg, files)
	if err != nil {
		return err
	}
	if !bootstrap {
		if err = checkStateInNormalMode(ctx, dir, state); err != nil {
			return err
		}
	}
	metrics := newAPIMetrics(prometheus.NewRegistry())
	store.SetMetrics(&cfg.CommonConfig, "api")

	// one store connection for the whole api, shared by the store and the election client
	cl, election, err := store.NewStoreAndElection(ctx, &cfg.CommonConfig, "")
	if err != nil {
		return err
	}

	var ready atomic.Bool
	h := NewHandlers(cl, election, &ready)
	h.storeTimeout = cfg.StoreTimeout
	if !cfg.explicitTLSFiles() {
		h.pkiDir = dir
	}
	if bootstrap {
		if h.bootstrap, err = newBootstrapperFromConfig(ctx, cfg, dir, state); err != nil {
			return err
		}
	}
	return run(ctx, cfg, files, bootstrap, h, metrics)
}

// run serves the api port (starting in bootstrap mode when bootstrap is set) and the probe port with h, until it gets
// SIGTERM or an interrupt, or ctx is done (then it shuts down gracefully), or a server fails
func run(ctx context.Context, cfg Config, files pki.TLSFiles, bootstrap bool, h *Handlers, metrics *apiMetrics) error {
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	ready := h.Ready
	h.bootstrapping.Store(bootstrap)
	probe := &http.Server{
		Handler:           newMux(ctx, h.ProbeRoutes(metrics.handler(prometheus.DefaultGatherer))),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	logger.Info().Str("host", cfg.BaseURL).Uint("port", cfg.Port).Uint("probe-port", cfg.ProbePort).
		Msg("starting server")
	ln, err := listen(cfg.BaseURL, cfg.Port)
	if err != nil {
		return err
	}
	// the port normal mode listens on after bootstrap mode (with --port 0, the free port bootstrap mode got)
	port := uint(ln.Addr().(*net.TCPAddr).Port) //nolint:gosec // a tcp port fits
	probeLn, err := listen(cfg.BaseURL, cfg.ProbePort)
	if err != nil {
		return err
	}

	stopCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt)
	defer stop()
	probeErr := make(chan error, 1)
	go func() { probeErr <- probe.Serve(probeLn) }()

	for {
		var srv *http.Server
		if bootstrap {
			srv, err = bootstrapServer(ctx, h, metrics)
		} else {
			srv, err = normalServer(ctx, files, h, metrics)
		}
		if err != nil {
			return err
		}
		apiErr := make(chan error, 1)
		go func(srv *http.Server, ln net.Listener) { apiErr <- srv.ServeTLS(ln, "", "") }(srv, ln)

		ready.Store(true)
		logger.Info().Str("host", cfg.BaseURL).Uint("port", port).Uint("probe-port", cfg.ProbePort).
			Str("mode", h.mode()).Msg("server ready")

		// only bootstrap mode ends with the bootstrap; a nil channel never fires
		var bootstrapDone <-chan struct{}
		if bootstrap {
			bootstrapDone = h.bootstrapDone
		}
		select {
		case err = <-apiErr:
			return fmt.Errorf("server failing: %w", err)
		case err = <-probeErr:
			return fmt.Errorf("probe server failing: %w", err)
		case <-stopCtx.Done():
			shutdown(ctx, ready, cfg.ShutdownDelay, srv, probe)
			return nil
		case <-bootstrapDone:
			if ln, err = switchToNormal(ctx, ready, srv, apiErr, cfg.BaseURL, port); err != nil {
				return err
			}
			bootstrap = false
			h.bootstrapping.Store(false)
		}
	}
}

// normalServer returns the server of the api port in normal mode: the certificates in files, a client certificate is
// required, and the api routes
func normalServer(ctx context.Context, files pki.TLSFiles, h *Handlers, metrics *apiMetrics) (*http.Server, error) {
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	tlsConfig, err := files.ServerConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load the certificates: %w", err)
	}
	certs, err := files.Certificates()
	if err != nil {
		return nil, fmt.Errorf("failed to load the certificates: %w", err)
	}
	metrics.setCertificates(certs)
	warnExpiring(ctx, certs)
	go func() {
		for range time.Tick(expiryCheckPeriod) {
			warnExpiring(ctx, certs)
		}
	}()

	logger.Debug().Msg("registering routes")
	routes := h.Routes()
	for i := range routes {
		routes[i].Handler = metrics.instrument(routes[i])
	}
	return &http.Server{
		Handler:           auth.Middleware(newMux(ctx, routes), auth.ClientCertAuthenticator{}),
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: readHeaderTimeout,
	}, nil
}

// shutdown stops the api gracefully: /readyz returns 503 first (ready is false), and after delay (so load balancers
// and Kubernetes stop sending requests) the servers finish their requests and stop.
func shutdown(ctx context.Context, ready *atomic.Bool, delay time.Duration, srv, probe *http.Server) {
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	ready.Store(false)
	logger.Info().Dur("delay", delay).Msg("shutting down: not ready anymore")
	time.Sleep(delay)

	shutdownCtx, cancelFunc := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelFunc()
	for name, s := range map[string]*http.Server{"api": srv, "probe": probe} {
		if err := s.Shutdown(shutdownCtx); err != nil {
			logger.Error().AnErr(keyError, err).Str("server", name).Msg("graceful shutdown failed")
		}
	}
	logger.Info().Msg("stopped")
}

// storeRetryAfter is the Retry-After (in seconds) of a 503 for a store that didn't answer in time
const storeRetryAfter = "2"

// handleError logs err with msg, and (when w is set) responds with a problem with msg as the detail: a 503 with
// Retry-After when the store didn't answer in time or can't serve requests right now (the request may succeed when
// retried; logged as a warning), a 500 otherwise (logged as an error)
func handleError(ctx context.Context, err error, w http.ResponseWriter, msg string) {
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	if err != nil {
		event := logger.Error()
		if consensus.IsUnavailable(err) {
			event = logger.Warn()
		}
		event.AnErr("error", err).Msg(msg)
	}
	if w == nil {
		return
	}
	status := http.StatusInternalServerError
	if consensus.IsUnavailable(err) {
		status = http.StatusServiceUnavailable
		w.Header().Set("Retry-After", storeRetryAfter)
	}
	// msg, not err: a store error can name store internals (endpoints, keys), so it is only logged
	endpoints.WriteProblem(w, status, msg)
}
