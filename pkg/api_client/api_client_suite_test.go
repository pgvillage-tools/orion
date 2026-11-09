package api_client_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	client "github.com/pgvillage-tools/orion/pkg/api_client"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

func TestAPIClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "API Client Suite")
}

// serve starts an http server that answers a GET on ep with status and body, and returns a connection to it. The
// server stops when the spec ends.
func serve(ep endpoints.EndPoint, status int, body string) client.Connection {
	mux := http.NewServeMux()
	mux.HandleFunc(ep.Route(endpoints.MethodGet), func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	DeferCleanup(srv.Close)
	return connectTo(srv.URL)
}

// connectTo returns a connection to the http server at rawURL
func connectTo(rawURL string) client.Connection {
	u, err := url.Parse(rawURL)
	Ω(err).NotTo(HaveOccurred())
	port, err := strconv.ParseUint(u.Port(), 10, 16)
	Ω(err).NotTo(HaveOccurred())
	return client.NewConnection(endpoints.HTTP, u.Hostname(), uint16(port), time.Second)
}

// received is a request that a test server received
type received struct {
	method, path, contentType, ifMatch, body string
}

// record starts an http server that answers every request with handler, and returns a connection to it, and the
// requests it received. The server stops when the spec ends.
func record(handler http.HandlerFunc) (client.Connection, *[]received) {
	var requests []received
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests = append(requests, received{r.Method, r.URL.Path, r.Header.Get("Content-Type"),
			r.Header.Get("If-Match"), string(body)})
		handler(w, r)
	}))
	DeferCleanup(srv.Close)
	return connectTo(srv.URL), &requests
}

// respond returns a handler that answers with status and body (JSON), and with etag as the ETag (when not empty)
func respond(status int, etag, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if etag != "" {
			w.Header().Set("ETag", etag)
		}
		if body != "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// refused returns a connection to a port where nothing listens (anymore)
func refused() client.Connection {
	srv := httptest.NewServer(http.NotFoundHandler())
	c := connectTo(srv.URL)
	srv.Close()
	return c
}
