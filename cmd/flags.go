// Copyright 2026 PgVillage
// Copyright 2017 Sorint.lab
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
	"os"

	"github.com/mattn/go-isatty"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/store"
	"github.com/spf13/cobra"
)

// AddCommonFlags is a function that gives flags to commands
func AddCommonFlags(cmd *cobra.Command, cfg *store.CommonConfig) {
	cmd.PersistentFlags().StringVar(
		&cfg.ClusterName,
		"cluster-name",
		"",
		"cluster name")

	cmd.PersistentFlags().StringVar(
		&cfg.StoreBackend,
		"store-backend",
		"",
		"store backend type (etcdv2/etcd, etcdv3, consul or kubernetes)")

	cmd.PersistentFlags().StringVar(
		&cfg.StoreEndpoints,
		"store-endpoints",
		"",
		// revive:disable-next-line
		"a comma-delimited list of store endpoints (use https scheme for tls communication) (defaults: http://127.0.0.1:2379 for etcd, http://127.0.0.1:8500 for consul)")

	cmd.PersistentFlags().DurationVar(
		&cfg.StoreTimeout,
		"store-timeout",
		cluster.DefaultStoreTimeout,
		"store request timeout")

	cmd.PersistentFlags().StringVar(
		&cfg.StorePrefix,
		"store-prefix",
		common.StorePrefix,
		"the store base prefix")

	cmd.PersistentFlags().StringVar(
		&cfg.StoreCertFile,
		"store-cert-file",
		"",
		"certificate file for client identification to the store")

	cmd.PersistentFlags().StringVar(
		&cfg.StoreKeyFile,
		"store-key",
		"",
		"private key file for client identification to the store")

	cmd.PersistentFlags().BoolVar(
		&cfg.StoreSkipTLSVerify,
		"store-skip-tls-verify",
		false,
		"skip store certificate verification (insecure!!!)")

	cmd.PersistentFlags().StringVar(
		&cfg.StoreCAFile,
		"store-ca-file",
		"",
		"verify certificates of HTTPS-enabled store servers using this CA bundle")

	cmd.PersistentFlags().StringVar(
		&cfg.MetricsListenAddress,
		"metrics-listen-address",
		"",
		"metrics listen address i.e \"0.0.0.0:8080\" (disabled by default)")

	if !cfg.IsCLI {
		cmd.PersistentFlags().BoolVar(
			&cfg.LogColor,
			"log-color",
			false,
			"enable color in log output (default if attached to a terminal)")
		cmd.PersistentFlags().StringVar(
			&cfg.LogLevel,
			"log-level",
			"info",
			"debug, info (default), warn or error")
	}

	if cfg.IsCLI {
		cmd.PersistentFlags().StringVar(
			&cfg.LogLevel,
			"log-level",
			"info",
			"debug, info (default), warn or error")
		cmd.PersistentFlags().StringVar(
			&cfg.KubeConfig,
			"kubeconfig",
			"",
			"path to kubeconfig file. Overrides $KUBECONFIG")
		cmd.PersistentFlags().StringVar(
			&cfg.KubeContext,
			"kube-context",
			"",
			"name of the kubeconfig context to use")
		cmd.PersistentFlags().StringVar(
			&cfg.KubeNamespace,
			"kube-namespace", "",
			"name of the kubernetes namespace to use")
	}
}

// IsColorLoggerEnable is a function which checks if logs should be colorized or not
func IsColorLoggerEnable(cmd *cobra.Command, cfg *store.CommonConfig) bool {
	if cmd.PersistentFlags().Changed("log-color") {
		return cfg.LogColor
	}
	return isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
}
