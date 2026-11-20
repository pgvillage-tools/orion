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
	"testing"
)

func TestCheckCommonConfig(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cfg     CommonConfig
		backend string
		wantErr bool
	}{
		{name: "kubernetes needs nothing more", cfg: CommonConfig{ClusterName: "c", StoreBackend: "kubernetes"}, backend: "kubernetes"},
		{name: "etcdv3", cfg: CommonConfig{ClusterName: "c", StoreBackend: "etcdv3"}, backend: "etcdv3"},
		{name: "etcd (v2) is no longer supported", cfg: CommonConfig{ClusterName: "c", StoreBackend: "etcd"}, wantErr: true},
		{name: "etcdv2 is no longer supported", cfg: CommonConfig{ClusterName: "c", StoreBackend: "etcdv2"}, wantErr: true},
		{name: "consul is no longer supported", cfg: CommonConfig{ClusterName: "c", StoreBackend: "consul"}, wantErr: true},
		{name: "no cluster name", cfg: CommonConfig{StoreBackend: "etcdv3"}, wantErr: true},
		{name: "no store backend", cfg: CommonConfig{ClusterName: "c"}, wantErr: true},
		{name: "unknown store backend", cfg: CommonConfig{ClusterName: "c", StoreBackend: "zookeeper"}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckCommonConfig(&tc.cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.cfg.StoreBackend != tc.backend {
				t.Errorf("store backend: got %q, want %q", tc.cfg.StoreBackend, tc.backend)
			}
		})
	}
}
