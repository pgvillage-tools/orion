// Copyright 2026 PgVillage
// Copyright 2015 Sorint.lab
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

package sentinel

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("creating and running a sentinel", func() {
	// etcdConfig returns a Config with an etcd endpoint where nothing listens: the store client connects on its first
	// call, so a sentinel can be created and run without a store
	etcdConfig := func() Config {
		var cfg Config
		cfg.ClusterName = "cluster1"
		cfg.StoreBackend = "etcdv3"
		cfg.StoreEndpoints = "http://127.0.0.1:1"
		cfg.StorePrefix = "orion/cluster"
		cfg.StoreTimeout = 100 * time.Millisecond
		return cfg
	}

	DescribeTable("reads the initial cluster spec file",
		// name is the file in a temporary dir, "" for no initial spec, and the file is only written with content; want
		// is the error, or "" when the sentinel is created
		func(ctx SpecContext, name, content, want string) {
			cfg := etcdConfig()
			if name != "" {
				cfg.InitialClusterSpecFile = filepath.Join(GinkgoT().TempDir(), name)
				if content != "" {
					Ω(os.WriteFile(cfg.InitialClusterSpecFile, []byte(content), 0o600)).To(Succeed())
				}
			}
			s, err := NewSentinel(ctx, "sentinel1", &cfg, make(chan bool))
			if want != "" {
				Ω(err).To(MatchError(ContainSubstring(want)))
				return
			}
			Ω(err).NotTo(HaveOccurred())
			Ω(s.uid).To(Equal("sentinel1"))
			if name != "" {
				Ω(s.initialClusterSpec).NotTo(BeNil())
			} else {
				Ω(s.initialClusterSpec).To(BeNil())
			}
		},
		Entry("creates a sentinel without an initial spec", "", "", ""),
		Entry("creates a sentinel with a valid spec", "valid.json", `{"initMode":"new"}`, ""),
		Entry("fails on a missing file", "missing.json", "", "cannot read provided initial cluster"),
		Entry("fails on a file that isn't json", "bad.json", `{`, "cannot parse provided initial cluster"),
		Entry("fails on an invalid spec", "invalid.json", `{"initMode":"bogus"}`, "invalid initial cluster"),
	)

	It("fails to create or run without a store", func(ctx SpecContext) {
		// the kubernetes backend needs POD_NAME (and a cluster)
		GinkgoT().Setenv("POD_NAME", "")
		var cfg Config
		cfg.ClusterName = "cluster1"
		cfg.StoreBackend = "kubernetes"
		_, err := NewSentinel(ctx, "sentinel1", &cfg, make(chan bool))
		Ω(err).To(HaveOccurred(), "expected an error without a store")
		Ω(Run(ctx, cfg)).NotTo(Succeed(), "expected Run to fail without a store")
	})

	It("runs until the context is done", func(ctx SpecContext) {
		cfg := etcdConfig()
		cfg.MetricsListenAddress = "127.0.0.1:0"
		runCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		DeferCleanup(cancel)
		// the store doesn't answer, so the sentinel never leads; Run returns when ctx is done
		Ω(Run(runCtx, cfg)).To(Succeed())
	})
})
