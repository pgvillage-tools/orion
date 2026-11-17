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
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/pgvillage-tools/orion/internal/pki"
)

func TestRunWithoutCertificates(t *testing.T) {
	err := Run(t.Context(), Config{PKIDir: filepath.Join(t.TempDir(), "missing")})
	if !errors.Is(err, pki.ErrFilesNotFound) {
		t.Fatalf("expected ErrFilesNotFound, got %v", err)
	}
}

func TestRunWithUnreadableCertificates(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads every file")
	}
	dir := t.TempDir()
	for _, f := range []string{pki.APIServerCertFile, pki.APIServerKeyFile, pki.APIClientCAFile} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o000); err != nil {
			t.Fatal(err)
		}
	}
	err := Run(t.Context(), Config{PKIDir: dir})
	if err == nil || errors.Is(err, pki.ErrFilesNotFound) {
		t.Fatalf("expected a read error, got %v", err)
	}
}

func TestRunWithInvalidCertificates(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{pki.APIServerCertFile, pki.APIServerKeyFile, pki.APIClientCAFile} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("not a certificate"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := Run(t.Context(), Config{PKIDir: dir}); err == nil {
		t.Fatal("expected an error for files that are no certificates")
	}
}

func TestListen(t *testing.T) {
	ln, err := listen("127.0.0.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	port, err := strconv.ParseUint(strconv.Itoa(ln.Addr().(*net.TCPAddr).Port), 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	// the port is taken now
	if _, err := listen("127.0.0.1", uint(port)); err == nil {
		t.Error("expected an error binding a port that is in use")
	}
}
