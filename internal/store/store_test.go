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

package store

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/consensus"
	"github.com/pgvillage-tools/orion/internal/util"
)

var _ = Describe("CheckCommonConfig", func() {
	DescribeTable("accepts the supported store backends",
		func(backend string) {
			cfg := CommonConfig{ClusterName: "c", StoreBackend: backend}
			Ω(CheckCommonConfig(&cfg)).To(Succeed())
			Ω(cfg.StoreBackend).To(Equal(backend))
		},
		Entry("etcdv3", "etcdv3"),
		Entry("kubernetes, which needs nothing more", "kubernetes"),
	)

	DescribeTable("refuses",
		func(cfg CommonConfig, msg string) {
			Ω(CheckCommonConfig(&cfg)).To(MatchError(ContainSubstring(msg)))
		},
		Entry("etcd (v2), no longer supported", CommonConfig{ClusterName: "c", StoreBackend: "etcd"},
			"no longer supported"),
		Entry("etcdv2, no longer supported", CommonConfig{ClusterName: "c", StoreBackend: "etcdv2"},
			"no longer supported"),
		Entry("consul, no longer supported", CommonConfig{ClusterName: "c", StoreBackend: "consul"},
			"no longer supported"),
		Entry("no cluster name", CommonConfig{StoreBackend: "etcdv3"}, "cluster name required"),
		Entry("no store backend", CommonConfig{ClusterName: "c"}, "store backend type required"),
		Entry("an unknown store backend", CommonConfig{ClusterName: "c", StoreBackend: "zookeeper"},
			"Unknown store backend"),
	)
})

var _ = Describe("the store and the election", func() {
	// kubeconfig is a kubeconfig for a server that isn't there: the kubernetes clients only connect on their first
	// request
	const kubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: c
  cluster:
    server: https://127.0.0.1:1
users:
- name: u
contexts:
- name: c
  context:
    cluster: c
    user: u
    namespace: orion
current-context: c
`

	var (
		ctx  context.Context
		etcd *CommonConfig
		kube *CommonConfig
	)

	BeforeEach(func() {
		ctx = context.Background()
		// the etcd client connects lazily too
		etcd = &CommonConfig{ClusterName: "c", StoreBackend: "etcdv3", StoreEndpoints: "http://127.0.0.1:1",
			StorePrefix: "orion/cluster", StoreTimeout: time.Second}
		path := filepath.Join(GinkgoT().TempDir(), "kubeconfig")
		Ω(os.WriteFile(path, []byte(kubeconfig), 0o600)).To(Succeed())
		kube = &CommonConfig{ClusterName: "c", StoreBackend: "kubernetes", KubeConfig: path,
			StoreTimeout: time.Second}
	})

	It("keeps a cluster in the kv store under the prefix and its name", func() {
		Ω(storePath(etcd)).To(Equal("orion/cluster/c"))
	})

	It("sets the cluster identifier metric", func() {
		Ω(func() { SetMetrics(etcd, "sentinel") }).NotTo(Panic())
	})

	Context("on etcd", func() {
		It("creates a kv backed store and election", func() {
			s, err := NewStore(ctx, etcd)
			Ω(err).NotTo(HaveOccurred())
			Ω(s).To(BeAssignableToTypeOf(&consensus.KVBackedStore{}))

			e, err := NewElection(ctx, etcd, "uid")
			Ω(err).NotTo(HaveOccurred())
			Ω(e).NotTo(BeNil())

			s, e, err = NewStoreAndElection(ctx, etcd, "uid")
			Ω(err).NotTo(HaveOccurred())
			Ω(s).To(BeAssignableToTypeOf(&consensus.KVBackedStore{}))
			Ω(e).NotTo(BeNil())
		})

		It("fails on invalid endpoints", func() {
			etcd.StoreEndpoints = "https://a:1,http://b:2"
			_, err := NewStore(ctx, etcd)
			Ω(err).To(HaveOccurred())
			_, err = NewElection(ctx, etcd, "uid")
			Ω(err).To(HaveOccurred())
			_, _, err = NewStoreAndElection(ctx, etcd, "uid")
			Ω(err).To(HaveOccurred())
		})
	})

	Context("on kubernetes", func() {
		It("creates a kube store and election for a member, with its pod name", func() {
			GinkgoT().Setenv(util.KubePodName, "pod-0")
			s, err := NewStore(ctx, kube)
			Ω(err).NotTo(HaveOccurred())
			Ω(s).To(BeAssignableToTypeOf(&consensus.KubeStore{}))

			e, err := NewElection(ctx, kube, "uid")
			Ω(err).NotTo(HaveOccurred())
			Ω(e).To(BeAssignableToTypeOf(&consensus.KubeElection{}))
		})

		It("needs the pod name of a member, but not of the cli", func() {
			GinkgoT().Setenv(util.KubePodName, "")
			_, err := NewStore(ctx, kube)
			Ω(err).To(MatchError(ContainSubstring(util.KubePodName)))
			_, err = NewElection(ctx, kube, "uid")
			Ω(err).To(MatchError(ContainSubstring(util.KubePodName)))

			kube.IsCLI = true
			_, err = NewStore(ctx, kube)
			Ω(err).NotTo(HaveOccurred())
		})

		It("creates a store and election without a pod name, for the api", func() {
			GinkgoT().Setenv(util.KubePodName, "")
			s, e, err := NewStoreAndElection(ctx, kube, "uid")
			Ω(err).NotTo(HaveOccurred())
			Ω(s).To(BeAssignableToTypeOf(&consensus.KubeStore{}))
			Ω(e).To(BeAssignableToTypeOf(&consensus.KubeElection{}))
		})

		It("fails without a kubeconfig", func() {
			kube.KubeConfig = filepath.Join(GinkgoT().TempDir(), "missing")
			_, err := NewStore(ctx, kube)
			Ω(err).To(HaveOccurred())
			_, err = NewElection(ctx, kube, "uid")
			Ω(err).To(HaveOccurred())
			_, _, err = NewStoreAndElection(ctx, kube, "uid")
			Ω(err).To(HaveOccurred())
		})
	})

	It("refuses an unknown backend in NewStoreAndElection", func() {
		etcd.StoreBackend = "zookeeper"
		_, _, err := NewStoreAndElection(ctx, etcd, "uid")
		Ω(err).To(MatchError(ContainSubstring("unknown store backend")))
	})
})
