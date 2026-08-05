package tls

import (
	"fmt"
	"net"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cert", func() {
	const (
		stringEmail  = "user@example.org"
		stringDomain = "someserver1.subdomain.example.org"
		stringIPv41  = "1.2.3.4"
		stringIPv42  = "127.0.0.1"
		stringIPv61  = "::1"
		stringIPv62  = "2001:0db8:85a3:0000:0000:8a2e:0370:7334"
		stringIPv63  = "2001:db8::1:0:0:1"
		stringIPv64  = "::ffff:192.0.2.128"
		stringURI1   = "https://private.api.example.org"
		stringURI2   = "http://public.api.example.org:8080"

		// This becomes a uri
		//broken_stringEmail  = "user@@example.org"
		//broken_stringDomain = "someserver1.subdomain.example.org."
		//broken_uri_s    = "https:private.api.example.org"
		stringBrokenIPv4s = "1.2.3:4"
		stringBrokenIPv6s = stringIPv62 + ":"
	)
	var (
		emails         = []string{stringEmail}
		domains        = []string{stringDomain}
		stringListIPv4 = []string{stringIPv41, stringIPv42}
		ipv4_1         = net.ParseIP(stringIPv41)
		ipv4_2         = net.ParseIP(stringIPv42)
		stringListIPv6 = []string{stringIPv61, stringIPv62, stringIPv63, stringIPv64}
		ipv6_1         = net.ParseIP(stringIPv61)
		ipv6_2         = net.ParseIP(stringIPv62)
		ipv6_3         = net.ParseIP(stringIPv63)
		ipv6_4         = net.ParseIP(stringIPv64)
		stringListURIs = []string{stringURI1, stringURI2}
		uriMustCompile = func(s string) *url.URL {
			uri, err := url.Parse(s)
			if err != nil {
				panic(fmt.Sprintf("%s could not be compiled: %e", s, err))
			}
			return uri
		}
		uri1            = uriMustCompile(stringURI1)
		uri2            = uriMustCompile(stringURI2)
		workingAltNames = append(
			append(
				append(
					append(emails, domains...),
					stringListIPv4...,
				),
				stringListIPv6...,
			),
			stringListURIs...)
		invalidAltNames = []string{
			stringBrokenIPv4s,
			stringBrokenIPv6s,
		}
	)
	/*
		BeforeAll(func() {
		})
		BeforeEach(func() {
		})
		AfterEach(func() {
		})
	*/
	Context("When splitting Alternate Names ", Ordered, func() {
		It("Should work with valid values", func() {
			result, err := splitAlternateNames(workingAltNames)
			Expect(err).Error().NotTo(HaveOccurred())

			Expect(result.dnsNames).To(HaveLen(len(domains)))
			Expect(result.dnsNames).To(ContainElements(stringDomain))

			Expect(result.eMailAddresses).To(HaveLen(len(emails)))
			Expect(result.eMailAddresses).To(ContainElements(stringEmail))

			Expect(result.ipAddresses).To(HaveLen(len(stringListIPv4) + len(stringListIPv6)))
			Expect(result.ipAddresses).To(ContainElements(ipv4_1, ipv4_2, ipv6_1, ipv6_2, ipv6_3, ipv6_4))

			Expect(result.uris).To(HaveLen(len(stringListURIs)))
			Expect(result.uris).To(ContainElements(uri2))
			Expect(result.uris).To(ContainElements(uri1))
		})
		It("Should not work with invalid values", func() {
			for _, invalid := range invalidAltNames {
				Expect(fmt.Fprintf(GinkgoWriter, "DEBUG - Test: %v\n", invalid)).
					Error().NotTo(HaveOccurred())
				myAltNames := append(workingAltNames, invalid)
				_, err := splitAlternateNames(myAltNames)
				Expect(err).Error().To(HaveOccurred())
			}
		})
	})
})
