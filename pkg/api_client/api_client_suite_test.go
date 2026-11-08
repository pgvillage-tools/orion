package api_client_test

import (
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
	u, err := url.Parse(srv.URL)
	Ω(err).NotTo(HaveOccurred())
	port, err := strconv.ParseUint(u.Port(), 10, 16)
	Ω(err).NotTo(HaveOccurred())
	return client.NewConnection(endpoints.HTTP, u.Hostname(), uint16(port), time.Second)
}
