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
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/pgvillage-tools/orion/pkg/bootstrap"
	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
)

const testBootstrapSecret = "the bootstrap secret of the api"

// portOf returns the port of the listener of server
func portOf(server *httptest.Server) string {
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	Ω(err).NotTo(HaveOccurred())
	return port
}

// fakeBootstrapAPI is a fake api for the bootstrap: a probe port with /status, and an api port with the bootstrap
// endpoints for testBootstrapSecret, which switches to mTLS with the certificates of the archive once the cli stored
// it
type fakeBootstrapAPI struct {
	// flags are the flags for orion-cli bootstrap to reach it; apiFlags leave out the probe port
	flags, apiFlags []string
	// sans are the extra names the cli sent for the api certificate
	sans []string
	// normal is the TLS config of normal mode, once the cli stored the archive
	normal atomic.Pointer[tls.Config]
	// confirmed is set when the cli confirmed the bootstrap with its new certificate
	confirmed atomic.Bool
}

// bootstrapAPI starts a fake api in mode, and returns it
func bootstrapAPI(mode string) *fakeBootstrapAPI {
	f := &fakeBootstrapAPI{}
	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Ω(r.URL.Path).To(Equal("/" + string(endpoints.ModeEndPoint)))
		m := mode
		if f.normal.Load() != nil {
			m = endpoints.ModeNormal
		}
		_ = json.NewEncoder(w).Encode(endpoints.ModeResponse{Mode: m})
	}))
	DeferCleanup(probe.Close)

	var (
		apiNonce []byte
		chain    *oriontls.Chain
	)
	mux := http.NewServeMux()
	mux.HandleFunc(endpoints.BootstrapChallengeEndPoint.Route(endpoints.MethodPost),
		func(w http.ResponseWriter, _ *http.Request) {
			apiNonce = make([]byte, bootstrap.NonceSize)
			_ = json.NewEncoder(w).Encode(endpoints.BootstrapChallengeResponse{Nonce: apiNonce})
		})
	mux.HandleFunc(endpoints.BootstrapProofEndPoint.Route(endpoints.MethodPost),
		func(w http.ResponseWriter, r *http.Request) {
			var req endpoints.BootstrapProofRequest
			Ω(json.NewDecoder(r.Body).Decode(&req)).To(Succeed())
			ekm, err := bootstrap.ExportKeyingMaterial(r.TLS)
			Ω(err).NotTo(HaveOccurred())
			want, err := bootstrap.CLIProof([]byte(testBootstrapSecret), apiNonce, req.Nonce, ekm)
			Ω(err).NotTo(HaveOccurred())
			if !bootstrap.Equal(want, req.Proof) {
				endpoints.WriteProblem(w, http.StatusForbidden, "wrong bootstrap proof")
				return
			}
			proof, err := bootstrap.APIProof([]byte(testBootstrapSecret), apiNonce, req.Nonce, ekm)
			Ω(err).NotTo(HaveOccurred())
			_ = json.NewEncoder(w).Encode(endpoints.BootstrapProofResponse{Proof: proof})
		})
	mux.HandleFunc(endpoints.BootstrapArchiveEndPoint.Route(endpoints.MethodPost),
		func(w http.ResponseWriter, r *http.Request) {
			var req endpoints.BootstrapArchiveRequest
			Ω(json.NewDecoder(r.Body).Decode(&req)).To(Succeed())
			f.sans = req.SANs
			pub, err := bootstrap.KEM().NewPublicKey(req.PublicKey)
			Ω(err).NotTo(HaveOccurred())
			chain, err = pki.Generate(pki.InitOptions{APISANs: req.SANs})
			Ω(err).NotTo(HaveOccurred())
			certs, keys := pki.ArchiveContents(*chain)
			ekm, err := bootstrap.ExportKeyingMaterial(r.TLS)
			Ω(err).NotTo(HaveOccurred())
			archive, err := bootstrap.SealArchive(pub, ekm, certs, keys)
			Ω(err).NotTo(HaveOccurred())
			_ = json.NewEncoder(w).Encode(archive)
		})
	mux.HandleFunc(endpoints.BootstrapStoredEndPoint.Route(endpoints.MethodPost),
		func(w http.ResponseWriter, _ *http.Request) {
			apiDir := GinkgoT().TempDir()
			Ω(pki.WriteAPIFiles(*chain, apiDir)).To(Succeed())
			config, err := pki.APIFiles(apiDir).ServerConfig()
			Ω(err).NotTo(HaveOccurred())
			f.normal.Store(config)
			_ = json.NewEncoder(w).Encode(endpoints.BootstrapStatusResponse{Status: endpoints.BootstrapStatusStored})
		})
	mux.HandleFunc(endpoints.BootstrapConfirmEndPoint.Route(endpoints.MethodPost),
		func(w http.ResponseWriter, r *http.Request) {
			Ω(r.TLS.PeerCertificates).NotTo(BeEmpty(), "the confirmation is over mTLS")
			Ω(r.TLS.PeerCertificates[0].Subject.CommonName).To(Equal(pki.CLIClientName))
			f.confirmed.Store(true)
			_ = json.NewEncoder(w).Encode(endpoints.BootstrapStatusResponse{
				Status: endpoints.BootstrapStatusConfirmed})
		})
	api := httptest.NewUnstartedServer(mux)
	// the self-signed certificate of httptest in bootstrap mode, the certificates of the archive in normal mode
	api.TLS = &tls.Config{GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
		return f.normal.Load(), nil
	}}
	api.StartTLS()
	DeferCleanup(api.Close)
	f.apiFlags = []string{"--host", "127.0.0.1", "--port", portOf(api), "--timeout", "5s"}
	f.flags = append([]string{"--probe-port", portOf(probe)}, f.apiFlags...)
	return f
}

// bootstrapSecretEnv sets ORIONCLI_BOOTSTRAP_SECRET to value for the spec ("" unsets it)
func bootstrapSecretEnv(value string) {
	if value == "" {
		Ω(os.Unsetenv(envBootstrapSecret)).To(Succeed())
	} else {
		Ω(os.Setenv(envBootstrapSecret, value)).To(Succeed())
	}
	DeferCleanup(os.Unsetenv, envBootstrapSecret)
}

var _ = Describe("bootstrapSecret", func() {
	It("reads the secret from a file, without the newline", func() {
		file := filepath.Join(GinkgoT().TempDir(), "secret")
		Ω(os.WriteFile(file, []byte(testBootstrapSecret+"\n"), 0o600)).To(Succeed())
		Ω(bootstrapSecret(file)).To(Equal([]byte(testBootstrapSecret)))
	})

	It("reads the secret from the environment", func() {
		bootstrapSecretEnv(testBootstrapSecret)
		Ω(bootstrapSecret("")).To(Equal([]byte(testBootstrapSecret)))
	})

	It("prompts on a terminal", func() {
		bootstrapSecretEnv("")
		fakeTerminal(testBootstrapSecret)
		Ω(bootstrapSecret("")).To(Equal([]byte(testBootstrapSecret)))
	})

	It("fails without a secret", func() {
		bootstrapSecretEnv("")
		_, err := bootstrapSecret("")
		Ω(err).To(MatchError(ContainSubstring("the bootstrap secret is required")))
	})

	It("fails on a missing file", func() {
		_, err := bootstrapSecret(filepath.Join(GinkgoT().TempDir(), "missing"))
		Ω(err).To(HaveOccurred())
	})

	It("refuses a secret that is too short to be the one of the api", func() {
		bootstrapSecretEnv("short")
		_, err := bootstrapSecret("")
		Ω(err).To(MatchError(ContainSubstring("at least 20 characters")))
	})

	It("fails when the terminal can't be read", func() {
		bootstrapSecretEnv("")
		fakeTerminal()
		_, err := bootstrapSecret("")
		Ω(err).To(HaveOccurred())
	})
})

var _ = Describe("bootstrap", func() {
	var dir string

	BeforeEach(func() {
		dir = filepath.Join(GinkgoT().TempDir(), "cli")
		usePKIDir(dir)
	})

	It("creates the key pair, gets and stores the certificates, and confirms with them", func() {
		bootstrapSecretEnv(testBootstrapSecret)
		api := bootstrapAPI(endpoints.ModeBootstrap)
		out, _, err := run(append([]string{"bootstrap", "--api-san", "lb.example.org"}, api.flags...)...)
		Ω(err).NotTo(HaveOccurred())
		Ω(out).To(ContainSubstring("Created the bootstrap key pair in " + dir))
		Ω(out).To(ContainSubstring("The api proved that it has the bootstrap secret"))
		Ω(out).To(ContainSubstring("Stored the certificates and the CA keys in " + dir))
		Ω(out).To(ContainSubstring("the bootstrap is done"))
		Ω(api.sans).To(Equal([]string{"lb.example.org"}))
		Ω(api.confirmed.Load()).To(BeTrue())
		_, err = bootstrap.ReadPrivateKey(dir, []byte(testCAPassphrase))
		Ω(err).NotTo(HaveOccurred(), "the private key is encrypted with the passphrase of the CA keys")
		root := oriontls.PrivateKey{Path: filepath.Join(dir, pki.CLICADir, "root.key"),
			Passphrase: []byte(testCAPassphrase)}
		Ω(root.Load()).To(Succeed(), "the CA keys are encrypted with the passphrase")

		// the confirmation can be repeated
		api.confirmed.Store(false)
		out, _, err = run(append([]string{"bootstrap", "confirm"}, api.apiFlags...)...)
		Ω(err).NotTo(HaveOccurred())
		Ω(out).To(ContainSubstring("the bootstrap is done"))
		Ω(api.confirmed.Load()).To(BeTrue())
	})

	It("uses the existing key pair", func() {
		bootstrapSecretEnv(testBootstrapSecret)
		priv, err := bootstrap.GenerateKey()
		Ω(err).NotTo(HaveOccurred())
		Ω(bootstrap.WriteKeyPair(dir, priv, []byte(testCAPassphrase))).To(Succeed())
		pub, err := os.ReadFile(filepath.Join(dir, bootstrap.PublicKeyFile))
		Ω(err).NotTo(HaveOccurred())
		out, _, err := run(append([]string{"bootstrap"}, bootstrapAPI(endpoints.ModeBootstrap).flags...)...)
		Ω(err).NotTo(HaveOccurred())
		Ω(out).NotTo(ContainSubstring("Created the bootstrap key pair"))
		Ω(os.ReadFile(filepath.Join(dir, bootstrap.PublicKeyFile))).To(Equal(pub))
	})

	It("fails with another passphrase than the one of the existing key pair", func() {
		bootstrapSecretEnv(testBootstrapSecret)
		priv, err := bootstrap.GenerateKey()
		Ω(err).NotTo(HaveOccurred())
		Ω(bootstrap.WriteKeyPair(dir, priv, []byte("another passphrase"))).To(Succeed())
		_, _, err = run(append([]string{"bootstrap"}, bootstrapAPI(endpoints.ModeBootstrap).flags...)...)
		Ω(err).To(MatchError(bootstrap.ErrWrongPassphrase))
	})

	It("refuses to overwrite the files of the cli, before it asks for anything", func() {
		bootstrapSecretEnv("")
		Ω(os.MkdirAll(dir, 0o700)).To(Succeed())
		Ω(os.WriteFile(filepath.Join(dir, pki.CLICertFile), []byte("mine"), 0o600)).To(Succeed())
		_, _, err := run(append([]string{"bootstrap"}, bootstrapAPI(endpoints.ModeBootstrap).flags...)...)
		Ω(err).To(MatchError(ContainSubstring("refusing to overwrite")))
		Ω(filepath.Join(dir, bootstrap.PublicKeyFile)).NotTo(BeAnExistingFile())
	})

	It("can't confirm without a client certificate", func() {
		_, _, err := run(append([]string{"bootstrap", "confirm"}, bootstrapAPI(endpoints.ModeNormal).apiFlags...)...)
		Ω(err).To(MatchError(ContainSubstring("pki init")))
	})

	It("fails with another secret than the one of the api", func() {
		bootstrapSecretEnv("not the secret of the api")
		_, _, err := run(append([]string{"bootstrap"}, bootstrapAPI(endpoints.ModeBootstrap).flags...)...)
		Ω(err).To(MatchError(ContainSubstring("wrong bootstrap proof")))
	})

	It("refuses an api in normal mode", func() {
		bootstrapSecretEnv(testBootstrapSecret)
		_, _, err := run(append([]string{"bootstrap"}, bootstrapAPI(endpoints.ModeNormal).flags...)...)
		Ω(err).To(MatchError(ContainSubstring("not in bootstrap mode")))
		Ω(filepath.Join(dir, bootstrap.PublicKeyFile)).NotTo(BeAnExistingFile(), "no key pair without a bootstrap")
	})

	It("fails without a probe port", func() {
		bootstrapSecretEnv(testBootstrapSecret)
		flags := bootstrapAPI(endpoints.ModeBootstrap).flags
		for i, f := range flags {
			if f == "--probe-port" {
				flags[i+1] = "1"
			}
		}
		_, _, err := run(append([]string{"bootstrap"}, flags...)...)
		Ω(err).To(MatchError(ContainSubstring("probe port")))
	})

	It("fails without a passphrase for a new key pair", func() {
		bootstrapSecretEnv(testBootstrapSecret)
		noPassphraseEnv()
		_, _, err := run(append([]string{"bootstrap"}, bootstrapAPI(endpoints.ModeBootstrap).flags...)...)
		Ω(err).To(MatchError(ContainSubstring("passphrase")))
	})

	It("is in the help", func() {
		out, _, err := run("--help")
		Ω(err).NotTo(HaveOccurred())
		Ω(strings.Contains(out, "bootstrap")).To(BeTrue())
	})
})
