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
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/pgvillage-tools/orion/internal/auth"
	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
)

// renewalTimeout is how long the private key of a certificate request waits for its certificate
const renewalTimeout = 10 * time.Minute

// pendingRenewal is the private key of the last certificate request of the online renewal. It only lives in memory:
// after a restart, the cli requests a new one.
type pendingRenewal struct {
	mu      sync.Mutex
	key     crypto.Signer
	created time.Time
}

func (p *pendingRenewal) set(key crypto.Signer, now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.key, p.created = key, now
}

// get returns the pending key, unless it is older than renewalTimeout; it stays pending until clear
func (p *pendingRenewal) get(now time.Time) crypto.Signer {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.key == nil || now.Sub(p.created) > renewalTimeout {
		return nil
	}
	return p.key
}

func (p *pendingRenewal) clear() {
	p.set(nil, time.Time{})
}

// APICertificateRoutes returns the routes of the online renewal of the api certificate
func (h *Handlers) APICertificateRoutes() []Route {
	return []Route{
		{endpoints.APICertificateRequestEndPoint.Route(endpoints.MethodPost), h.APICertificateRequestHandler},
		{endpoints.APICertificateEndPoint.Route(endpoints.MethodPut), h.APICertificateHandler},
	}
}

// onlyCLI answers 403 and returns false unless the client is the cli
func onlyCLI(w http.ResponseWriter, r *http.Request) bool {
	identity, ok := auth.FromContext(r.Context())
	if !ok || identity.Name != pki.CLIClientName {
		endpoints.WriteProblem(w, http.StatusForbidden, "only "+pki.CLIClientName+" can renew the api certificate")
		return false
	}
	return true
}

// decodeBody decodes the JSON body of r into v; an empty body leaves v as it is
func decodeBody(r *http.Request, v any) error {
	data, err := io.ReadAll(io.LimitReader(r.Body, readMaxBytes))
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}

// APICertificateRequestHandler creates a new private key for the api, keeps it in memory, and returns a certificate
// request for it
func (h *Handlers) APICertificateRequestHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	if !onlyCLI(w, r) {
		return
	}
	if h.certs == nil {
		endpoints.WriteProblem(w, http.StatusConflict, "the api has no certificates to renew")
		return
	}
	var req endpoints.APICertificateRequestRequest
	if err := decodeBody(r, &req); err != nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	algorithm := oriontls.KeyAlgorithm(req.KeyAlgorithm)
	if algorithm == "" {
		algorithm = h.certs.keyAlgorithm()
	}
	if !slices.Contains(oriontls.KeyAlgorithms(), algorithm) {
		endpoints.WriteProblem(w, http.StatusBadRequest, fmt.Sprintf("unknown key algorithm %q", algorithm))
		return
	}
	key := oriontls.PrivateKey{Algorithm: algorithm}
	if err := key.Generate(); err != nil {
		handleError(ctx, err, w, "failed to create a private key")
		return
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: pki.APIServerName},
	}, key.Signer())
	if err != nil {
		handleError(ctx, err, w, "failed to create the certificate request")
		return
	}
	h.renewal.set(key.Signer(), time.Now())
	logger.Info().Str("algorithm", string(algorithm)).Msg("created a key and a certificate request for a renewal")
	h.writeJSON(ctx, w, endpoints.APICertificateRequestResponse{
		Request: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}),
	})
}

// errRenewal is a problem with the certificate of a renewal, answered with 400
var errRenewal = errors.New("invalid certificate")

// checkRenewal checks that chain (PEM) is a valid certificate for key, signed by intermediate (the one the api serves
// with now), and returns it
func checkRenewal(chainPEM []byte, key crypto.Signer, intermediate *x509.Certificate, now time.Time) (
	*x509.Certificate, error) {
	var certs []*x509.Certificate
	for rest := chainPEM; ; {
		var block *pem.Block
		if block, rest = pem.Decode(rest); block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errRenewal, err)
		}
		certs = append(certs, c)
	}
	if len(certs) != 2 {
		return nil, fmt.Errorf("%w: expected the certificate and its intermediate, got %d certificates", errRenewal,
			len(certs))
	}
	leaf := certs[0]
	type publicKey interface{ Equal(crypto.PublicKey) bool }
	if pub, ok := key.Public().(publicKey); !ok || !pub.Equal(leaf.PublicKey) {
		return nil, fmt.Errorf("%w: it isn't for the key of the last certificate request", errRenewal)
	}
	if !certs[1].Equal(intermediate) {
		return nil, fmt.Errorf("%w: it isn't signed by the intermediate the api has (%s): renewing the "+
			"intermediate needs the offline renewal", errRenewal, intermediate.Subject.CommonName)
	}
	if err := leaf.CheckSignatureFrom(intermediate); err != nil {
		return nil, fmt.Errorf("%w: %v", errRenewal, err)
	}
	if leaf.Subject.CommonName != pki.APIServerName || !slices.Contains(leaf.ExtKeyUsage, x509.ExtKeyUsageServerAuth) {
		return nil, fmt.Errorf("%w: it isn't a server certificate for %s", errRenewal, pki.APIServerName)
	}
	if now.After(leaf.NotAfter) || now.Before(leaf.NotBefore) {
		return nil, fmt.Errorf("%w: it is valid from %s to %s only", errRenewal, leaf.NotBefore.Format(time.RFC3339),
			leaf.NotAfter.Format(time.RFC3339))
	}
	return leaf, nil
}

// APICertificateHandler takes the signed certificate for the last certificate request, stores it with its key, and
// serves new connections with it
func (h *Handlers) APICertificateHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	if !onlyCLI(w, r) {
		return
	}
	if h.certs == nil {
		endpoints.WriteProblem(w, http.StatusConflict, "the api has no certificates to renew")
		return
	}
	var req endpoints.APICertificate
	if err := decodeBody(r, &req); err != nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	now := time.Now()
	key := h.renewal.get(now)
	if key == nil {
		endpoints.WriteProblem(w, http.StatusConflict, "there is no certificate request (anymore): request one first")
		return
	}
	intermediate := h.certs.intermediate()
	if intermediate == nil {
		endpoints.WriteProblem(w, http.StatusConflict, "the certificate of the api has no intermediate")
		return
	}
	leaf, err := checkRenewal(req.Chain, key, intermediate, now)
	if err != nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		handleError(ctx, err, w, "failed to encode the private key")
		return
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err = h.certs.replace(req.Chain, keyPEM); err != nil {
		handleError(ctx, err, w, "failed to store the new certificate: use the offline renewal")
		return
	}
	h.renewal.clear()
	serial := hex.EncodeToString(leaf.SerialNumber.Bytes())
	logger.Info().Str("serial", serial).Time("notAfter", leaf.NotAfter).Msg("renewed the api certificate")
	h.writeJSON(ctx, w, endpoints.APICertificateResponse{Serial: serial, NotAfter: leaf.NotAfter})
}
