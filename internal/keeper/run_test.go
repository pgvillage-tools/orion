// Copyright 2026 PgVillage
// Copyright 2017 Sorint.lab
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

package keeper

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// etcdConfig returns a Config with a data dir in dir and an etcd endpoint where nothing listens: the store client
// connects on its first call, so a keeper can be created and run without a store (and without postgres)
func etcdConfig(dir string) Config {
	var cfg Config
	cfg.ClusterName = "cluster1"
	cfg.StoreBackend = "etcdv3"
	cfg.StoreEndpoints = "http://127.0.0.1:1"
	cfg.StorePrefix = "orion/cluster"
	cfg.StoreTimeout = 100 * time.Millisecond
	cfg.DataDir = filepath.Join(dir, "data")
	cfg.PGBinPath = filepath.Join(dir, "bin")
	return cfg
}

var _ = Describe("NewPostgresKeeper", func() {
	DescribeTable("uses",
		// wantUID is "" for a generated one
		func(ctx SpecContext, cfgUID, savedUID, wantUID string) {
			cfg := etcdConfig(GinkgoT().TempDir())
			cfg.UID = cfgUID
			Expect(os.MkdirAll(cfg.DataDir, ownerRWExecPermisions)).To(Succeed())
			if savedUID != "" {
				saved := &PostgresKeeper{cfg: &cfg, keeperLocalState: &LocalState{UID: savedUID}}
				Expect(saved.saveKeeperLocalState()).To(Succeed())
			}
			p, err := NewPostgresKeeper(ctx, &cfg, make(chan error))
			Expect(err).NotTo(HaveOccurred())
			uid := p.keeperLocalState.UID
			Expect(uid).NotTo(BeEmpty())
			if wantUID != "" {
				Expect(uid).To(Equal(wantUID))
			}
			// the uid is saved, so a restart keeps it
			restarted := &PostgresKeeper{cfg: &cfg}
			Expect(restarted.loadKeeperLocalState()).To(Succeed())
			Expect(restarted.keeperLocalState.UID).To(Equal(uid))
			Expect(filepath.IsAbs(p.dataDir)).To(BeTrue(), "data dir %q is not absolute", p.dataDir)
		},
		Entry("a generated uid", "", "", ""),
		Entry("the configured uid", "keeper1", "", "keeper1"),
		Entry("the saved uid", "", "keeper2", "keeper2"),
		Entry("the same uid as before", "keeper3", "keeper3", "keeper3"),
	)

	DescribeTable("fails with a corrupt local state",
		func(ctx SpecContext, file func(p *PostgresKeeper) string) {
			cfg := etcdConfig(GinkgoT().TempDir())
			Expect(os.MkdirAll(cfg.DataDir, ownerRWExecPermisions)).To(Succeed())
			writeFile(GinkgoTB(), file(&PostgresKeeper{cfg: &cfg}), "{not json", ownerRWPermisions)
			_, err := NewPostgresKeeper(ctx, &cfg, make(chan error))
			Expect(err).To(MatchError(ContainSubstring("failed to load")))
		},
		Entry("keeper state", (*PostgresKeeper).keeperLocalStateFilePath),
		Entry("db state", (*PostgresKeeper).dbLocalStateFilePath),
	)
})

var _ = Describe("Run", func() {
	Describe("fails", func() {
		var dir, file string

		BeforeEach(func() {
			dir = GinkgoT().TempDir()
			file = filepath.Join(dir, "file")
			writeFile(GinkgoTB(), file, "", ownerRWPermisions)
		})

		It("with a data dir that can't be created", func(ctx SpecContext) {
			cfg := etcdConfig(dir)
			cfg.DataDir = filepath.Join(file, "data")
			Expect(Run(ctx, cfg)).To(MatchError(ContainSubstring("cannot create data dir")))
		})
		It("with a lock file that can't be opened", func(ctx SpecContext) {
			cfg := etcdConfig(dir)
			cfg.DataDir = filepath.Join(dir, "locked")
			Expect(os.MkdirAll(filepath.Join(cfg.DataDir, "lock"), ownerRWExecPermisions)).To(Succeed())
			Expect(Run(ctx, cfg)).To(MatchError(ContainSubstring("cannot take exclusive lock")))
		})
		It("without a store", func(ctx SpecContext) {
			// the kubernetes backend needs POD_NAME (and a cluster)
			GinkgoT().Setenv("POD_NAME", "")
			cfg := etcdConfig(dir)
			cfg.StoreBackend = "kubernetes"
			Expect(Run(ctx, cfg)).To(MatchError(ContainSubstring("cannot create keeper")))
		})
	})

	DescribeTable("runs until the context is done",
		func(sctx SpecContext, disableLocking bool) {
			cfg := etcdConfig(GinkgoT().TempDir())
			cfg.DisableDataDirLocking = disableLocking
			cfg.MetricsListenAddress = "127.0.0.1:0"
			ctx, cancel := context.WithTimeout(sctx, 500*time.Millisecond)
			defer cancel()
			// the store doesn't answer and there is no postgres: the keeper keeps retrying, and Run returns when
			// ctx is done
			Expect(Run(ctx, cfg)).To(Succeed())
			_, err := os.Stat(filepath.Join(cfg.DataDir, "lock"))
			Expect(os.IsNotExist(err)).To(Equal(disableLocking), "lock file: %v", err)
		},
		Entry("with a lock", false),
		Entry("without a lock", true),
	)
})
