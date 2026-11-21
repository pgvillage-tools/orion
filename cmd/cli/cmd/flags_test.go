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
	"fmt"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
)

// The api flags --host, --port and --timeout must reach viper (where the api client reads them) for every command
// that uses the api. They used to be defined and bound per command, so only the last bound one worked.
var _ = Describe("The api flags", func() {
	// apiFlagsEntries returns an entry per command that uses the api, with its index (to make its values unique)
	apiFlagsEntries := func() []TableEntry {
		var entries []TableEntry
		for i, command := range [][]string{
			{"clusterdata", "read"}, {"clusterdata", "write"}, {"connect"}, {"failkeeper"}, {"init"}, {"promote"},
			{"removekeeper"}, {"spec"}, {"status"}, {"update"},
		} {
			entries = append(entries, Entry(strings.Join(command, " "), i, command))
		}
		return entries
	}

	BeforeEach(func() {
		// ParseFlags leaves the values on the flags: don't let them reach other specs
		resetCommands(CmdCLI)
		DeferCleanup(resetCommands, CmdCLI)
	})

	DescribeTable("reach viper for every command that uses the api",
		func(i int, command []string) {
			for j, names := range [][3]string{{"--host", "--port", "--timeout"}, {"-H", "-p", "-T"}} {
				// different values per command and per style, so a value left by an earlier parse doesn't make it pass
				host := fmt.Sprintf("api-%d-%d.example", i, j)
				port := uint16(10000 + 100*j + i)
				timeout := time.Duration(10*j+i+1) * time.Second
				flags := []string{names[0], host, names[1], strconv.Itoa(int(port)), names[2], timeout.String()}
				c, args, err := CmdCLI.Find(append(append([]string{}, command...), flags...))
				Ω(err).NotTo(HaveOccurred())
				Ω(c.ParseFlags(args)).To(Succeed())
				Ω(viper.GetString(flagHost)).To(Equal(host), "%v", flags)
				Ω(viper.GetUint16(flagPort)).To(Equal(port), "%v", flags)
				Ω(viper.GetDuration(flagTimeout)).To(Equal(timeout), "%v", flags)
			}
		},
		apiFlagsEntries(),
	)
})
