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
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"sync/atomic"

	"github.com/pgvillage-tools/orion/internal/auth"
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

// Run runs the api with cfg until it gets SIGTERM or an interrupt (then it shuts down gracefully), or a server fails
func Run(ctx context.Context, cfg Config) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)

	files, err := cfg.tlsFiles()
	if err != nil {
		return err
	}
	if err = files.Check(); err != nil {
		if errors.Is(err, pki.ErrFilesNotFound) {
			return fmt.Errorf("%w: create the certificates with `orion-cli pki init`, and copy the api files to "+
				"--pki-dir (starting without certificates is not supported yet)", err)
		}
		return fmt.Errorf("%w: the api must be able to read its certificates and its key: make them readable for "+
			"the user it runs as (uid 1000 in the container image)", err)
	}
	tlsConfig, err := files.ServerConfig()
	if err != nil {
		return fmt.Errorf("failed to load the certificates: %w", err)
	}
	certs, err := files.Certificates()
	if err != nil {
		return fmt.Errorf("failed to load the certificates: %w", err)
	}
	metrics := newAPIMetrics(prometheus.NewRegistry())
	metrics.setCertificates(certs)
	store.SetMetrics(&cfg.CommonConfig, "api")
	warnExpiring(ctx, certs)
	go func() {
		for range time.Tick(expiryCheckPeriod) {
			warnExpiring(ctx, certs)
		}
	}()

	// one store connection for the whole api, shared by the store and the election client
	cl, election, err := store.NewStoreAndElection(ctx, &cfg.CommonConfig, "")
	if err != nil {
		return err
	}

	var ready atomic.Bool
	h := NewHandlers(cl, election, &ready)
	h.storeTimeout = cfg.StoreTimeout
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

	logger.Info().Str("host", cfg.BaseURL).Uint("port", cfg.Port).Uint("probe-port", cfg.ProbePort).
		Msg("starting server")
	ln, err := listen(cfg.BaseURL, cfg.Port)
	if err != nil {
		return err
	}
	probeLn, err := listen(cfg.BaseURL, cfg.ProbePort)
	if err != nil {
		return err
	}

	stopCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt)
	defer stop()
	errs := make(chan error, 2)
	go func() { errs <- probe.Serve(probeLn) }()
	go func() { errs <- srv.ServeTLS(ln, "", "") }()

	ready.Store(true)
	logger.Info().Str("host", cfg.BaseURL).Uint("port", cfg.Port).Uint("probe-port", cfg.ProbePort).
		Str("mode", endpoints.ModeNormal).Msg("server ready")

	select {
	case err = <-errs:
		return fmt.Errorf("server failing: %w", err)
	case <-stopCtx.Done():
		shutdown(ctx, &ready, cfg.ShutdownDelay, srv, probe)
	}
	return nil
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

// handleError logs err with msg, and (when w is set) responds with a 500 problem with msg as the detail
func handleError(ctx context.Context, err error, w http.ResponseWriter, msg string) {
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	if err != nil {
		logger.Error().AnErr("error", err).Msg(msg)
	}
	if w == nil {
		return
	}
	// msg, not err: a store error can name store internals (endpoints, keys), so it is only logged
	endpoints.WriteProblem(w, http.StatusInternalServerError, msg)
}
