// Copyright 2026 PgVillage
// Copyright 2018 Sorint.lab
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

// Package main is a package that provides functionality concerning postgresSQL lifecycles
package main

import (
	"fmt"
	"log"
	"os"

	apicmd "github.com/pgvillage-tools/orion/cmd/api/cmd"
	cli "github.com/pgvillage-tools/orion/cmd/cli/cmd"
	keepercmd "github.com/pgvillage-tools/orion/cmd/keeper/cmd"
	proxycmd "github.com/pgvillage-tools/orion/cmd/proxy/cmd"
	sentinelcmd "github.com/pgvillage-tools/orion/cmd/sentinel/cmd"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func main() {
	// use os.Args instead of "flags" because "flags" will mess up the man pages!
	var outDir string
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s [output directory]", os.Args[0])
		os.Exit(1)
	}
	outDir = os.Args[1]

	for _, cmd := range []*cobra.Command{
		apicmd.CmdWebApi, cli.CmdCLI, keepercmd.CmdKeeper, proxycmd.CmdProxy, sentinelcmd.CmdSentinel,
	} {
		// without the date, so that the pages only change when the commands change (CI checks they are up to date)
		cmd.DisableAutoGenTag = true
		if err := doc.GenMarkdownTree(cmd, outDir); err != nil {
			log.Fatal(err)
		}
	}
}
