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

package flagutil_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/flagutil"
	flag "github.com/spf13/pflag"
)

var _ = Describe("SetFlagsFromEnv", func() {
	var fs *flag.FlagSet

	BeforeEach(func() {
		fs = flag.NewFlagSet("test", flag.ContinueOnError)
		fs.String("store-endpoints", "default", "")
		fs.Duration("store-timeout", time.Second, "")
		fs.Int("port", 1, "")
	})

	It("sets unset flags from PREFIX_FLAG_NAME", func() {
		GinkgoT().Setenv("TEST_STORE_ENDPOINTS", "http://etcd:2379")
		GinkgoT().Setenv("TEST_STORE_TIMEOUT", "3s")
		Ω(flagutil.SetFlagsFromEnv(fs, "TEST")).To(Succeed())
		Ω(fs.GetString("store-endpoints")).To(Equal("http://etcd:2379"))
		Ω(fs.GetDuration("store-timeout")).To(Equal(3 * time.Second))
		Ω(fs.GetInt("port")).To(Equal(1), "the default without an environment variable")
	})

	It("keeps flags that were set on the command line", func() {
		GinkgoT().Setenv("TEST_PORT", "2")
		Ω(fs.Parse([]string{"--port", "3"})).To(Succeed())
		Ω(flagutil.SetFlagsFromEnv(fs, "TEST")).To(Succeed())
		Ω(fs.GetInt("port")).To(Equal(3))
	})

	It("returns an error for an invalid value", func() {
		GinkgoT().Setenv("TEST_PORT", "not-a-number")
		Ω(flagutil.SetFlagsFromEnv(fs, "TEST")).To(MatchError(ContainSubstring("TEST_PORT")))
	})
})
