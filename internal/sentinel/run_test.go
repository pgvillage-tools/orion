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
	"strings"
	"testing"
	"time"
)

// etcdConfig returns a Config with an etcd endpoint where nothing listens: the store client connects on its first
// call, so a sentinel can be created and run without a store
func etcdConfig() Config {
	var cfg Config
	cfg.ClusterName = "cluster1"
	cfg.StoreBackend = "etcdv3"
	cfg.StoreEndpoints = "http://127.0.0.1:1"
	cfg.StorePrefix = "orion/cluster"
	cfg.StoreTimeout = 100 * time.Millisecond
	return cfg
}

func TestNewSentinelInitialClusterSpec(t *testing.T) {
	dir := t.TempDir()
	file := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for name, tc := range map[string]struct {
		file string
		want string // the error, or "" when the sentinel is created
	}{
		"no initial spec": {},
		"a valid spec":    {file: file("valid.json", `{"initMode":"new"}`)},
		"a missing file":  {file: filepath.Join(dir, "missing.json"), want: "cannot read provided initial cluster"},
		"not json":        {file: file("bad.json", `{`), want: "cannot parse provided initial cluster"},
		"an invalid spec": {file: file("invalid.json", `{"initMode":"bogus"}`), want: "invalid initial cluster"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := etcdConfig()
			cfg.InitialClusterSpecFile = tc.file
			s, err := NewSentinel(t.Context(), "sentinel1", &cfg, make(chan bool))
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("got %v, want an error containing %q", err, tc.want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if s.uid != "sentinel1" || (tc.file != "") != (s.initialClusterSpec != nil) {
				t.Errorf("unexpected sentinel: uid %q, initial spec %v", s.uid, s.initialClusterSpec)
			}
		})
	}
}

func TestNewSentinelWithoutAStore(t *testing.T) {
	// the kubernetes backend needs POD_NAME (and a cluster)
	t.Setenv("POD_NAME", "")
	var cfg Config
	cfg.ClusterName = "cluster1"
	cfg.StoreBackend = "kubernetes"
	if _, err := NewSentinel(t.Context(), "sentinel1", &cfg, make(chan bool)); err == nil {
		t.Fatal("expected an error without a store")
	}
	if err := Run(t.Context(), cfg); err == nil {
		t.Fatal("expected Run to fail without a store")
	}
}

func TestRunUntilTheContextIsDone(t *testing.T) {
	cfg := etcdConfig()
	cfg.MetricsListenAddress = "127.0.0.1:0"
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	// the store doesn't answer, so the sentinel never leads; Run returns when ctx is done
	if err := Run(ctx, cfg); err != nil {
		t.Fatal(err)
	}
}
