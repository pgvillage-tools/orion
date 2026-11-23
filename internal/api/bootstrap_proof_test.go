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
	"context"
	"crypto/tls"
	"encoding/json"
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
	"github.com/pgvillage-tools/orion/internal/logging"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/pgvillage-tools/orion/pkg/bootstrap"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

const testSecret = "a bootstrap secret of twenty-plus characters"

// statusOf returns the http status of err, 0 for no error
func statusOf(err *bootstrapError) int {
	if err == nil {
		return 0
	}
	return err.status
}

var _ = Describe("bootstrapSecret", func() {
	// logged returns a context with a logger that writes to out, at the level of the api component
	logged := func(ctx context.Context, out *bytes.Buffer) context.Context {
		_, componentLogger := logging.GetLogComponent(ctx, logging.WebApiComponent)
		logger := zerolog.New(out).Level(componentLogger.GetLevel())
		return logger.WithContext(ctx)
	}

	It("takes the secret from the environment variable", func(ctx SpecContext) {
		Expect(bootstrapSecret(ctx, Config{BootstrapSecret: testSecret})).To(Equal([]byte(testSecret)))
	})

	It("takes the secret from a file, without the newline", func(ctx SpecContext) {
		file := filepath.Join(GinkgoT().TempDir(), "secret")
		Expect(os.WriteFile(file, []byte(testSecret+"\n"), 0o600)).To(Succeed())
		Expect(bootstrapSecret(ctx, Config{BootstrapSecretFile: file})).To(Equal([]byte(testSecret)))
	})

	It("generates a secret of 128 bits, and logs it once", func(ctx SpecContext) {
		var out bytes.Buffer
		secret, err := bootstrapSecret(logged(ctx, &out), Config{})
		Expect(err).NotTo(HaveOccurred())
		Expect(secret).To(HaveLen(26))
		raw, err := generatedSecretEncoding.DecodeString(string(secret))
		Expect(err).NotTo(HaveOccurred())
		Expect(raw).To(HaveLen(generatedSecretBytes))
		Expect(strings.Count(out.String(), string(secret))).To(Equal(1), "the secret is logged once")

		other, err := bootstrapSecret(ctx, Config{})
		Expect(err).NotTo(HaveOccurred())
		Expect(other).NotTo(Equal(secret))
	})

	DescribeTable("refuses",
		func(ctx SpecContext, cfg func(dir string) Config, want string) {
			_, err := bootstrapSecret(ctx, cfg(GinkgoT().TempDir()))
			Expect(err).To(MatchError(ContainSubstring(want)))
		},
		Entry("a short secret", func(string) Config { return Config{BootstrapSecret: "too short"} },
			"at least 20 characters"),
		Entry("a secret in the environment and in a file", func(dir string) Config {
			return Config{BootstrapSecret: testSecret, BootstrapSecretFile: filepath.Join(dir, "secret")}
		}, "not both"),
		Entry("a missing file", func(dir string) Config {
			return Config{BootstrapSecretFile: filepath.Join(dir, "missing")}
		}, "failed to read the bootstrap secret"),
		Entry("an empty file", func(dir string) Config {
			file := filepath.Join(dir, "empty")
			Expect(os.WriteFile(file, []byte("\n"), 0o600)).To(Succeed())
			return Config{BootstrapSecretFile: file}
		}, "is empty"),
	)
})

var _ = Describe("newBootstrapperFromConfig", func() {
	It("uses the default ttl", func(ctx SpecContext) {
		b, err := newBootstrapperFromConfig(ctx, Config{BootstrapSecret: testSecret})
		Expect(err).NotTo(HaveOccurred())
		Expect(b.expires).To(BeTemporally("~", time.Now().Add(DefaultBootstrapSecretTTL), time.Second))
	})

	It("refuses a negative ttl", func(ctx SpecContext) {
		_, err := newBootstrapperFromConfig(ctx, Config{BootstrapSecret: testSecret, BootstrapSecretTTL: -1})
		Expect(err).To(MatchError(errBootstrapTTL))
	})

	It("returns the error of the secret", func(ctx SpecContext) {
		_, err := newBootstrapperFromConfig(ctx, Config{BootstrapSecret: "short"})
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("bootstrapper", func() {
	var (
		b   *bootstrapper
		now time.Time
		ekm []byte
	)

	BeforeEach(func() {
		now = time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
		b = newBootstrapper([]byte(testSecret), time.Hour)
		b.now = func() time.Time { return now }
		b.expires = now.Add(time.Hour)
		ekm = bytes.Repeat([]byte{7}, bootstrap.ExporterLength)
	})

	// nonce returns a nonce filled with n
	nonce := func(n byte) []byte { return bytes.Repeat([]byte{n}, bootstrap.NonceSize) }

	// prove proves the secret for the challenge with apiNonce
	prove := func(secret string, apiNonce []byte) ([]byte, *bootstrapError) {
		proof, err := bootstrap.CLIProof([]byte(secret), apiNonce, nonce(2), ekm)
		Expect(err).NotTo(HaveOccurred())
		return b.prove(ekm, nonce(2), proof)
	}

	It("returns the proof of the api for a right proof", func() {
		Expect(b.challenge(string(ekm), nonce(1))).To(BeNil())
		apiProof, err := prove(testSecret, nonce(1))
		Expect(err).To(BeNil())
		want, _ := bootstrap.APIProof([]byte(testSecret), nonce(1), nonce(2), ekm)
		Expect(apiProof).To(Equal(want))
		Expect(b.sessions[string(ekm)].proven).To(BeTrue())
	})

	It("refuses a wrong proof, and the challenge gets one try", func() {
		Expect(b.challenge(string(ekm), nonce(1))).To(BeNil())
		_, err := prove("another secret of twenty characters", nonce(1))
		Expect(statusOf(err)).To(Equal(http.StatusForbidden))
		_, err = prove(testSecret, nonce(1))
		Expect(statusOf(err)).To(Equal(http.StatusConflict), "the challenge is gone")
	})

	It("refuses a proof without a challenge on the connection", func() {
		_, err := prove(testSecret, nonce(1))
		Expect(statusOf(err)).To(Equal(http.StatusConflict))
	})

	It("uses the latest challenge on a connection", func() {
		Expect(b.challenge(string(ekm), nonce(1))).To(BeNil())
		Expect(b.challenge(string(ekm), nonce(3))).To(BeNil())
		_, err := prove(testSecret, nonce(1))
		Expect(statusOf(err)).To(Equal(http.StatusForbidden))
		Expect(b.challenge(string(ekm), nonce(3))).To(BeNil())
		_, err = prove(testSecret, nonce(3))
		Expect(err).To(BeNil())
	})

	It("rate limits wrong proofs, for a minute", func() {
		for range maxFailedProofs {
			Expect(b.challenge(string(ekm), nonce(1))).To(BeNil())
			_, err := prove("another secret of twenty characters", nonce(1))
			Expect(statusOf(err)).To(Equal(http.StatusForbidden))
			now = now.Add(time.Second)
		}
		err := b.challenge(string(ekm), nonce(1))
		Expect(statusOf(err)).To(Equal(http.StatusTooManyRequests))
		Expect(err.retryAfter).To(Equal(failedProofWindow - maxFailedProofs*time.Second))

		now = now.Add(failedProofWindow)
		Expect(b.challenge(string(ekm), nonce(1))).To(BeNil(), "the failures are out of the window")
		_, err = prove(testSecret, nonce(1))
		Expect(err).To(BeNil())
	})

	It("refuses everything once the secret expired", func() {
		Expect(b.challenge(string(ekm), nonce(1))).To(BeNil())
		now = now.Add(time.Hour)
		_, err := prove(testSecret, nonce(1))
		Expect(statusOf(err)).To(Equal(http.StatusForbidden))
		Expect(err.detail).To(ContainSubstring("expired"))
		Expect(statusOf(b.challenge(string(ekm), nonce(1)))).To(Equal(http.StatusForbidden))
	})

	It("keeps a limited number of challenges, for a limited time", func() {
		for i := range maxBootstrapSessions {
			Expect(b.challenge(strconv.Itoa(i), nonce(1))).To(BeNil())
		}
		err := b.challenge("one more", nonce(1))
		Expect(statusOf(err)).To(Equal(http.StatusTooManyRequests))
		Expect(b.challenge("0", nonce(1))).To(BeNil(), "a new challenge on a known connection")

		now = now.Add(bootstrapSessionTTL + time.Second)
		Expect(b.challenge("one more", nonce(1))).To(BeNil(), "the old challenges are gone")
		Expect(b.sessions).To(HaveLen(1))
	})
})

var _ = Describe("the bootstrap endpoints", func() {
	var (
		h      *Handlers
		server *httptest.Server
	)

	BeforeEach(func(ctx SpecContext) {
		h = NewHandlers(nil, nil, nil)
		h.bootstrap = newBootstrapper([]byte(testSecret), time.Hour)
		srv, err := bootstrapServer(ctx, h, newAPIMetrics(prometheus.NewRegistry()))
		Expect(err).NotTo(HaveOccurred())
		server = httptest.NewUnstartedServer(srv.Handler)
		server.TLS = srv.TLSConfig
		server.StartTLS()
		DeferCleanup(server.Close)
	})

	// newClient returns a client that uses one TLS connection, and can't verify the self-signed certificate
	newClient := func() *http.Client {
		return &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // the bootstrap certificate
			MaxConnsPerHost: 1,
		}}
	}

	// post posts body as JSON to ep, and decodes the response into out (when it is 200)
	post := func(client *http.Client, ep endpoints.EndPoint, body, out any) *http.Response {
		GinkgoHelper()
		data, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		resp, err := client.Post(server.URL+"/"+endpoints.APIVersion+"/"+string(ep), "application/json",
			bytes.NewReader(data))
		Expect(err).NotTo(HaveOccurred())
		// read and close the body, so the next request can use the connection
		respBody, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Body.Close()).To(Succeed())
		if resp.StatusCode == http.StatusOK && out != nil {
			Expect(json.Unmarshal(respBody, out)).To(Succeed())
		}
		return resp
	}

	// challenge posts a challenge with client, and returns the nonce of the api and the keying material
	challenge := func(client *http.Client) ([]byte, []byte) {
		GinkgoHelper()
		var c endpoints.BootstrapChallengeResponse
		resp := post(client, endpoints.BootstrapChallengeEndPoint, nil, &c)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(c.Nonce).To(HaveLen(bootstrap.NonceSize))
		ekm, err := bootstrap.ExportKeyingMaterial(resp.TLS)
		Expect(err).NotTo(HaveOccurred())
		return c.Nonce, ekm
	}

	cliNonce := bytes.Repeat([]byte{2}, bootstrap.NonceSize)

	It("let the cli and the api prove that they have the secret, on one connection", func() {
		client := newClient()
		apiNonce, ekm := challenge(client)
		proof, err := bootstrap.CLIProof([]byte(testSecret), apiNonce, cliNonce, ekm)
		Expect(err).NotTo(HaveOccurred())

		var p endpoints.BootstrapProofResponse
		resp := post(client, endpoints.BootstrapProofEndPoint,
			endpoints.BootstrapProofRequest{Nonce: cliNonce, Proof: proof}, &p)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		want, err := bootstrap.APIProof([]byte(testSecret), apiNonce, cliNonce, ekm)
		Expect(err).NotTo(HaveOccurred())
		Expect(bootstrap.Equal(p.Proof, want)).To(BeTrue(), "the api proved that it has the secret")
	})

	It("refuse a proof on another connection", func() {
		apiNonce, ekm := challenge(newClient())
		proof, err := bootstrap.CLIProof([]byte(testSecret), apiNonce, cliNonce, ekm)
		Expect(err).NotTo(HaveOccurred())
		resp := post(newClient(), endpoints.BootstrapProofEndPoint,
			endpoints.BootstrapProofRequest{Nonce: cliNonce, Proof: proof}, nil)
		Expect(resp.StatusCode).To(Equal(http.StatusConflict))
	})

	It("refuse a wrong proof, and rate limit wrong proofs", func() {
		client := newClient()
		for range maxFailedProofs {
			apiNonce, ekm := challenge(client)
			proof, err := bootstrap.CLIProof([]byte("another secret of twenty characters"), apiNonce, cliNonce, ekm)
			Expect(err).NotTo(HaveOccurred())
			resp := post(client, endpoints.BootstrapProofEndPoint,
				endpoints.BootstrapProofRequest{Nonce: cliNonce, Proof: proof}, nil)
			Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
			Expect(resp.Header.Get("Content-Type")).To(Equal(endpoints.ContentTypeProblem))
		}
		resp := post(client, endpoints.BootstrapChallengeEndPoint, nil, nil)
		Expect(resp.StatusCode).To(Equal(http.StatusTooManyRequests))
		Expect(resp.Header.Get("Retry-After")).NotTo(BeEmpty())
	})

	DescribeTable("refuse an invalid proof request",
		func(body any) {
			client := newClient()
			challenge(client)
			resp := post(client, endpoints.BootstrapProofEndPoint, body, nil)
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		},
		Entry("no JSON object", "not an object"),
		Entry("a short nonce", endpoints.BootstrapProofRequest{Nonce: []byte{1}, Proof: make([]byte, 32)}),
		Entry("no proof", endpoints.BootstrapProofRequest{Nonce: make([]byte, 32)}),
	)

	It("serve nothing else", func() {
		resp, err := newClient().Get(server.URL + "/" + endpoints.APIVersion + "/" + string(endpoints.WhoAmIEndPoint))
		Expect(err).NotTo(HaveOccurred())
		_ = resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
	})

	It("require TLS", func() {
		for _, handler := range []http.HandlerFunc{h.BootstrapChallengeHandler, h.BootstrapProofHandler} {
			w := httptest.NewRecorder()
			handler(w, httptest.NewRequest(http.MethodPost, "/", nil))
			Expect(w.Code).To(Equal(http.StatusBadRequest))
		}
	})

	It("are only there in bootstrap mode", func() {
		Expect(NewHandlers(nil, nil, nil).BootstrapRoutes()).To(BeEmpty())
	})
})
