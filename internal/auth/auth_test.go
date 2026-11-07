package auth_test

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pgvillage-tools/orion/internal/auth"
)

type staticAuthenticator struct {
	identity *auth.Identity
	err      error
}

func (s staticAuthenticator) Authenticate(_ *http.Request) (*auth.Identity, error) {
	return s.identity, s.err
}

func serve(r *http.Request, authenticators ...auth.Authenticator) (*httptest.ResponseRecorder, *auth.Identity) {
	var seen *auth.Identity
	handler := auth.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen, _ = auth.FromContext(r.Context())
	}), authenticators...)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w, seen
}

func requestWithCert(cn string, ou ...string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	cert := &x509.Certificate{Subject: pkix.Name{CommonName: cn, OrganizationalUnit: ou}}
	r.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{cert},
		VerifiedChains:   [][]*x509.Certificate{{cert}},
	}
	return r
}

func TestClientCert(t *testing.T) {
	w, identity := serve(requestWithCert("orion-cli", "admins"), auth.ClientCertAuthenticator{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if identity == nil || identity.Name != "orion-cli" || identity.Method != auth.MethodClientCert {
		t.Fatalf("unexpected identity %+v", identity)
	}
	if len(identity.Groups) != 1 || identity.Groups[0] != "admins" {
		t.Fatalf("unexpected groups %v", identity.Groups)
	}
}

func TestUnauthenticated(t *testing.T) {
	unverified := httptest.NewRequest(http.MethodGet, "/", nil)
	unverified.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{}}}
	for name, r := range map[string]*http.Request{
		"no tls":         httptest.NewRequest(http.MethodGet, "/", nil),
		"unverified":     unverified,
		"no common name": requestWithCert(""),
	} {
		w, identity := serve(r, auth.ClientCertAuthenticator{})
		if w.Code != http.StatusUnauthorized || identity != nil {
			t.Errorf("%s: expected 401 without identity, got %d with %+v", name, w.Code, identity)
		}
	}
}

func TestChain(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	next := &auth.Identity{Name: "token-user"}
	w, identity := serve(r, auth.ClientCertAuthenticator{}, staticAuthenticator{identity: next})
	if w.Code != http.StatusOK || identity != next {
		t.Fatalf("expected the second authenticator to apply, got %d with %+v", w.Code, identity)
	}
	w, identity = serve(r, staticAuthenticator{err: errors.New("bad token")}, staticAuthenticator{identity: next})
	if w.Code != http.StatusUnauthorized || identity != nil {
		t.Fatalf("expected a failing authenticator to stop the chain, got %d with %+v", w.Code, identity)
	}
	if _, ok := auth.FromContext(r.Context()); ok {
		t.Fatal("expected no identity in a plain context")
	}
}
