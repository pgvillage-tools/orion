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

package api

import (
	"errors"
	"time"

	"github.com/pgvillage-tools/orion/internal/pki"
	"github.com/pgvillage-tools/orion/internal/store"
)

const (
	// DefaultPort is the default api port
	DefaultPort = uint(8443)
	// DefaultProbePort is the default probe port
	DefaultProbePort = uint(8080)
	// DefaultPKIDir is the default directory with the certificates of the api
	DefaultPKIDir = "~/.orion/api"
	// DefaultShutdownDelay is the default time /readyz returns 503 before the api stops, on SIGTERM
	DefaultShutdownDelay = 5 * time.Second
)

// Config is the configuration of the api
type Config struct {
	store.CommonConfig

	// BaseURL is the address the api and probe ports listen on
	BaseURL string
	// Port is the api port (TLS, a client certificate is required)
	Port uint
	// ProbePort serves /livez, /readyz, /status and /metrics (plain HTTP, no authentication)
	ProbePort uint
	// PKIDir holds the certificates of the api, as created by orion-cli pki init
	PKIDir string
	// TLSCertFile, TLSKeyFile and TLSCAFile override the files in PKIDir
	TLSCertFile string
	TLSKeyFile  string
	TLSCAFile   string
	// ShutdownDelay is the time between not being ready anymore and stopping the servers, on SIGTERM
	ShutdownDelay time.Duration
	// BootstrapSecret is the bootstrap secret (from BootstrapSecretEnv); BootstrapSecretFile is a file with it.
	// Without either, the api generates one in bootstrap mode, and logs it.
	BootstrapSecret     string
	BootstrapSecretFile string
	// BootstrapSecretTTL is the time the bootstrap secret is valid, from the start of bootstrap mode
	BootstrapSecretTTL time.Duration
	// Rebootstrap removes the files of a bootstrap the cli didn't confirm, so the api starts a new one
	Rebootstrap bool
}

// errRebootstrapExplicit is returned for --rebootstrap with certificate files that are set explicitly: those are
// never from a bootstrap
var errRebootstrapExplicit = errors.New("--rebootstrap only works with the files in --pki-dir, not with " +
	"--tls-cert-file, --tls-key-file or --tls-ca-file")

// explicitTLSFiles returns whether a certificate file is set explicitly: then there is no bootstrap
func (c Config) explicitTLSFiles() bool {
	return c.TLSCertFile != "" || c.TLSKeyFile != "" || c.TLSCAFile != ""
}

// tlsFiles returns the files of the api: the files in the pki directory, unless a file is set explicitly
func (c Config) tlsFiles() (pki.TLSFiles, error) {
	files := pki.APIFiles(c.PKIDir)
	for _, override := range []struct {
		value  string
		target *string
	}{
		{c.TLSCertFile, &files.CertFile},
		{c.TLSKeyFile, &files.KeyFile},
		{c.TLSCAFile, &files.CAFile},
	} {
		if override.value != "" {
			*override.target = override.value
		}
	}
	return files.Expand()
}
