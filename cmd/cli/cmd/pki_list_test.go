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

package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/pki"
)

var _ = Describe("pki list", func() {
	// newPKI runs pki init with a leaf validity in new directories, makes the cli use them, and returns a fake api
	// with the api certificate of it
	newPKI := func(leafValidity string) *fakeAPI {
		cliDir := GinkgoT().TempDir()
		apiDir := filepath.Join(GinkgoT().TempDir(), "api")
		usePKIDir(cliDir)
		_, _, err := run("pki", "init", "--api-dir", apiDir, "--leaf-validity", leafValidity)
		Ω(err).NotTo(HaveOccurred())
		tlsConfig, err := pki.APIFiles(apiDir).ServerConfig()
		Ω(err).NotTo(HaveOccurred())
		return newFakeAPIWithTLS(tlsConfig)
	}
	// rowsOf returns the rows of the table in out (without the header) that start with source
	rowsOf := func(out, source string) []string {
		var rows []string
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, source+"\t") {
				rows = append(rows, line)
			}
		}
		return rows
	}

	It("lists each certificate of the cli once, without connecting with --local", func() {
		api := newFakeAPI()
		out, _, err := api.run("pki", "list", "--local")
		Ω(err).NotTo(HaveOccurred())
		rows := rowsOf(out, sourceCLI)
		// ca.crt holds the root too: it is listed once, with ca/root.crt
		Ω(rows).To(HaveLen(4))
		Ω(rows[0]).To(And(ContainSubstring(pki.CLICertFile), ContainSubstring(pki.CLIClientName),
			ContainSubstring(statusValid)))
		Ω(out).To(And(ContainSubstring(pki.APIClientIntermediate), ContainSubstring(pki.ServerIntermediate),
			ContainSubstring("ca/root.crt")))
		Ω(rowsOf(out, sourceAPI)).To(BeEmpty())
		Ω(out).NotTo(ContainSubstring("trusted"))
	})

	It("lists the certificates of the api, and says the cli trusts them", func() {
		api := newFakeAPI()
		out, _, err := api.run("pki", "list")
		Ω(err).NotTo(HaveOccurred())
		rows := rowsOf(out, sourceAPI)
		Ω(rows).To(HaveLen(2), "expected the api certificate and the server intermediate")
		Ω(rows[0]).To(ContainSubstring(pki.APIServerName))
		Ω(rows[1]).To(ContainSubstring(pki.ServerIntermediate))
		Ω(out).To(ContainSubstring("The certificate of the api at 127.0.0.1:" + api.port() +
			" is trusted by the root of the cli"))
		Ω(api.requests()).To(BeEmpty(), "expected only a handshake")
	})

	It("says when the api has a certificate of another CA", func() {
		api := newPKI("1y")
		usePKIDir(filepath.Join(testHome, ".orion", "cli"))
		out, _, err := api.run("pki", "list")
		Ω(err).NotTo(HaveOccurred())
		Ω(rowsOf(out, sourceAPI)).To(HaveLen(2))
		Ω(out).To(ContainSubstring("is NOT trusted by the root of the cli: x509: certificate signed by unknown authority"))
	})

	It("lists expired certificates, also of the api", func() {
		const validity = 2 * time.Second
		api := newPKI(validity.String())
		time.Sleep(validity)
		out, _, err := api.run("pki", "list")
		Ω(err).NotTo(HaveOccurred())
		Ω(rowsOf(out, sourceCLI)[0]).To(HaveSuffix(statusExpired))
		Ω(rowsOf(out, sourceAPI)[0]).To(HaveSuffix(statusExpired))
		Ω(out).To(And(ContainSubstring("is NOT trusted by the root of the cli"), ContainSubstring("expired")))
	})

	It("marks a certificate that expires within 30 days", func() {
		api := newPKI("10d")
		out, _, err := api.run("pki", "list")
		Ω(err).NotTo(HaveOccurred())
		Ω(rowsOf(out, sourceCLI)[0]).To(ContainSubstring(statusExpiring + " (9d left)"))
		Ω(rowsOf(out, sourceAPI)[0]).To(ContainSubstring(statusExpiring))
	})

	It("lists the certificates of the cli and fails when the api is down", func() {
		api := newFakeAPI()
		api.server.Close()
		out, _, err := api.run("pki", "list")
		expectError(err, "failed to get the certificates of the api at 127.0.0.1:"+api.port(), "connection refused")
		Ω(rowsOf(out, sourceCLI)).To(HaveLen(4))
	})

	It("prints json", func() {
		api := newFakeAPI()
		out, _, err := api.run("pki", "list", "--format", "json")
		Ω(err).NotTo(HaveOccurred())
		var result pkiList
		Ω(json.Unmarshal([]byte(out), &result)).To(Succeed())
		Ω(result.Certificates).To(HaveLen(6))
		Ω(result.Certificates[0].CommonName).To(Equal(pki.CLIClientName))
		Ω(result.Certificates[0].File).To(Equal(pki.CLICertFile))
		Ω(result.Certificates[4].Source).To(Equal(sourceAPI))
		Ω(result.API).NotTo(BeNil())
		Ω(result.API.Trusted).To(BeTrue())
		Ω(result.API.Error).To(BeEmpty())
	})

	It("refuses an unknown format", func() {
		_, _, err := run("pki", "list", "--local", "--format", "yaml")
		expectError(err, "unrecognised output format yaml", "")
	})

	It("fails without certificates", func() {
		dir := GinkgoT().TempDir()
		usePKIDir(dir)
		_, _, err := run("pki", "list", "--local")
		expectError(err, "no certificates found in "+dir, "`orion-cli bootstrap`")
	})
})

var _ = Describe("certStatus", func() {
	now := time.Now()
	DescribeTable("returns the status at now",
		func(notBefore, notAfter time.Time, expected string) {
			Ω(certStatus(testCert("c", notBefore, notAfter), now)).To(Equal(expected))
		},
		Entry("valid", now.Add(-time.Hour), now.Add(pki.ExpiryWarning+time.Hour), statusValid),
		Entry("expires soon", now.Add(-time.Hour), now.Add(pki.ExpiryWarning-time.Hour), statusExpiring),
		Entry("expired", now.Add(-2*time.Hour), now.Add(-time.Hour), statusExpired),
		Entry("not valid yet", now.Add(time.Hour), now.Add(2*time.Hour), statusNotYet),
	)
})
