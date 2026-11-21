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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/store"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

var _ = Describe("AddCommonFlags", func() {
	It("no longer has --kube-resource-kind", func() {
		c := &cobra.Command{Use: "test"}
		AddCommonFlags(c, &store.CommonConfig{})
		Ω(c.PersistentFlags().Lookup("kube-resource-kind")).To(BeNil())
	})
})

var _ = Describe("SetupLogging", func() {
	// level returns the log level of component comp
	level := func(comp logging.Component) zerolog.Level {
		_, logger := logging.GetLogComponent(context.Background(), comp)
		return logger.GetLevel()
	}
	// setup sets up the logging from the common flags args
	setup := func(args ...string) error {
		cfg := &store.CommonConfig{}
		c := &cobra.Command{Use: "test"}
		AddCommonFlags(c, cfg)
		Ω(c.PersistentFlags().Parse(args)).To(Succeed())
		return SetupLogging(c, cfg, false)
	}

	AfterEach(func() {
		logging.SetStaticLevel("info")
		logging.SetStaticComponents(nil)
	})

	It("sets the log level, and the levels of the components in --log-components", func() {
		Ω(setup("--log-level=warn", "--log-components=keeper=debug,kv-store=error")).To(Succeed())
		Ω(level(logging.KeeperComponent)).To(Equal(zerolog.DebugLevel))
		Ω(level(logging.StoreComponent)).To(Equal(zerolog.ErrorLevel))
		Ω(level(logging.SentinelComponent)).To(Equal(zerolog.WarnLevel), "the log level for the others")
	})

	It("refuses an unknown level in --log-components", func() {
		Ω(setup("--log-components=keeper=loud")).
			To(MatchError(ContainSubstring(`invalid --log-components: unknown log level "loud"`)))
	})
})
