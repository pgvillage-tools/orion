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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
	"github.com/pgvillage-tools/orion/internal/logging"
	client "github.com/pgvillage-tools/orion/pkg/api_client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	tabWidth = 8
)

var cmdStatus = &cobra.Command{
	Use:   "status",
	Run:   status,
	Short: "Display the current cluster status",
}

var statusOpts struct {
	Format  string
	Host    string        `mapstructure:"host"`
	Port    uint16        `mapstructure:"port"`
	TLS     bool          `mapstructure:"tls"`
	Timeout time.Duration `mapstructure:"timeout"`
}

func init() {
	_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
	cmdStatus.PersistentFlags().StringVarP(&statusOpts.Format, "format", "f", "", "output format")
	cmdStatus.PersistentFlags().BoolVarP(&statusOpts.TLS, "tls", "t", true, "use tls")
	if err := viper.BindPFlag("tls", cmdStatus.PersistentFlags().Lookup("tls")); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
	cmdStatus.PersistentFlags().Uint16VarP(&statusOpts.Port, "port", "p", defaultAPIPort,
		"protocol for connecting to the api")
	if err := viper.BindPFlag("port", cmdStatus.PersistentFlags().Lookup("port")); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
	cmdStatus.PersistentFlags().StringVarP(&statusOpts.Host, "host", "H", defaultAPIIP,
		"hostname or ip for connecting to the api")
	if err := viper.BindPFlag("host", cmdStatus.PersistentFlags().Lookup("host")); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
	cmdStatus.PersistentFlags().DurationVarP(&statusOpts.Timeout, flagTimeout, "T", defaultTimeout,
		"connection timeout for api endpoint")
	if err := viper.BindPFlag(flagTimeout, cmdStatus.PersistentFlags().Lookup(flagTimeout)); err != nil {
		logger.Fatal().AnErr("error", err).Msg("")
	}
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

func status(_ *cobra.Command, _ []string) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	ic, generateErr := generateStatus(ctx)
	switch statusOpts.Format {
	case "json":
		renderJSON(ctx, ic, generateErr)
	case "text":
		renderText(ctx, ic, generateErr)
	case "":
		renderText(ctx, ic, generateErr)
	default:
		die("unrecognised output format %s", statusOpts.Format)
	}
}

func renderJSON(_ context.Context, ic *cluster.InfoCluster, generateErr error) {
	if generateErr != nil {
		marshalJSON(generateErr)
	} else {
		marshalJSON(status)
	}
}

func marshalJSON(value any) {
	output, err := json.MarshalIndent(value, "", "\t")
	if err != nil {
		die("failed to marshal error: %v", err)
	}
	stdout("%s", output)
}

func tabPrint(ctx context.Context, tw *tabwriter.Writer, formatted string, args ...any) {
	if _, err := fmt.Fprintf(tw, formatted, args...); err != nil {
		_, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
		logger.Fatal().AnErr("err", err).Msg("failed to write to tab writer")
	}
}

func tabFlush(ctx context.Context, tw *tabwriter.Writer) {
	if err := tw.Flush(); err != nil {
		_, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
		logger.Fatal().AnErr("err", err).Msg("failed to flush tab writer")
	}
}
func renderText(ctx context.Context, ic *cluster.InfoCluster, generateErr error) {
	if generateErr != nil {
		die("%v", generateErr)
	}

	tabOut := new(tabwriter.Writer)
	tabOut.Init(os.Stdout, 0, tabWidth, 1, '\t', 0)

	stdout("=== Active sentinels ===")
	stdout("")
	if len(ic.Sentinels) == 0 {
		stdout("No active sentinels")
	} else {
		tabPrint(ctx, tabOut, "ID\tLEADER\n")
		for _, s := range ic.Sentinels {
			tabPrint(ctx, tabOut, "%s\t%t\n", s.UID, s.Leader)
			tabFlush(ctx, tabOut)
		}
	}

	stdout("")
	stdout("=== Active proxies ===")
	stdout("")
	if len(ic.Proxies) == 0 {
		stdout("No active proxies")
	} else {
		tabPrint(ctx, tabOut, "ID\n")
		for _, p := range ic.Proxies {
			tabPrint(ctx, tabOut, "%s\n", p.UID)
			tabFlush(ctx, tabOut)
		}
	}

	stdout("")
	stdout("=== Keepers ===")
	stdout("")
	if len(ic.Keepers) == 0 {
		stdout("No keepers available")
		stdout("")
	} else {
		tabPrint(ctx, tabOut,
			"UID\tHEALTHY\tPG LISTENADDRESS\tPG HEALTHY\tPG WANTEDGENERATION\tPG CURRENTGENERATION\n")
		for _, k := range ic.Keepers {
			tabPrint(
				ctx,
				tabOut,
				"%s\t%t\t%s\t%t\t%d\t%d\t\n",
				k.UID,
				k.Healthy,
				k.ListenAddress,
				k.PgHealthy,
				k.PgWantedGeneration,
				k.PgCurrentGeneration,
			)
			tabFlush(ctx, tabOut)
		}
	}
	primaryKeeper := ic.Status["primaryKeeper"]
	if primaryKeeper == "" {
		stdout("No cluster available")
	} else {
		stdout("")
		stdout("=== Cluster Info ===")
		stdout("")
		if primaryKeeper != "" {
			stdout("Master Keeper: %s", primaryKeeper)
		} else {
			stdout("Master Keeper: (none)")
		}
	}

	ctx, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
	p := endpoints.HTTPS
	if !viper.GetBool(flagTLS) {
		p = endpoints.HTTP
	}
	apiClient := client.NewConnection(p, viper.GetString(flagHost), viper.GetUint16(flagPort),
		viper.GetDuration(flagTimeout))
	cd, httpCode, getClusterErr := apiClient.GetCluster()
	if getClusterErr != nil {
		logger.Fatal().
			AnErr("error", getClusterErr).
			Int("http return code", httpCode).
			Msg("failed to get clusterdata")
	}
	if primaryDB, ok := ic.Status["primaryDB"].(string); ok && primaryDB != "" {
		stdout("")
		stdout("===== Keepers/DB tree =====")
		stdout("")
		printTree(primaryDB, cd, 0, "", true)
	}
	stdout("")
}

func printTree(dbuid string, cd *cluster.Data, level int, prefix string, tail bool) {
	// skip not existing db: specified as a follower but not available in the
	// cluster spec (this should happen only when doing an `orion removekeeper`)
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

func generateStatus(ctx context.Context) (*cluster.InfoCluster, error) {
	ctx, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
	p := endpoints.HTTPS
	if !statusOpts.TLS {
		p = endpoints.HTTP
	}
	apiClient := client.NewConnection(p, statusOpts.Host, statusOpts.Port,
		viper.GetDuration(flagTimeout))
	ic, httpCode, getClusterErr := apiClient.GetStatus()
	if getClusterErr != nil {
		logger.Fatal().
			AnErr("error", getClusterErr).
			Int("http return code", httpCode).
			Msg("failed to get info on cluster")
	}
	return ic, nil
}
