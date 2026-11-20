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
	"context"
	"strings"
	"testing"

	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/store"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

func TestTheKubeResourceKindFlagIsGone(t *testing.T) {
	c := &cobra.Command{Use: "test"}
	AddCommonFlags(c, &store.CommonConfig{})
	if c.PersistentFlags().Lookup("kube-resource-kind") != nil {
		t.Error("--kube-resource-kind is still there")
	}
}

func TestSetupLogging(t *testing.T) {
	level := func(comp logging.Component) zerolog.Level {
		_, logger := logging.GetLogComponent(context.Background(), comp)
		return logger.GetLevel()
	}
	setup := func(t *testing.T, args ...string) error {
		t.Helper()
		cfg := &store.CommonConfig{}
		c := &cobra.Command{Use: "test"}
		AddCommonFlags(c, cfg)
		if err := c.PersistentFlags().Parse(args); err != nil {
			t.Fatal(err)
		}
		return SetupLogging(c, cfg, false)
	}
	t.Cleanup(func() {
		logging.SetStaticLevel("info")
		logging.SetStaticComponents(nil)
	})

	if err := setup(t, "--log-level=warn", "--log-components=keeper=debug,kv-store=error"); err != nil {
		t.Fatal(err)
	}
	for comp, want := range map[logging.Component]zerolog.Level{
		logging.KeeperComponent:   zerolog.DebugLevel,
		logging.StoreComponent:    zerolog.ErrorLevel,
		logging.SentinelComponent: zerolog.WarnLevel,
	} {
		if got := level(comp); got != want {
			t.Errorf("level of component %d = %v, want %v", comp, got, want)
		}
	}

	if err := setup(t, "--log-components=keeper=loud"); err == nil ||
		!strings.Contains(err.Error(), `invalid --log-components: unknown log level "loud"`) {
		t.Errorf("expected an invalid level error, got %v", err)
	}
}
