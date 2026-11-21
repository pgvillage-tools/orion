package auth_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/auth"
)

// staticAuthenticator is an authenticator that returns identity or err. It is the only declaration outside the
// Describes: Go doesn't allow methods on a type declared in a function.
type staticAuthenticator struct {
	identity *auth.Identity
	err      error
}

func (s staticAuthenticator) Authenticate(_ *http.Request) (*auth.Identity, error) {
	return s.identity, s.err
}

var _ = Describe("Middleware", func() {
	// serve serves r with the middleware and authenticators, and returns the response and the identity the handler
	// saw
	serve := func(r *http.Request, authenticators ...auth.Authenticator) (*httptest.ResponseRecorder, *auth.Identity) {
		var seen *auth.Identity
		handler := auth.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seen, _ = auth.FromContext(r.Context())
		}), authenticators...)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w, seen
	}

	// requestWithCert returns a request with a verified client certificate for cn, in the groups ou
	requestWithCert := func(cn string, ou ...string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		cert := &x509.Certificate{Subject: pkix.Name{CommonName: cn, OrganizationalUnit: ou}}
		r.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{cert},
			VerifiedChains:   [][]*x509.Certificate{{cert}},
		}
		return r
	}

	Context("with client certificates", func() {
		It("authenticates the common name, with the organizational units as groups", func() {
			w, identity := serve(requestWithCert("orion-cli", "admins"), auth.ClientCertAuthenticator{})
			Ω(w.Code).To(Equal(http.StatusOK))
			Ω(identity).To(Equal(&auth.Identity{Name: "orion-cli", Method: auth.MethodClientCert,
				Groups: []string{"admins"}}))
		})

		DescribeTable("answers 401 without an identity",
			func(r func() *http.Request) {
				w, identity := serve(r(), auth.ClientCertAuthenticator{})
				Ω(w.Code).To(Equal(http.StatusUnauthorized))
				Ω(identity).To(BeNil())
			},
			Entry("without TLS", func() *http.Request { return httptest.NewRequest(http.MethodGet, "/", nil) }),
			Entry("with an unverified certificate", func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{}}}
				return r
			}),
			Entry("without a common name", func() *http.Request { return requestWithCert("") }),
		)
	})

	Context("with a chain of authenticators", func() {
		next := &auth.Identity{Name: "token-user"}

		It("goes to the next authenticator when one doesn't apply", func() {
			w, identity := serve(httptest.NewRequest(http.MethodGet, "/", nil), auth.ClientCertAuthenticator{},
				staticAuthenticator{identity: next})
			Ω(w.Code).To(Equal(http.StatusOK))
			Ω(identity).To(BeIdenticalTo(next))
		})

		It("stops at an authenticator that fails", func() {
			w, identity := serve(httptest.NewRequest(http.MethodGet, "/", nil),
				staticAuthenticator{err: errors.New("bad token")}, staticAuthenticator{identity: next})
			Ω(w.Code).To(Equal(http.StatusUnauthorized))
			Ω(identity).To(BeNil())
		})
	})
})

var _ = Describe("FromContext", func() {
	It("has no identity in a plain context", func() {
		_, ok := auth.FromContext(context.Background())
		Ω(ok).To(BeFalse())
	})
})
