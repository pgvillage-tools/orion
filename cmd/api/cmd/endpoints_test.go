package cmd_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiCmd "github.com/pgvillage-tools/orion/cmd/api/cmd"
)

var _ = Describe("Endpoints", func() {
	When("Getting Route from Endpoint", func() {
		It("should work as expected", func() {
			for _, t := range []struct {
				ep       apiCmd.EndPoint
				method   apiCmd.Method
				vars     []string
				expected string
			}{
				{ep: apiCmd.EndPoint("/a/b/c/d/"), method: apiCmd.MethodPost, expected: "POST /a/b/c/d"},
				{ep: apiCmd.EndPoint("a/b/c/d"), method: apiCmd.MethodPost, expected: "POST /a/b/c/d"},
				{ep: apiCmd.EndPoint("a/b/c/d"), method: apiCmd.MethodGet, expected: "GET /a/b/c/d"},
				{ep: apiCmd.EndPoint("a/b/c/d"), method: apiCmd.MethodPut, vars: []string{"id", "num"},
					expected: "PUT /a/b/c/d/{id}/{num}"},
			} {
				Ω(t.ep.Route(t.method, t.vars...)).To(Equal(t.expected))
			}
		})
	})
	When("Getting URL from Endpoint", func() {
		It("should work as expected", func() {
			for _, t := range []struct {
				ep       apiCmd.EndPoint
				proto    apiCmd.Protocol
				ip       string
				port     uint16
				expected string
			}{
				{ep: apiCmd.EndPoint("/a/b/c/d/"), proto: apiCmd.HTTPS, port: 8080, ip: "1.2.3.4",
					expected: "https://1.2.3.4:8080/a/b/c/d"},
				{ep: apiCmd.EndPoint("/a/b/c/d/"), proto: apiCmd.HTTP, port: 8081, ip: "1.2.3.5",
					expected: "http://1.2.3.5:8081/a/b/c/d"},
			} {
				Ω(t.ep.URL(t.proto, t.ip, t.port)).To(Equal(t.expected))
			}
		})
	})
})
