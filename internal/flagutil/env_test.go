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

package flagutil_test

import (
	"strings"
	"testing"
	"time"

	"github.com/pgvillage-tools/orion/internal/flagutil"
	flag "github.com/spf13/pflag"
)

func TestSetFlagsFromEnv(t *testing.T) {
	newFlags := func() *flag.FlagSet {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.String("store-endpoints", "default", "")
		fs.Duration("store-timeout", time.Second, "")
		fs.Int("port", 1, "")
		return fs
	}

	t.Run("sets unset flags from PREFIX_FLAG_NAME", func(t *testing.T) {
		t.Setenv("TEST_STORE_ENDPOINTS", "http://etcd:2379")
		t.Setenv("TEST_STORE_TIMEOUT", "3s")
		fs := newFlags()
		if err := flagutil.SetFlagsFromEnv(fs, "TEST"); err != nil {
			t.Fatal(err)
		}
		if got, _ := fs.GetString("store-endpoints"); got != "http://etcd:2379" {
			t.Errorf("expected the endpoints from the environment, got %q", got)
		}
		if got, _ := fs.GetDuration("store-timeout"); got != 3*time.Second {
			t.Errorf("expected 3s from the environment, got %s", got)
		}
		if got, _ := fs.GetInt("port"); got != 1 {
			t.Errorf("expected the default port without an environment variable, got %d", got)
		}
	})
	t.Run("keeps flags that were set on the command line", func(t *testing.T) {
		t.Setenv("TEST_PORT", "2")
		fs := newFlags()
		if err := fs.Parse([]string{"--port", "3"}); err != nil {
			t.Fatal(err)
		}
		if err := flagutil.SetFlagsFromEnv(fs, "TEST"); err != nil {
			t.Fatal(err)
		}
		if got, _ := fs.GetInt("port"); got != 3 {
			t.Errorf("expected the command line value 3, got %d", got)
		}
	})
	t.Run("returns an error for an invalid value", func(t *testing.T) {
		t.Setenv("TEST_PORT", "not-a-number")
		err := flagutil.SetFlagsFromEnv(newFlags(), "TEST")
		if err == nil || !strings.Contains(err.Error(), "TEST_PORT") {
			t.Errorf("expected an error about TEST_PORT, got %v", err)
		}
	})
}
