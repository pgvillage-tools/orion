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

package cmd

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	client "github.com/pgvillage-tools/orion/pkg/api_client"

	"github.com/spf13/cobra"
)

const (
	tabWidth = 8
)

var cmdStatus = &cobra.Command{
	Use:   "status",
	RunE:  status,
	Short: "Display the current cluster status",
}

var statusOpts struct {
	Format string
}

func init() {
	cmdStatus.PersistentFlags().StringVarP(&statusOpts.Format, "format", "f", "", "output format")
	CmdCLI.AddCommand(cmdStatus)
}

// Status stores that state of all sentinels, proxies, keepers and the cluster
type Status struct {
	Sentinels []SentinelStatus `json:"sentinels"`
	Proxies   []ProxyStatus    `json:"proxies"`
	Keepers   []KeeperStatus   `json:"keepers"`
	Cluster   ClusterStatus    `json:"cluster"`
}

// SentinelStatus stores the status of the Sentinel
type SentinelStatus struct {
	UID    string `json:"uid"`
	Leader bool   `json:"leader"`
}

// ProxyStatus stores the status of the Proxy
type ProxyStatus struct {
	UID        string `json:"uid"`
	Generation int64  `json:"generation"`
}

// KeeperStatus stores the status of the Keeper
type KeeperStatus struct {
	UID                 string `json:"uid"`
	ListenAddress       string `json:"listen_address"`
	Healthy             bool   `json:"healthy"`
	PgHealthy           bool   `json:"pg_healthy"`
	PgWantedGeneration  int64  `json:"pg_wanted_generation"`
	PgCurrentGeneration int64  `json:"pg_current_generation"`
}

// ClusterStatus stores the status of the Cluster
type ClusterStatus struct {
	Available       bool   `json:"available"`
	MasterKeeperUID string `json:"master_keeper_uid"`
	MasterDBUID     string `json:"master_db_uid"`
}

func status(_ *cobra.Command, _ []string) error {
	switch statusOpts.Format {
	case "json", "text", "":
	default:
		return fmt.Errorf("unrecognised output format %s", statusOpts.Format)
	}
	apiClient, err := defaultAPIClient()
	if err != nil {
		return err
	}
	ic, httpCode, err := apiClient.GetStatus()
	if err != nil {
		return apiError("failed to get the cluster status", httpCode, err)
	}
	if statusOpts.Format == "json" {
		return renderJSON(ic)
	}
	return renderText(apiClient, ic)
}

func renderJSON(ic *cluster.InfoCluster) error {
	output, err := json.MarshalIndent(ic, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to marshal the status: %w", err)
	}
	stdout("%s", output)
	return nil
}

// tabPrinter prints aligned columns to the output of the cli; it keeps the first error, so it can be checked once
type tabPrinter struct {
	tw  *tabwriter.Writer
	err error
}

func newTabPrinter() *tabPrinter {
	return &tabPrinter{tw: tabwriter.NewWriter(CmdCLI.OutOrStdout(), 0, tabWidth, 1, '\t', 0)}
}

func (p *tabPrinter) printf(format string, args ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintf(p.tw, format, args...)
	}
}

func (p *tabPrinter) flush() {
	if p.err == nil {
		p.err = p.tw.Flush()
	}
}

func renderText(apiClient client.Connection, ic *cluster.InfoCluster) error {
	tabOut := newTabPrinter()
	stdout("=== Active sentinels ===")
	stdout("")
	if len(ic.Sentinels) == 0 {
		stdout("No active sentinels")
	} else {
		tabOut.printf("ID\tLEADER\n")
		for _, s := range ic.Sentinels {
			tabOut.printf("%s\t%t\n", s.UID, s.Leader)
			tabOut.flush()
		}
	}
	stdout("")
	stdout("=== Active proxies ===")
	stdout("")
	if len(ic.Proxies) == 0 {
		stdout("No active proxies")
	} else {
		tabOut.printf("ID\n")
		for _, p := range ic.Proxies {
			tabOut.printf("%s\n", p.UID)
			tabOut.flush()
		}
	}
	stdout("")
	stdout("=== Keepers ===")
	stdout("")
	if len(ic.Keepers) == 0 {
		stdout("No keepers available")
		stdout("")
	} else {
		tabOut.printf("UID\tHEALTHY\tPG LISTENADDRESS\tPG HEALTHY\tPG WANTEDGENERATION\tPG CURRENTGENERATION\n")
		for _, k := range ic.Keepers {
			tabOut.printf("%s\t%t\t%s\t%t\t%d\t%d\t\n", k.UID, k.Healthy, k.ListenAddress, k.PgHealthy,
				k.PgWantedGeneration, k.PgCurrentGeneration)
			tabOut.flush()
		}
	}
	if tabOut.err != nil {
		return fmt.Errorf("failed to write the status: %w", tabOut.err)
	}

	// the api only sets primaryKeeper when the cluster is available (it has a primary)
	if primaryKeeper, ok := ic.Status["primaryKeeper"].(string); !ok || primaryKeeper == "" {
		stdout("No cluster available")
	} else {
		stdout("")
		stdout("=== Cluster Info ===")
		stdout("")
		if role, ok := ic.Status["role"].(string); ok {
			stdout("Cluster role: %s", role)
		}
		stdout("Master Keeper: %s", primaryKeeper)
	}

	cd, httpCode, getClusterErr := apiClient.GetCluster()
	if getClusterErr != nil {
		return apiError("failed to get the cluster data", httpCode, getClusterErr)
	}
	if primaryDB, ok := ic.Status["primaryDB"].(string); ok && primaryDB != "" {
		stdout("")
		stdout("===== Keepers/DB tree =====")
		stdout("")
		printTree(primaryDB, cd, 0, "", true)
	}
	stdout("")
	return nil
}

func printTree(dbuid string, cd *cluster.Data, level int, prefix string, tail bool) {
	// skip not existing db: specified as a follower but not available in the
	// cluster spec (this should happen only when doing an `orion-cli removekeeper`)
	if _, ok := cd.DBs[dbuid]; !ok {
		return
	}
	out := prefix
	if level > 0 {
		if tail {
			out += "└─"
		} else {
			out += "├─"
		}
	}
	out += cd.DBs[dbuid].Spec.KeeperUID
	if dbuid == cd.Cluster.Status.Master {
		out += " (master)"
	}
	stdout("%s", out)
	db := cd.DBs[dbuid]
	followers := db.Spec.Followers
	c := len(followers)
	for i, f := range followers {
		emptyspace := ""
		if level > 0 {
			emptyspace = "  "
		}
		linespace := "│ "
		if i < c-1 {
			if tail {
				printTree(f, cd, level+1, prefix+emptyspace, false)
			} else {
				printTree(f, cd, level+1, prefix+linespace, false)
			}
		} else {
			if tail {
				printTree(f, cd, level+1, prefix+emptyspace, true)
			} else {
				printTree(f, cd, level+1, prefix+linespace, true)
			}
		}
	}
}
