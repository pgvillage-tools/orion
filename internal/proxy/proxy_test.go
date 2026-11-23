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

package proxy

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Run", func() {
	It("fails without a store", func(ctx SpecContext) {
		// the kubernetes backend needs POD_NAME (and a cluster): no store, so no proxy
		GinkgoT().Setenv("POD_NAME", "")
		cfg := Config{ListenAddress: "127.0.0.1", Port: "0"}
		cfg.ClusterName = "cluster1"
		cfg.StoreBackend = "kubernetes"
		Expect(Run(ctx, cfg)).NotTo(Succeed(), "expected an error without a store")
	})

	It("runs until the context is done", func(ctx SpecContext) {
		cfg := Config{ListenAddress: "127.0.0.1", Port: "0", StopListening: true}
		cfg.ClusterName = "cluster1"
		cfg.StoreBackend = "etcdv3"
		cfg.StoreEndpoints = "http://127.0.0.1:1"
		cfg.StorePrefix = "orion/cluster"
		cfg.StoreTimeout = shortTimeout
		cfg.MetricsListenAddress = "127.0.0.1:0"
		runCtx, cancel := context.WithTimeout(ctx, 4*shortTimeout)
		DeferCleanup(cancel)
		// the store doesn't answer, so the checks fail; Run stops with ctx
		Expect(Run(runCtx, cfg)).To(Succeed())
	})
})
