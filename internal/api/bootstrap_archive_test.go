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

package api

import (
	"bytes"
	"crypto/hpke"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/auth"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/pgvillage-tools/orion/pkg/bootstrap"
	"github.com/prometheus/client_golang/prometheus"
)

var _ = Describe("detectedSANs", func() {
	It("has the hostname, the addresses of the host and the host of --base-url", func() {
		host, err := os.Hostname()
		Expect(err).NotTo(HaveOccurred())
		sans := detectedSANs("orion-api.example.org")
		Expect(sans).To(ContainElements("127.0.0.1", "orion-api.example.org"))
		if !strings.Contains(host, "_") {
			Expect(sans).To(ContainElement(host))
		}
	})

	It("leaves out a --base-url that listens on all addresses", func() {
		Expect(detectedSANs("0.0.0.0")).NotTo(ContainElement("0.0.0.0"))
		Expect(detectedSANs("::")).NotTo(ContainElement("::"))
	})
})

var _ = Describe("the bootstrap archive endpoints", func() {
	var (
		h      *Handlers
		dir    string
		server *httptest.Server
		priv   hpke.PrivateKey
	)

	BeforeEach(func(ctx SpecContext) {
		dir = filepath.Join(GinkgoT().TempDir(), "api")
		h = NewHandlers(nil, nil, nil)
		h.bootstrap = newBootstrapper([]byte(testSecret), time.Hour)
		h.bootstrap.dir, h.bootstrap.sans = dir, []string{"detected.example.org"}
		srv, err := bootstrapServer(ctx, h, newAPIMetrics(prometheus.NewRegistry()))
		Expect(err).NotTo(HaveOccurred())
		server = httptest.NewUnstartedServer(srv.Handler)
		server.TLS = srv.TLSConfig
		server.StartTLS()
		DeferCleanup(server.Close)
		priv, err = bootstrap.GenerateKey()
		Expect(err).NotTo(HaveOccurred())
	})

	// newClient returns a client that uses one TLS connection, and can't verify the self-signed certificate
	newClient := func() *http.Client {
		return &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // the bootstrap certificate
			MaxConnsPerHost: 1,
		}}
	}

	// post posts body (when not nil) as JSON to ep, decodes the response into out (when it is 200), and returns the
	// response and the keying material of its connection
	post := func(client *http.Client, ep endpoints.EndPoint, body, out any) (*http.Response, []byte) {
		GinkgoHelper()
		var reader io.Reader
		if body != nil {
			data, err := json.Marshal(body)
			Expect(err).NotTo(HaveOccurred())
			reader = bytes.NewReader(data)
		}
		resp, err := client.Post(server.URL+"/"+endpoints.APIVersion+"/"+string(ep), "application/json", reader)
		Expect(err).NotTo(HaveOccurred())
		respBody, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Body.Close()).To(Succeed())
		if resp.StatusCode == http.StatusOK && out != nil {
			Expect(json.Unmarshal(respBody, out)).To(Succeed())
		}
		ekm, err := bootstrap.ExportKeyingMaterial(resp.TLS)
		Expect(err).NotTo(HaveOccurred())
		return resp, ekm
	}

	// proven returns a client whose connection passed the proofs
	proven := func() *http.Client {
		GinkgoHelper()
		client := newClient()
		var c endpoints.BootstrapChallengeResponse
		_, ekm := post(client, endpoints.BootstrapChallengeEndPoint, nil, &c)
		cliNonce := bytes.Repeat([]byte{3}, bootstrap.NonceSize)
		proof, err := bootstrap.CLIProof([]byte(testSecret), c.Nonce, cliNonce, ekm)
		Expect(err).NotTo(HaveOccurred())
		resp, _ := post(client, endpoints.BootstrapProofEndPoint,
			endpoints.BootstrapProofRequest{Nonce: cliNonce, Proof: proof}, nil)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		return client
	}

	archiveRequest := func(sans ...string) endpoints.BootstrapArchiveRequest {
		return endpoints.BootstrapArchiveRequest{PublicKey: priv.PublicKey().Bytes(), SANs: sans}
	}

	It("hand out the archive on a proven connection, encrypted to the cli and bound to the connection", func() {
		client := proven()
		var archive endpoints.BootstrapArchive
		resp, ekm := post(client, endpoints.BootstrapArchiveEndPoint, archiveRequest("lb.example.org"), &archive)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		keys, err := bootstrap.OpenArchive(priv, ekm, archive)
		Expect(err).NotTo(HaveOccurred())
		Expect(pki.VerifyArchive(archive.Certificates, keys, time.Now())).To(Succeed())
		block, _ := pem.Decode(archive.Certificates.APIServer)
		cert, err := x509.ParseCertificate(block.Bytes)
		Expect(err).NotTo(HaveOccurred())
		Expect(cert.DNSNames).To(ContainElements("localhost", "detected.example.org", "lb.example.org"))
		Expect(filepath.Join(dir, pki.APIServerCertFile)).NotTo(BeAnExistingFile(), "nothing stored yet")

		// a second archive has a new CA
		var second endpoints.BootstrapArchive
		resp, _ = post(client, endpoints.BootstrapArchiveEndPoint, archiveRequest(), &second)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(second.Certificates.Root).NotTo(Equal(archive.Certificates.Root))
	})

	It("store the api files and the archive once the cli stored it, and use the secret up", func() {
		client := proven()
		var archive endpoints.BootstrapArchive
		post(client, endpoints.BootstrapArchiveEndPoint, archiveRequest(), &archive)
		var status endpoints.BootstrapStatusResponse
		resp, _ := post(client, endpoints.BootstrapStoredEndPoint, nil, &status)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(status.Status).To(Equal(endpoints.BootstrapStatusStored))
		Expect(h.bootstrapDone).To(BeClosed(), "the api switches to normal mode")

		files := pki.APIFiles(dir)
		Expect(files.Check()).To(Succeed())
		serverCert, err := os.ReadFile(files.CertFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(serverCert)).To(Equal(string(archive.Certificates.APIServer) +
			string(archive.Certificates.ServerIntermediate)))
		Expect(os.ReadFile(files.CAFile)).To(Equal(archive.Certificates.APIClientIntermediate))

		archiveFile := filepath.Join(dir, pki.APIBootstrapArchiveFile)
		info, err := os.Stat(archiveFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
		data, err := os.ReadFile(archiveFile)
		Expect(err).NotTo(HaveOccurred())
		var stored endpoints.BootstrapArchive
		Expect(json.Unmarshal(data, &stored)).To(Succeed())
		Expect(stored).To(Equal(archive), "the api keeps the archive it can't read, not the CA keys")
		Expect(string(data)).NotTo(ContainSubstring("PRIVATE KEY"))

		// the secret is used up
		resp, _ = post(newClient(), endpoints.BootstrapChallengeEndPoint, nil, nil)
		Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
		resp, _ = post(client, endpoints.BootstrapStoredEndPoint, nil, nil)
		Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
	})

	It("keep the secret when the api files can't be stored", func() {
		Expect(os.MkdirAll(filepath.Dir(dir), 0o700)).To(Succeed())
		Expect(os.WriteFile(dir, []byte("not a directory"), 0o600)).To(Succeed())
		client := proven()
		post(client, endpoints.BootstrapArchiveEndPoint, archiveRequest(), nil)
		resp, _ := post(client, endpoints.BootstrapStoredEndPoint, nil, nil)
		Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
		Expect(h.bootstrapDone).NotTo(BeClosed())
		resp, _ = post(newClient(), endpoints.BootstrapChallengeEndPoint, nil, nil)
		Expect(resp.StatusCode).To(Equal(http.StatusOK), "the secret isn't used up")
	})

	It("refuse an archive and stored without a proof on the connection", func() {
		proven()
		for _, ep := range []endpoints.EndPoint{endpoints.BootstrapArchiveEndPoint, endpoints.BootstrapStoredEndPoint} {
			resp, _ := post(newClient(), ep, archiveRequest(), nil)
			Expect(resp.StatusCode).To(Equal(http.StatusConflict), string(ep))
		}
	})

	It("refuse stored without an archive on the connection", func() {
		resp, _ := post(proven(), endpoints.BootstrapStoredEndPoint, nil, nil)
		Expect(resp.StatusCode).To(Equal(http.StatusConflict))
	})

	DescribeTable("refuse an invalid archive request",
		func(body func() any) {
			resp, _ := post(proven(), endpoints.BootstrapArchiveEndPoint, body(), nil)
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		},
		Entry("no JSON object", func() any { return "not an object" }),
		Entry("no public key", func() any { return endpoints.BootstrapArchiveRequest{} }),
		Entry("an invalid name", func() any { return archiveRequest("*.example.org") }),
		Entry("too many names", func() any {
			sans := make([]string, maxExtraSANs+1)
			for i := range sans {
				sans[i] = "host" + strconv.Itoa(i)
			}
			return archiveRequest(sans...)
		}),
	)

	It("require TLS", func() {
		for _, handler := range []http.HandlerFunc{h.BootstrapArchiveHandler, h.BootstrapStoredHandler} {
			w := httptest.NewRecorder()
			handler(w, httptest.NewRequest(http.MethodPost, "/", nil))
			Expect(w.Code).To(Equal(http.StatusBadRequest))
		}
	})
})

var _ = Describe("BootstrapConfirmHandler", func() {
	var (
		h           *Handlers
		archiveFile string
	)

	BeforeEach(func() {
		h = NewHandlers(nil, nil, nil)
		archiveFile = filepath.Join(GinkgoT().TempDir(), pki.APIBootstrapArchiveFile)
		h.archiveFile = archiveFile
	})

	// confirm posts a confirmation as identity (none when nil), and returns the response
	confirm := func(identity *auth.Identity) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		if identity != nil {
			r = r.WithContext(auth.WithIdentity(r.Context(), identity))
		}
		w := httptest.NewRecorder()
		h.BootstrapConfirmHandler(w, r)
		return w
	}

	It("removes the archive, also when repeated", func() {
		Expect(os.WriteFile(archiveFile, []byte("{}"), 0o600)).To(Succeed())
		for range 2 {
			w := confirm(&auth.Identity{Name: pki.CLIClientName})
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Body.String()).To(ContainSubstring(endpoints.BootstrapStatusConfirmed))
			Expect(archiveFile).NotTo(BeAnExistingFile())
		}
	})

	It("only takes the confirmation of the cli", func() {
		Expect(os.WriteFile(archiveFile, []byte("{}"), 0o600)).To(Succeed())
		Expect(confirm(nil).Code).To(Equal(http.StatusForbidden))
		Expect(confirm(&auth.Identity{Name: "someone else"}).Code).To(Equal(http.StatusForbidden))
		Expect(archiveFile).To(BeAnExistingFile())
	})

	It("fails when the archive can't be removed", func() {
		Expect(os.MkdirAll(filepath.Join(archiveFile, "something"), 0o700)).To(Succeed())
		Expect(confirm(&auth.Identity{Name: pki.CLIClientName}).Code).To(Equal(http.StatusInternalServerError))
	})

	It("is in the routes of normal mode", func() {
		var paths []string
		for _, r := range h.Routes() {
			paths = append(paths, r.Pattern)
		}
		Expect(paths).To(ContainElement(endpoints.BootstrapConfirmEndPoint.Route(endpoints.MethodPost)))
	})
})
