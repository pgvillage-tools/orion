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
	"strings"
	"testing"
	"time"
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

func TestNewPostgresKeeper(t *testing.T) {
	for name, tc := range map[string]struct {
		cfgUID   string
		savedUID string
		wantUID  string // "" for a generated one
	}{
		"a generated uid":        {},
		"the configured uid":     {cfgUID: "keeper1", wantUID: "keeper1"},
		"the saved uid":          {savedUID: "keeper2", wantUID: "keeper2"},
		"the same uid as before": {cfgUID: "keeper3", savedUID: "keeper3", wantUID: "keeper3"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := etcdConfig(t.TempDir())
			cfg.UID = tc.cfgUID
			if err := os.MkdirAll(cfg.DataDir, ownerRWExecPermisions); err != nil {
				t.Fatal(err)
			}
			if tc.savedUID != "" {
				saved := &PostgresKeeper{cfg: &cfg, keeperLocalState: &LocalState{UID: tc.savedUID}}
				if err := saved.saveKeeperLocalState(); err != nil {
					t.Fatal(err)
				}
			}
			p, err := NewPostgresKeeper(t.Context(), &cfg, make(chan error))
			if err != nil {
				t.Fatal(err)
			}
			uid := p.keeperLocalState.UID
			if uid == "" || (tc.wantUID != "" && uid != tc.wantUID) {
				t.Errorf("uid %q, want %q", uid, tc.wantUID)
			}
			// the uid is saved, so a restart keeps it
			restarted := &PostgresKeeper{cfg: &cfg}
			if err := restarted.loadKeeperLocalState(); err != nil || restarted.keeperLocalState.UID != uid {
				t.Errorf("saved uid %v (%v), want %q", restarted.keeperLocalState, err, uid)
			}
			if !filepath.IsAbs(p.dataDir) {
				t.Errorf("data dir %q is not absolute", p.dataDir)
			}
		})
	}
}

func TestNewPostgresKeeperCorruptLocalState(t *testing.T) {
	for name, file := range map[string]func(p *PostgresKeeper) string{
		"keeper state": (*PostgresKeeper).keeperLocalStateFilePath,
		"db state":     (*PostgresKeeper).dbLocalStateFilePath,
	} {
		t.Run(name, func(t *testing.T) {
			cfg := etcdConfig(t.TempDir())
			if err := os.MkdirAll(cfg.DataDir, ownerRWExecPermisions); err != nil {
				t.Fatal(err)
			}
			writeFile(t, file(&PostgresKeeper{cfg: &cfg}), "{not json", ownerRWPermisions)
			if _, err := NewPostgresKeeper(t.Context(), &cfg, make(chan error)); err == nil ||
				!strings.Contains(err.Error(), "failed to load") {
				t.Fatalf("got %v, want a load error", err)
			}
		})
	}
}

func TestRunErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	writeFile(t, file, "", ownerRWPermisions)

	t.Run("a data dir that can't be created", func(t *testing.T) {
		cfg := etcdConfig(dir)
		cfg.DataDir = filepath.Join(file, "data")
		if err := Run(t.Context(), cfg); err == nil || !strings.Contains(err.Error(), "cannot create data dir") {
			t.Fatalf("got %v, want a data dir error", err)
		}
	})
	t.Run("a lock file that can't be opened", func(t *testing.T) {
		cfg := etcdConfig(dir)
		cfg.DataDir = filepath.Join(dir, "locked")
		if err := os.MkdirAll(filepath.Join(cfg.DataDir, "lock"), ownerRWExecPermisions); err != nil {
			t.Fatal(err)
		}
		if err := Run(t.Context(), cfg); err == nil || !strings.Contains(err.Error(), "cannot take exclusive lock") {
			t.Fatalf("got %v, want a lock error", err)
		}
	})
	t.Run("without a store", func(t *testing.T) {
		// the kubernetes backend needs POD_NAME (and a cluster)
		t.Setenv("POD_NAME", "")
		cfg := etcdConfig(dir)
		cfg.StoreBackend = "kubernetes"
		if err := Run(t.Context(), cfg); err == nil || !strings.Contains(err.Error(), "cannot create keeper") {
			t.Fatalf("got %v, want a store error", err)
		}
	})
}

func TestRunUntilTheContextIsDone(t *testing.T) {
	for name, disableLocking := range map[string]bool{"with a lock": false, "without a lock": true} {
		t.Run(name, func(t *testing.T) {
			cfg := etcdConfig(t.TempDir())
			cfg.DisableDataDirLocking = disableLocking
			cfg.MetricsListenAddress = "127.0.0.1:0"
			ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
			defer cancel()
			// the store doesn't answer and there is no postgres: the keeper keeps retrying, and Run returns when
			// ctx is done
			if err := Run(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(cfg.DataDir, "lock")); os.IsNotExist(err) != disableLocking {
				t.Errorf("lock file: %v, locking disabled: %v", err, disableLocking)
			}
		})
	}
}
