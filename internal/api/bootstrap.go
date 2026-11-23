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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/pki"
)

// bootstrapSupported enables bootstrap mode: an api started without certificates starts in it, instead of failing.
// It stays off until the bootstrap is complete (the shared secret, the archive and the confirmation, #320-#322):
// until then, an api without certificates fails with a clear error instead of waiting for a bootstrap that can't
// happen.
var bootstrapSupported = false

const (
	// bootstrapCertValidity is the validity of the temporary certificate of bootstrap mode. The cli doesn't verify it
	// (the HMAC of the bootstrap proves the api), and the api makes a new one at every start.
	bootstrapCertValidity = 7 * 24 * time.Hour
	// bootstrapCertBackdate makes the certificate valid on hosts with a slightly late clock
	bootstrapCertBackdate = 5 * time.Minute
	// serialBits is the size of the random serial number of the certificate
	serialBits = 128
	// bootstrapSwitchTimeout is the time the bootstrap server gets to finish its requests before normal mode starts;
	// the bootstrap requests are small, and a connection that stays open longer is closed
	bootstrapSwitchTimeout = time.Second
)

// startsInBootstrap returns whether the api starts in bootstrap mode: when bootstrap mode is supported, none of its
// files exist, and none is set explicitly (with your own certificates there is no bootstrap). Otherwise it returns
// the error of checking the files, if any.
func startsInBootstrap(cfg Config, files pki.TLSFiles) (bool, error) {
	err := files.Check()
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, pki.ErrFilesNotFound) {
		return false, fmt.Errorf("%w: the api must be able to read its certificates and its key: make them readable "+
			"for the user it runs as (uid 1000 in the container image)", err)
	}
	explicit := cfg.TLSCertFile != "" || cfg.TLSKeyFile != "" || cfg.TLSCAFile != ""
	if bootstrapSupported && !explicit && noneExist(files) {
		return true, nil
	}
	return false, fmt.Errorf("%w: create the certificates with `orion-cli pki init`, and copy the api files to "+
		"--pki-dir (starting without certificates is not supported yet)", err)
}

// noneExist returns whether none of the files exist: a part of them is a mistake (e.g. a file that wasn't copied),
// which bootstrap mode must not paper over
func noneExist(files pki.TLSFiles) bool {
	for _, p := range []string{files.CertFile, files.KeyFile, files.CAFile} {
		if _, err := os.Stat(p); !errors.Is(err, fs.ErrNotExist) {
			return false
		}
	}
	return true
}

// selfSignedCert returns the temporary self-signed certificate of the api port in bootstrap mode, for localhost, the
// loopback addresses and the hostname
func selfSignedCert(now time.Time) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), serialBits))
	if err != nil {
		return tls.Certificate{}, err
	}
	names := []string{"localhost"}
	if host, hostErr := os.Hostname(); hostErr == nil && host != "" && host != "localhost" {
		names = append(names, host)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "orion-api bootstrap"},
		NotBefore:    now.Add(-bootstrapCertBackdate),
		NotAfter:     now.Add(bootstrapCertValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     names,
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}

// bootstrapServer returns the server of the api port in bootstrap mode: a temporary self-signed certificate, no
// client certificate, and only the bootstrap routes
func bootstrapServer(ctx context.Context, h *Handlers, metrics *apiMetrics) (*http.Server, error) {
	cert, err := selfSignedCert(time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to create the certificate of bootstrap mode: %w", err)
	}
	routes := h.BootstrapRoutes()
	for i := range routes {
		routes[i].Handler = metrics.instrument(routes[i])
	}
	return &http.Server{
		Handler: newMux(ctx, routes),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.NoClientCert,
			MinVersion:   tls.VersionTLS13,
		},
		ReadHeaderTimeout: readHeaderTimeout,
	}, nil
}

// finishBootstrap makes the api switch from bootstrap mode to normal mode; it can be called more than once
func (h *Handlers) finishBootstrap() {
	h.bootstrapOnce.Do(func() { close(h.bootstrapDone) })
}

// switchToNormal stops the bootstrap server srv, which closes all its connections (a connection without a client
// certificate can't stay open into normal mode), and listens on port again for the normal server
func switchToNormal(ctx context.Context, ready *atomic.Bool, srv *http.Server, served <-chan error, host string,
	port uint) (net.Listener, error) {
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	logger.Info().Msg("bootstrap done: switching to normal mode")
	ready.Store(false)
	switchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), bootstrapSwitchTimeout)
	defer cancel()
	if err := srv.Shutdown(switchCtx); err != nil {
		logger.Debug().AnErr(keyError, err).Msg("closing the bootstrap connections that are still open")
	}
	// Shutdown leaves the connections that are still open after the timeout: close those too
	_ = srv.Close()
	// ServeTLS returns http.ErrServerClosed now
	_ = <-served
	return listen(host, port)
}
