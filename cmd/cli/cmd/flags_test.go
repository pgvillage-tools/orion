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
	"testing"
	"time"

	"github.com/spf13/viper"
)

// TestAPIFlags checks that --host, --port and --timeout reach viper (where the api client reads them) for every
// command that uses the api. They used to be defined and bound per command, so only the last bound one worked.
func TestAPIFlags(t *testing.T) {
	for i, command := range [][]string{
		{"clusterdata", "read"}, {"clusterdata", "write"}, {"connect"}, {"failkeeper"}, {"init"}, {"promote"},
		{"removekeeper"}, {"spec"}, {"status"}, {"update"},
	} {
		t.Run(strings.Join(command, " "), func(t *testing.T) {
			for j, names := range [][3]string{{"--host", "--port", "--timeout"}, {"-H", "-p", "-T"}} {
				// different values per command and per style, so a value left by an earlier parse doesn't make it pass
				host := fmt.Sprintf("api-%d-%d.example", i, j)
				port := uint16(10000 + 100*j + i)
				timeout := time.Duration(10*j+i+1) * time.Second
				flags := []string{names[0], host, names[1], strconv.Itoa(int(port)), names[2], timeout.String()}
				c, args, err := CmdCLI.Find(append(append([]string{}, command...), flags...))
				if err != nil {
					t.Fatal(err)
				}
				if err := c.ParseFlags(args); err != nil {
					t.Fatal(err)
				}
				if got := viper.GetString(flagHost); got != host {
					t.Errorf("%v: expected host %q, got %q", flags, host, got)
				}
				if got := viper.GetUint16(flagPort); got != port {
					t.Errorf("%v: expected port %d, got %d", flags, port, got)
				}
				if got := viper.GetDuration(flagTimeout); got != timeout {
					t.Errorf("%v: expected timeout %s, got %s", flags, timeout, got)
				}
			}
		})
	}
}
