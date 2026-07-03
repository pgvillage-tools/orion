package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiv1 "github.com/pgvillage-tools/orion/cmd/api/v1"
)

var _ = Describe("Endpoints", func() {
	When("Getting Route from Endpoint", func() {
		It("should work as expected", func() {
			for _, t := range []struct {
				ep       apiv1.EndPoint
				method   apiv1.Method
				vars     []string
				expected string
			}{
				{ep: apiv1.EndPoint("/a/b/c/d/"), method: apiv1.MethodPost, expected: "POST /v1/a/b/c/d"},
				{ep: apiv1.EndPoint("a/b/c/d"), method: apiv1.MethodPost, expected: "POST /v1/a/b/c/d"},
				{ep: apiv1.EndPoint("a/b/c/d"), method: apiv1.MethodGet, expected: "GET /v1/a/b/c/d"},
				{ep: apiv1.EndPoint("a/b/c/d"), method: apiv1.MethodPut, vars: []string{"id", "num"},
					expected: "PUT /v1/a/b/c/d/{id}/{num}"},
			} {
				Ω(t.ep.Route(t.method, t.vars...)).To(Equal(t.expected))
			}
		})
	})
	When("Getting URL from Endpoint", func() {
		It("should work as expected", func() {
			for _, t := range []struct {
				ep       apiv1.EndPoint
				proto    apiv1.Protocol
				ip       string
				port     uint16
				expected string
			}{
				{ep: apiv1.EndPoint("/a/b/c/d/"), proto: apiv1.HTTPS, port: 8080, ip: "1.2.3.4",
					expected: "https://1.2.3.4:8080/v1/a/b/c/d"},
				{ep: apiv1.EndPoint("/a/b/c/d/"), proto: apiv1.HTTP, port: 8081, ip: "1.2.3.5",
					expected: "http://1.2.3.5:8081/v1/a/b/c/d"},
			} {
				Ω(t.ep.URL(t.proto, t.ip, t.port)).To(Equal(t.expected))
			}
		})
	})
})
