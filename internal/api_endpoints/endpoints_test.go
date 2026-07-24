package api_endpoints_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
)

var _ = Describe("Endpoints", func() {
	When("Getting Route from Endpoint", func() {
		It("should work as expected", func() {
			for _, t := range []struct {
				ep       endpoints.EndPoint
				method   endpoints.Method
				vars     []string
				expected string
			}{
				{ep: endpoints.EndPoint("/a/b/c/d/"), method: endpoints.MethodPost, expected: "POST /v1/a/b/c/d"},
				{ep: endpoints.EndPoint("a/b/c/d"), method: endpoints.MethodPost, expected: "POST /v1/a/b/c/d"},
				{ep: endpoints.EndPoint("a/b/c/d"), method: endpoints.MethodGet, expected: "GET /v1/a/b/c/d"},
				{ep: endpoints.EndPoint("a/b/c/d"), method: endpoints.MethodPut, vars: []string{"id", "num"},
					expected: "PUT /v1/a/b/c/d/{id}/{num}"},
			} {
				Ω(t.ep.Route(t.method, t.vars...)).To(Equal(t.expected))
			}
		})
	})
	When("Getting URL from Endpoint", func() {
		It("should work as expected", func() {
			for _, t := range []struct {
				ep       endpoints.EndPoint
				proto    endpoints.Protocol
				ip       string
				port     uint16
				expected string
			}{
				{ep: endpoints.EndPoint("/a/b/c/d/"), proto: endpoints.HTTPS, port: 8080, ip: "1.2.3.4",
					expected: "https://1.2.3.4:8080/v1/a/b/c/d"},
				{ep: endpoints.EndPoint("/a/b/c/d/"), proto: endpoints.HTTP, port: 8081, ip: "1.2.3.5",
					expected: "http://1.2.3.5:8081/v1/a/b/c/d"},
			} {
				Ω(t.ep.URL(t.proto, t.ip, t.port)).To(Equal(t.expected))
			}
		})
	})
})
