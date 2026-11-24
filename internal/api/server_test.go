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
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
)

var _ = Describe("Run", func() {
	It("fails without certificates that are set explicitly", func(ctx SpecContext) {
		dir := filepath.Join(GinkgoT().TempDir(), "missing")
		err := Run(ctx, Config{PKIDir: dir, TLSCertFile: filepath.Join(dir, "server.pem")})
		Expect(err).To(MatchError(pki.ErrFilesNotFound))
	})

	It("refuses --rebootstrap with certificates that are set explicitly", func(ctx SpecContext) {
		dir := GinkgoT().TempDir()
		err := Run(ctx, Config{PKIDir: dir, TLSCAFile: filepath.Join(dir, "ca.pem"), Rebootstrap: true})
		Expect(err).To(MatchError(errRebootstrapExplicit))
	})

	It("fails with unreadable certificates", func(ctx SpecContext) {
		if os.Getuid() == 0 {
			Skip("root reads every file")
		}
		dir := GinkgoT().TempDir()
		for _, f := range []string{pki.APIServerCertFile, pki.APIServerKeyFile, pki.APIClientCAFile} {
			Expect(os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o000)).To(Succeed())
		}
		err := Run(ctx, Config{PKIDir: dir})
		Expect(err).To(HaveOccurred(), "expected a read error")
		Expect(err).NotTo(MatchError(pki.ErrFilesNotFound), "expected a read error")
	})

	It("fails with invalid certificates", func(ctx SpecContext) {
		dir := GinkgoT().TempDir()
		for _, f := range []string{pki.APIServerCertFile, pki.APIServerKeyFile, pki.APIClientCAFile} {
			Expect(os.WriteFile(filepath.Join(dir, f), []byte("not a certificate"), 0o600)).To(Succeed())
		}
		Expect(Run(ctx, Config{PKIDir: dir})).NotTo(Succeed(), "expected an error for files that are no certificates")
	})

	It("serves until the context is done", func(ctx SpecContext) {
		dir := GinkgoT().TempDir()
		opts := pki.InitOptions{CLIDir: filepath.Join(dir, "cli"), APIDir: filepath.Join(dir, "api")}
		_, err := pki.Init(opts)
		Expect(err).NotTo(HaveOccurred())
		cfg := Config{PKIDir: opts.APIDir, BaseURL: "127.0.0.1", ShutdownDelay: time.Millisecond}
		cfg.ClusterName = "cluster1"
		cfg.StoreBackend = "etcdv3"
		// nothing listens there: the store client connects on its first call
		cfg.StoreEndpoints = "http://127.0.0.1:1"
		cfg.StorePrefix = "orion/cluster"
		cfg.StoreTimeout = time.Second

		runCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()
		// port 0: free ports for the api and the probe port; Run serves until ctx is done, then shuts down
		Expect(Run(runCtx, cfg)).To(Succeed())
	})
})

var _ = Describe("listen", func() {
	It("fails to bind a port that is in use", func() {
		ln, err := listen("127.0.0.1", 0)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = ln.Close() })
		port, err := strconv.ParseUint(strconv.Itoa(ln.Addr().(*net.TCPAddr).Port), 10, 32)
		Expect(err).NotTo(HaveOccurred())
		// the port is taken now
		_, err = listen("127.0.0.1", uint(port))
		Expect(err).To(HaveOccurred(), "expected an error binding a port that is in use")
	})
})

var _ = Describe("handleError", func() {
	DescribeTable("writes a problem",
		func(ctx SpecContext, err error, status int, retryAfter string) {
			w := httptest.NewRecorder()
			handleError(ctx, err, w, "failed to patch cluster spec")
			Expect(w.Code).To(Equal(status))
			Expect(w.Header().Get("Retry-After")).To(Equal(retryAfter))
			Expect(w.Header().Get("Content-Type")).To(Equal(endpoints.ContentTypeProblem))
			Expect(w.Body.String()).To(ContainSubstring("failed to patch cluster spec"), "want msg")
			Expect(w.Body.String()).NotTo(ContainSubstring("etcdserver"), "want not the store error")
		},
		Entry("an error", errors.New("boom"), http.StatusInternalServerError, ""),
		Entry("no error", nil, http.StatusInternalServerError, ""),
		Entry("the store timed out",
			fmt.Errorf("cannot store updated cluster spec: %w", rpctypes.ErrTimeout),
			http.StatusServiceUnavailable, storeRetryAfter),
		Entry("the store request hit its deadline",
			fmt.Errorf("cannot get cluster data: %w", context.DeadlineExceeded),
			http.StatusServiceUnavailable, storeRetryAfter),
	)

	It("only logs without a writer", func(ctx SpecContext) {
		handleError(ctx, errors.New("boom"), nil, "failed to get sentinels info")
	})
})
