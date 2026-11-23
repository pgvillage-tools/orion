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
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pgvillage-tools/orion/internal/logging"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/pgvillage-tools/orion/pkg/bootstrap"
)

// The bootstrap secret and the proofs: see docs/api/bootstrap-design.md, "The bootstrap protocol"

const (
	// BootstrapSecretEnv is the environment variable with the bootstrap secret. There is no flag for the secret
	// itself: arguments are visible in ps.
	BootstrapSecretEnv = "ORIONAPI_BOOTSTRAP_SECRET"
	// DefaultBootstrapSecretTTL is the default time the bootstrap secret is valid, from the start of bootstrap mode
	DefaultBootstrapSecretTTL = 24 * time.Hour
	// MinBootstrapSecretLength is the minimum length of a bootstrap secret you choose: with an active man in the
	// middle, a weak secret can be guessed offline
	MinBootstrapSecretLength = 20

	// generatedSecretBytes is the size of a generated secret (128 bits; 26 base32 characters)
	generatedSecretBytes = 16
	// maxFailedProofs is the number of wrong proofs the api accepts per failedProofWindow, for all clients together;
	// after that it answers 429 until the oldest failure is out of the window
	maxFailedProofs   = 5
	failedProofWindow = time.Minute
	// maxBootstrapSessions is the number of challenges without a proof the api keeps; bootstrapSessionTTL is the time
	// it keeps one
	maxBootstrapSessions = 16
	bootstrapSessionTTL  = 5 * time.Minute
	// maxBootstrapBody is the maximum size of the body of a bootstrap request
	maxBootstrapBody = 4 << 10
)

// generatedSecretEncoding writes a generated secret in upper case letters and digits, without padding
var generatedSecretEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// bootstrapSecret returns the bootstrap secret: from cfg (the environment variable, or the file), or a new one, which
// it logs once, since the cli needs it
func bootstrapSecret(ctx context.Context, cfg Config) ([]byte, error) {
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	secret := cfg.BootstrapSecret
	if cfg.BootstrapSecretFile != "" {
		if secret != "" {
			return nil, fmt.Errorf("set the bootstrap secret in %s or with --bootstrap-secret-file, not both",
				BootstrapSecretEnv)
		}
		data, err := os.ReadFile(cfg.BootstrapSecretFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read the bootstrap secret: %w", err)
		}
		// a file usually ends with a newline
		secret = strings.TrimSpace(string(data))
		if secret == "" {
			return nil, fmt.Errorf("the bootstrap secret file %s is empty", cfg.BootstrapSecretFile)
		}
	}
	if secret != "" {
		if len(secret) < MinBootstrapSecretLength {
			return nil, fmt.Errorf("the bootstrap secret must have at least %d characters", MinBootstrapSecretLength)
		}
		return []byte(secret), nil
	}
	raw := make([]byte, generatedSecretBytes)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	secret = generatedSecretEncoding.EncodeToString(raw)
	logger.Warn().Str("secret", secret).
		Msg("generated a bootstrap secret: bootstrap the api with it (it can be used once), or set your own in " +
			BootstrapSecretEnv)
	return []byte(secret), nil
}

// bootstrapper holds the state of the bootstrap: the secret, the challenges per TLS connection, and the failed proofs
type bootstrapper struct {
	secret  []byte
	expires time.Time
	now     func() time.Time

	mutex sync.Mutex
	// sessions are the challenges, by the keying material of their TLS connection
	sessions map[string]*bootstrapSession
	// failures are the times of the wrong proofs within failedProofWindow
	failures []time.Time
}

// bootstrapSession is a challenge of the api on one TLS connection
type bootstrapSession struct {
	apiNonce []byte
	created  time.Time
	// proven is set when the cli proved it knows the secret on this connection
	proven bool
}

// newBootstrapper returns a bootstrapper with secret, which expires after ttl
func newBootstrapper(secret []byte, ttl time.Duration) *bootstrapper {
	b := &bootstrapper{secret: secret, now: time.Now, sessions: map[string]*bootstrapSession{}}
	b.expires = b.now().Add(ttl)
	return b
}

// bootstrapError is an error of a bootstrap request, with its http status
type bootstrapError struct {
	status     int
	detail     string
	retryAfter time.Duration
}

// prune removes the sessions and failures that are too old; the caller holds the mutex
func (b *bootstrapper) prune(now time.Time) {
	for key, s := range b.sessions {
		if !s.proven && now.Sub(s.created) > bootstrapSessionTTL {
			delete(b.sessions, key)
		}
	}
	for len(b.failures) > 0 && now.Sub(b.failures[0]) >= failedProofWindow {
		b.failures = b.failures[1:]
	}
}

// check returns an error when the secret expired, or there were too many wrong proofs; the caller holds the mutex
func (b *bootstrapper) check(now time.Time) *bootstrapError {
	if !now.Before(b.expires) {
		return &bootstrapError{status: http.StatusForbidden,
			detail: "the bootstrap secret expired: restart the api for a new one"}
	}
	if len(b.failures) >= maxFailedProofs {
		return &bootstrapError{status: http.StatusTooManyRequests, detail: "too many wrong bootstrap proofs",
			retryAfter: failedProofWindow - now.Sub(b.failures[0])}
	}
	return nil
}

// challenge starts a session on the TLS connection with keying material key, with apiNonce
func (b *bootstrapper) challenge(key string, apiNonce []byte) *bootstrapError {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	now := b.now()
	b.prune(now)
	if err := b.check(now); err != nil {
		return err
	}
	if _, ok := b.sessions[key]; !ok && len(b.sessions) >= maxBootstrapSessions {
		return &bootstrapError{status: http.StatusTooManyRequests, detail: "too many bootstrap sessions",
			retryAfter: bootstrapSessionTTL}
	}
	// a new challenge on the same connection replaces the old one
	b.sessions[key] = &bootstrapSession{apiNonce: apiNonce, created: now}
	return nil
}

// prove checks the proof of the cli on the TLS connection with keying material ekm, and returns the proof of the api
func (b *bootstrapper) prove(ekm []byte, cliNonce, cliProof []byte) ([]byte, *bootstrapError) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	now := b.now()
	b.prune(now)
	if err := b.check(now); err != nil {
		return nil, err
	}
	key := string(ekm)
	session, ok := b.sessions[key]
	if !ok {
		return nil, &bootstrapError{status: http.StatusConflict,
			detail: "no bootstrap challenge on this connection: post to " +
				string(endpoints.BootstrapChallengeEndPoint) + " first, on the same TLS connection"}
	}
	expected, err := bootstrap.CLIProof(b.secret, session.apiNonce, cliNonce, ekm)
	if err != nil || !bootstrap.Equal(expected, cliProof) {
		// a challenge gets one try
		delete(b.sessions, key)
		b.failures = append(b.failures, now)
		return nil, &bootstrapError{status: http.StatusForbidden,
			detail: "wrong bootstrap proof: the cli doesn't have the bootstrap secret of the api"}
	}
	apiProof, err := bootstrap.APIProof(b.secret, session.apiNonce, cliNonce, ekm)
	if err != nil {
		return nil, &bootstrapError{status: http.StatusInternalServerError, detail: "failed to create the proof"}
	}
	session.proven = true
	return apiProof, nil
}

// writeBootstrapError writes err as a problem, with Retry-After for a 429
func writeBootstrapError(w http.ResponseWriter, err *bootstrapError) {
	if err.retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(err.retryAfter.Seconds()))))
	}
	endpoints.WriteProblem(w, err.status, err.detail)
}

// BootstrapRoutes returns the routes of the api port in bootstrap mode: the bootstrap endpoints, and nothing else
func (h *Handlers) BootstrapRoutes() []Route {
	if h.bootstrap == nil {
		return nil
	}
	return []Route{
		{endpoints.BootstrapChallengeEndPoint.Route(endpoints.MethodPost), h.BootstrapChallengeHandler},
		{endpoints.BootstrapProofEndPoint.Route(endpoints.MethodPost), h.BootstrapProofHandler},
	}
}

// BootstrapChallengeHandler starts the bootstrap on the TLS connection of r: it returns the nonce of the api
func (h *Handlers) BootstrapChallengeHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	ekm, err := bootstrap.ExportKeyingMaterial(r.TLS)
	if err != nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	nonce := make([]byte, bootstrap.NonceSize)
	if _, err = rand.Read(nonce); err != nil {
		handleError(ctx, err, w, "failed to create a nonce")
		return
	}
	if bErr := h.bootstrap.challenge(string(ekm), nonce); bErr != nil {
		logger.Warn().Str("client", r.RemoteAddr).Str("reason", bErr.detail).Msg("refused a bootstrap challenge")
		writeBootstrapError(w, bErr)
		return
	}
	logger.Info().Str("client", r.RemoteAddr).Msg("bootstrap challenge")
	h.writeJSON(ctx, w, endpoints.BootstrapChallengeResponse{Nonce: nonce})
}

// BootstrapProofHandler checks the proof of the cli on the TLS connection of r, and returns the proof of the api
func (h *Handlers) BootstrapProofHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	ekm, err := bootstrap.ExportKeyingMaterial(r.TLS)
	if err != nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var req endpoints.BootstrapProofRequest
	if err = json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBootstrapBody)).Decode(&req); err != nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, "invalid bootstrap proof request")
		return
	}
	if len(req.Nonce) != bootstrap.NonceSize || len(req.Proof) != bootstrap.ProofSize {
		endpoints.WriteProblem(w, http.StatusBadRequest, fmt.Sprintf(
			"the nonce and the proof must have %d and %d bytes", bootstrap.NonceSize, bootstrap.ProofSize))
		return
	}
	apiProof, bErr := h.bootstrap.prove(ekm, req.Nonce, req.Proof)
	if bErr != nil {
		logger.Warn().Str("client", r.RemoteAddr).Str("reason", bErr.detail).Msg("refused a bootstrap proof")
		writeBootstrapError(w, bErr)
		return
	}
	logger.Info().Str("client", r.RemoteAddr).Msg("the cli proved that it has the bootstrap secret")
	h.writeJSON(ctx, w, endpoints.BootstrapProofResponse{Proof: apiProof})
}

// errBootstrapTTL is returned for a bootstrap secret ttl that isn't positive
var errBootstrapTTL = errors.New("--bootstrap-secret-ttl must be positive")

// newBootstrapperFromConfig returns the bootstrapper with the secret and its ttl from cfg (DefaultBootstrapSecretTTL
// when it isn't set)
func newBootstrapperFromConfig(ctx context.Context, cfg Config) (*bootstrapper, error) {
	ttl := cfg.BootstrapSecretTTL
	if ttl == 0 {
		ttl = DefaultBootstrapSecretTTL
	}
	if ttl < 0 {
		return nil, errBootstrapTTL
	}
	secret, err := bootstrapSecret(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return newBootstrapper(secret, ttl), nil
}
