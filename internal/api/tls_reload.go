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
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/pki"
)

// certReloadPeriod is how often the api checks its certificate files for changes
const certReloadPeriod = time.Minute

// loadedCerts is a set of certificates the api serves with: its certificate and key, and the CA of the clients
type loadedCerts struct {
	// config is the TLS config of a connection with these certificates
	config *tls.Config
	// certs are all certificates in the files (for the metrics and the expiry warnings)
	certs []*x509.Certificate
	// digest is a hash of the contents of the files, to see whether they changed
	digest [sha256.Size]byte
}

// certReloader serves the api port with the certificates in its files, and loads them again when they change: on
// SIGHUP, and every certReloadPeriod. New files are only used when they are valid together; otherwise the api keeps
// the certificates it has.
type certReloader struct {
	files   pki.TLSFiles
	metrics *apiMetrics
	current atomic.Pointer[loadedCerts]
	// mu makes reloads run one at a time
	mu sync.Mutex
	// now is the time, replaced in the tests
	now func() time.Time
}

// newCertReloader loads the certificates in files, and fails when they can't be used
func newCertReloader(files pki.TLSFiles, metrics *apiMetrics) (*certReloader, error) {
	r := &certReloader{files: files, metrics: metrics, now: time.Now}
	// at start, an expired certificate is used (with a warning, by the caller): the api must run to renew it online
	loaded, err := r.load(false)
	if err != nil {
		return nil, err
	}
	r.use(loaded)
	return r, nil
}

// readFiles returns the contents of the certificate, the key and the CA file
func (r *certReloader) readFiles() (cert, key, ca []byte, err error) {
	if cert, err = os.ReadFile(r.files.CertFile); err != nil {
		return nil, nil, nil, err
	}
	if key, err = os.ReadFile(r.files.KeyFile); err != nil {
		return nil, nil, nil, err
	}
	if ca, err = os.ReadFile(r.files.CAFile); err != nil {
		return nil, nil, nil, err
	}
	return cert, key, ca, nil
}

func digestOf(parts ...[]byte) [sha256.Size]byte {
	return sha256.Sum256(bytes.Join(parts, []byte{0}))
}

// load reads and checks the files: the key must belong to the certificate, the certificate must be signed by the
// next certificate in the file (its intermediate), and the CA file must hold a certificate. With checkValidity, the
// certificate must also be valid now.
func (r *certReloader) load(checkValidity bool) (*loadedCerts, error) {
	certPEM, keyPEM, caPEM, err := r.readFiles()
	if err != nil {
		return nil, err
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("the key in %s doesn't belong to the certificate in %s: %w", r.files.KeyFile,
			r.files.CertFile, err)
	}
	_, chain, err := pki.LoadCertPool(r.files.CertFile)
	if err != nil {
		return nil, err
	}
	leaf := chain[0]
	now := r.now()
	if checkValidity && (now.After(leaf.NotAfter) || now.Before(leaf.NotBefore)) {
		return nil, fmt.Errorf("the certificate in %s is valid from %s to %s only", r.files.CertFile,
			leaf.NotBefore.Format(time.RFC3339), leaf.NotAfter.Format(time.RFC3339))
	}
	if len(chain) > 1 {
		if err = leaf.CheckSignatureFrom(chain[1]); err != nil {
			return nil, fmt.Errorf("the certificate in %s isn't signed by the next one in the file: %w",
				r.files.CertFile, err)
		}
	}
	clientCAs, caCerts, err := pki.LoadCertPool(r.files.CAFile)
	if err != nil {
		return nil, err
	}
	return &loadedCerts{
		config: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			Certificates: []tls.Certificate{pair},
			ClientCAs:    clientCAs,
			ClientAuth:   tls.RequireAndVerifyClientCert,
		},
		certs:  append(chain, caCerts...),
		digest: digestOf(certPEM, keyPEM, caPEM),
	}, nil
}

// use makes the api serve new connections with loaded
func (r *certReloader) use(loaded *loadedCerts) {
	r.current.Store(loaded)
	r.metrics.setCertificates(loaded.certs)
}

// errUnchanged is returned by reload when the files didn't change
var errUnchanged = errors.New("the certificate files didn't change")

// reload loads the files again, and uses them when they changed and are valid. It returns errUnchanged when they
// didn't change, and the reason when they can't be used (the api then keeps its certificates).
func (r *certReloader) reload() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	certPEM, keyPEM, caPEM, err := r.readFiles()
	if err != nil {
		return err
	}
	if digestOf(certPEM, keyPEM, caPEM) == r.current.Load().digest {
		return errUnchanged
	}
	// a new certificate that isn't valid now would replace one that may still be
	loaded, err := r.load(true)
	if err != nil {
		return err
	}
	r.use(loaded)
	return nil
}

// tlsConfig returns the TLS config of the api port: every new connection gets the current certificates
func (r *certReloader) tlsConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		ClientAuth: tls.RequireAndVerifyClientCert,
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			return r.current.Load().config, nil
		},
	}
}

// certificates returns the certificates the api serves with now
func (r *certReloader) certificates() []*x509.Certificate {
	return r.current.Load().certs
}

// reloadAndLog reloads the files, and logs the result; reason says what started it
func (r *certReloader) reloadAndLog(ctx context.Context, reason string) {
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	err := r.reload()
	switch {
	case errors.Is(err, errUnchanged):
		if reason == reloadOnSignal {
			logger.Info().Msg("SIGHUP: the certificate files didn't change")
		}
	case err != nil:
		logger.Warn().AnErr(keyError, err).Str("reason", reason).
			Msg("the certificate files changed, but can't be used: keeping the current certificates")
	default:
		logger.Info().Str("reason", reason).Msg("loaded the new certificate files")
		warnExpiring(ctx, r.certificates())
	}
}

const (
	reloadOnSignal = "SIGHUP"
	reloadOnTimer  = "periodic check"
)

// notifyHUP returns a channel that gets SIGHUP, and a function that stops that
func notifyHUP() (<-chan os.Signal, func()) {
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	return hup, func() { signal.Stop(hup) }
}

// watch reloads the files when hup fires (SIGHUP) and every period, and warns about expiring certificates every
// expiryCheckPeriod, until ctx is done
func (r *certReloader) watch(ctx context.Context, period time.Duration, hup <-chan os.Signal) {
	reloadTicker := time.NewTicker(period)
	defer reloadTicker.Stop()
	expiryTicker := time.NewTicker(expiryCheckPeriod)
	defer expiryTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-hup:
			r.reloadAndLog(ctx, reloadOnSignal)
		case <-reloadTicker.C:
			r.reloadAndLog(ctx, reloadOnTimer)
		case <-expiryTicker.C:
			warnExpiring(ctx, r.certificates())
		}
	}
}
