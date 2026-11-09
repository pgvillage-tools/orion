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
func newAPIClient(host string, port uint16) (client.Connection, error) {
	files, err := cliTLSFiles()
	if err != nil {
		return client.Connection{}, err
	}
	if !files.Exist() {
		return client.Connection{}, fmt.Errorf("client certificate not found (cert %s, key %s, ca %s): create it "+
			"with `orion-cli pki init`", files.CertFile, files.KeyFile, files.CAFile)
	}
	tlsConfig, err := files.ClientConfig()
	if err != nil {
		return client.Connection{}, fmt.Errorf("failed to load the client certificate: %w", err)
	}
	return client.NewTLSConnection(host, port, viper.GetDuration(flagTimeout), tlsConfig), nil
}

// defaultAPIClient returns a connection to the api on the configured host and port
func defaultAPIClient() (client.Connection, error) {
	return newAPIClient(viper.GetString(flagHost), viper.GetUint16(flagPort))
}
