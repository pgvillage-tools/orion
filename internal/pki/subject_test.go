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

package pki_test

import (
	"crypto/x509"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/pgvillage-tools/orion/internal/pki"
	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
)

var _ = Describe("SubjectOptions", func() {
	DescribeTable("accepts valid subjects",
		func(s pki.SubjectOptions) {
			Expect(s.Check()).To(Succeed())
		},
		Entry("empty", pki.SubjectOptions{}),
		Entry("all fields", pki.SubjectOptions{
			Organisation: "Acme B.V.", Country: "NL", State: "Utrecht", Locality: "'s-Hertogenbosch",
		}),
		Entry("non ascii", pki.SubjectOptions{Locality: "Zürich"}),
		Entry("64 characters", pki.SubjectOptions{Organisation: strings.Repeat("é", 64)}),
	)

	DescribeTable("refuses invalid subjects",
		func(s pki.SubjectOptions, message string) {
			Expect(s.Check()).To(MatchError(ContainSubstring(message)))
		},
		Entry("lower case country", pki.SubjectOptions{Country: "nl"}, "invalid country"),
		Entry("long country", pki.SubjectOptions{Country: "NLD"}, "invalid country"),
		Entry("short country", pki.SubjectOptions{Country: "N"}, "invalid country"),
		Entry("non letter country", pki.SubjectOptions{Country: "N1"}, "invalid country"),
		Entry("long organisation", pki.SubjectOptions{Organisation: strings.Repeat("a", 65)},
			"invalid organisation"),
		Entry("control character", pki.SubjectOptions{State: "a\nb"}, "invalid state"),
		Entry("invalid utf-8", pki.SubjectOptions{Locality: "\xff"}, "invalid locality"),
	)

	It("keeps the default organisation, and sets the other fields", func() {
		Expect(pki.SubjectOptions{}.Subject()).To(Equal(oriontls.DefaultSubject))
		Expect(pki.SubjectOptions{Organisation: "Acme", Country: "NL", State: "UT", Locality: "Utrecht"}.Subject()).
			To(Equal(oriontls.Subject{
				CommonName:   oriontls.DefaultSubject.CommonName,
				Organisation: "Acme",
				Country:      "NL",
				State:        "UT",
				Locality:     "Utrecht",
			}))
	})

	It("puts the subject on all certificates, with their own common name and without an OU", func() {
		dir := GinkgoT().TempDir()
		subject := pki.SubjectOptions{Organisation: "Acme", Country: "NL", State: "Utrecht", Locality: "Utrecht"}
		chain, err := pki.Init(pki.InitOptions{
			CLIDir:  filepath.Join(dir, "cli"),
			APIDir:  filepath.Join(dir, "api"),
			Subject: subject,
		})
		Expect(err).NotTo(HaveOccurred())
		certs := []*x509.Certificate{chain.Root.Cert.X509()}
		for _, im := range chain.Intermediates {
			certs = append(certs, im.Cert.Cert.X509())
			for _, child := range im.Children() {
				certs = append(certs, child.Cert.X509())
			}
		}
		Expect(certs).To(HaveLen(5))
		names := map[string]bool{}
		for _, c := range certs {
			Expect(c.Subject.Organization).To(Equal([]string{"Acme"}), c.Subject.CommonName)
			Expect(c.Subject.Country).To(Equal([]string{"NL"}), c.Subject.CommonName)
			Expect(c.Subject.Province).To(Equal([]string{"Utrecht"}), c.Subject.CommonName)
			Expect(c.Subject.Locality).To(Equal([]string{"Utrecht"}), c.Subject.CommonName)
			Expect(c.Subject.OrganizationalUnit).To(BeEmpty(), c.Subject.CommonName)
			names[c.Subject.CommonName] = true
		}
		Expect(names).To(HaveKey(pki.CLIClientName))
		Expect(names).To(HaveKey(pki.APIServerName))
		Expect(names).To(HaveLen(5), "expected every certificate to have its own common name")
	})

	It("refuses an invalid subject in Init and Generate, before it writes anything", func() {
		dir := GinkgoT().TempDir()
		opts := pki.InitOptions{
			CLIDir:  filepath.Join(dir, "cli"),
			APIDir:  filepath.Join(dir, "api"),
			Subject: pki.SubjectOptions{Country: "Netherlands"},
		}
		_, err := pki.Init(opts)
		Expect(err).To(MatchError(ContainSubstring("invalid country")))
		for _, f := range opts.Files() {
			Expect(f).NotTo(BeAnExistingFile())
		}
		_, err = pki.Generate(opts)
		Expect(err).To(MatchError(ContainSubstring("invalid country")))
	})

	It("puts the subject on the certificates of Generate", func() {
		chain, err := pki.Generate(pki.InitOptions{Subject: pki.SubjectOptions{Country: "BE"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(chain.Root.Cert.X509().Subject.Country).To(Equal([]string{"BE"}))
	})
})
