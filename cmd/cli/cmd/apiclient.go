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

	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/pki"
	client "github.com/pgvillage-tools/orion/pkg/api_client"
	"github.com/spf13/viper"
)

const (
	flagPKIDir       = "pki-dir"
	defaultCLIPKIDir = "~/.orion/cli"
)

// cliTLSFiles returns the files with the client certificate of the cli
func cliTLSFiles() (pki.TLSFiles, error) {
	return pki.CLIFiles(viper.GetString(flagPKIDir)).Expand()
}

// newAPIClient returns a connection to the api on host and port, which logs in with the client certificate of the
// cli
func newAPIClient(ctx context.Context, host string, port uint16) client.Connection {
	_, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
	files, err := cliTLSFiles()
	if err != nil {
		logger.Fatal().AnErr(logging.KeyError, err).Msg("")
	}
	if !files.Exist() {
		logger.Fatal().
			Str("cert", files.CertFile).
			Str("key", files.KeyFile).
			Str("ca", files.CAFile).
			Msg("client certificate not found: create it with `orion pki init`")
	}
	tlsConfig, err := files.ClientConfig()
	if err != nil {
		logger.Fatal().AnErr(logging.KeyError, err).Msg("failed to load the client certificate")
	}
	return client.NewTLSConnection(host, port, viper.GetDuration(flagTimeout), tlsConfig)
}

// defaultAPIClient returns a connection to the api on the configured host and port
func defaultAPIClient(ctx context.Context) client.Connection {
	return newAPIClient(ctx, viper.GetString(flagHost), viper.GetUint16(flagPort))
}
