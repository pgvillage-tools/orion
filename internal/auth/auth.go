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

// Package auth authenticates requests to the api. It is a chain of authenticators that all return the same
// Identity, so other methods (e.g. OIDC) can be added without changing the handlers.
package auth

import (
	"context"
	"errors"
	"net/http"
)

// MethodClientCert is the method of an identity that was authenticated with a client certificate
const MethodClientCert = "client-certificate"

// Identity describes who sent a request
type Identity struct {
	// Name is the name of the client (e.g. the CommonName of its certificate)
	Name string
	// Groups are the groups the client is a member of (e.g. the OrganizationalUnits of its certificate)
	Groups []string
	// Method is how the client was authenticated
	Method string
}

// ErrNotApplicable is returned by an Authenticator when the request has no credentials for it, so the next
// Authenticator can try
var ErrNotApplicable = errors.New("authentication method not applicable")

// Authenticator authenticates a request. It returns ErrNotApplicable when the request has no credentials for it.
type Authenticator interface {
	Authenticate(r *http.Request) (*Identity, error)
}

// ClientCertAuthenticator authenticates requests with the verified client certificate of the TLS connection
type ClientCertAuthenticator struct{}

// Authenticate returns the identity of the verified client certificate
func (ClientCertAuthenticator) Authenticate(r *http.Request) (*Identity, error) {
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.VerifiedChains[0]) == 0 {
		return nil, ErrNotApplicable
	}
	cert := r.TLS.VerifiedChains[0][0]
	if cert.Subject.CommonName == "" {
		return nil, errors.New("client certificate has no CommonName")
	}
	return &Identity{
		Name:   cert.Subject.CommonName,
		Groups: cert.Subject.OrganizationalUnit,
		Method: MethodClientCert,
	}, nil
}

type contextKey struct{}

// WithIdentity returns a copy of ctx with the identity
func WithIdentity(ctx context.Context, identity *Identity) context.Context {
	return context.WithValue(ctx, contextKey{}, identity)
}

// FromContext returns the identity in ctx
func FromContext(ctx context.Context) (*Identity, bool) {
	identity, ok := ctx.Value(contextKey{}).(*Identity)
	return identity, ok && identity != nil
}

// Middleware authenticates every request with the first authenticator that applies, and puts the identity in the
// request context. Requests that no authenticator accepts get 401 Unauthorized.
func Middleware(next http.Handler, authenticators ...Authenticator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, a := range authenticators {
			identity, err := a.Authenticate(r)
			if errors.Is(err, ErrNotApplicable) {
				continue
			}
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), identity)))
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}
