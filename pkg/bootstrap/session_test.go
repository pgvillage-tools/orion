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

package bootstrap_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/pgvillage-tools/orion/pkg/bootstrap"
)

// fakeAPI plays the api in bootstrap mode, with apiSecret; it closes its connections after each response when
// closeConns is set
type fakeAPI struct {
	apiSecret  []byte
	closeConns bool
	// proofBody, when set, is the body of the proof response
	proofBody string

	mutex    sync.Mutex
	apiNonce []byte
	server   *httptest.Server
}

func newFakeAPI(apiSecret []byte) *fakeAPI {
	f := &fakeAPI{apiSecret: apiSecret}
	mux := http.NewServeMux()
	mux.HandleFunc(endpoints.BootstrapChallengeEndPoint.Route(endpoints.MethodPost), f.challenge)
	mux.HandleFunc(endpoints.BootstrapProofEndPoint.Route(endpoints.MethodPost), f.proof)
	f.server = httptest.NewUnstartedServer(mux)
	f.server.StartTLS()
	DeferCleanup(f.server.Close)
	return f
}

func (f *fakeAPI) challenge(w http.ResponseWriter, _ *http.Request) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.apiNonce = bytes.Repeat([]byte{1}, bootstrap.NonceSize)
	if f.closeConns {
		w.Header().Set("Connection", "close")
	}
	_ = json.NewEncoder(w).Encode(endpoints.BootstrapChallengeResponse{Nonce: f.apiNonce})
}

func (f *fakeAPI) proof(w http.ResponseWriter, r *http.Request) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if f.proofBody != "" {
		_, _ = w.Write([]byte(f.proofBody))
		return
	}
	var req endpoints.BootstrapProofRequest
	Expect(json.NewDecoder(r.Body).Decode(&req)).To(Succeed())
	ekm, err := bootstrap.ExportKeyingMaterial(r.TLS)
	Expect(err).NotTo(HaveOccurred())
	// the api checks the proof of the cli with its own secret
	want, err := bootstrap.CLIProof(f.apiSecret, f.apiNonce, req.Nonce, ekm)
	Expect(err).NotTo(HaveOccurred())
	if !bootstrap.Equal(want, req.Proof) {
		endpoints.WriteProblem(w, http.StatusForbidden, "wrong bootstrap proof")
		return
	}
	apiProof, err := bootstrap.APIProof(f.apiSecret, f.apiNonce, req.Nonce, ekm)
	Expect(err).NotTo(HaveOccurred())
	_ = json.NewEncoder(w).Encode(endpoints.BootstrapProofResponse{Proof: apiProof})
}

var _ = Describe("Start", func() {
	var ctx context.Context

	BeforeEach(func() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		DeferCleanup(cancel)
	})

	start := func(f *fakeAPI, cliSecret []byte) error {
		_, err := bootstrap.Start(ctx, bootstrap.NewHTTPClient(5*time.Second), f.server.URL+"/", cliSecret)
		return err
	}

	It("succeeds when both have the secret", func() {
		Expect(start(newFakeAPI(secret), secret)).To(Succeed())
	})

	It("fails when the api refuses the proof of the cli", func() {
		var problem *bootstrap.ProblemError
		err := start(newFakeAPI(secret), []byte("another secret of twenty characters"))
		Expect(err).To(BeAssignableToTypeOf(problem))
		Expect(err).To(MatchError(ContainSubstring("403 Forbidden: wrong bootstrap proof")))
	})

	It("fails when the api doesn't prove the secret", func() {
		f := newFakeAPI(secret)
		f.proofBody = `{"proof":"` + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=" + `"}`
		Expect(start(f, secret)).To(MatchError(bootstrap.ErrAPIProof))
	})

	It("fails on a response that isn't JSON", func() {
		f := newFakeAPI(secret)
		f.proofBody = "not json"
		Expect(start(f, secret)).To(MatchError(ContainSubstring("invalid response")))
	})

	It("fails when the connection changes", func() {
		f := newFakeAPI(secret)
		f.closeConns = true
		Expect(start(f, secret)).To(MatchError(ContainSubstring("connection to the api changed")))
	})

	It("fails without an api", func() {
		f := newFakeAPI(secret)
		f.server.Close()
		Expect(start(f, secret)).NotTo(Succeed())
	})
})
