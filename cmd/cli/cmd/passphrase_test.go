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
	"errors"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/pki"
	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
)

// fakeTerminal makes the cli think it runs in a terminal, where the user types answers (one per prompt)
func fakeTerminal(answers ...string) {
	origTerminal, origRead := stdinIsTerminal, readPassword
	DeferCleanup(func() { stdinIsTerminal, readPassword = origTerminal, origRead })
	stdinIsTerminal = func() bool { return true }
	readPassword = func() ([]byte, error) {
		if len(answers) == 0 {
			return nil, errors.New("no more input")
		}
		answer := answers[0]
		answers = answers[1:]
		return []byte(answer), nil
	}
}

// noPassphraseEnv unsets ORIONCLI_CA_PASSPHRASE for the spec
func noPassphraseEnv() {
	Ω(os.Unsetenv(envCAPassphrase)).To(Succeed())
	DeferCleanup(os.Setenv, envCAPassphrase, testCAPassphrase)
}

var _ = Describe("newCAPassphrase", func() {
	It("reads the passphrase from a file, without the trailing newline", func() {
		file := filepath.Join(GinkgoT().TempDir(), "passphrase")
		Ω(os.WriteFile(file, []byte("from a file\r\n"), 0o600)).To(Succeed())
		Ω(newCAPassphrase(file)).To(Equal([]byte("from a file")))
	})

	It("prefers the file over the environment", func() {
		file := filepath.Join(GinkgoT().TempDir(), "passphrase")
		Ω(os.WriteFile(file, []byte("from a file"), 0o600)).To(Succeed())
		Ω(newCAPassphrase(file)).To(Equal([]byte("from a file")))
	})

	It("fails on a missing file", func() {
		_, err := newCAPassphrase(filepath.Join(GinkgoT().TempDir(), "missing"))
		Ω(err).To(MatchError(ContainSubstring("cannot read")))
	})

	It("reads the passphrase from the environment", func() {
		Ω(newCAPassphrase("")).To(Equal([]byte(testCAPassphrase)))
	})

	It("prompts twice on a terminal", func() {
		noPassphraseEnv()
		fakeTerminal("typed passphrase", "typed passphrase")
		Ω(newCAPassphrase("")).To(Equal([]byte("typed passphrase")))
	})

	It("refuses passphrases that don't match", func() {
		noPassphraseEnv()
		fakeTerminal("typed passphrase", "another passphrase")
		_, err := newCAPassphrase("")
		Ω(err).To(MatchError("the passphrases don't match"))
	})

	It("fails when the terminal can't be read", func() {
		noPassphraseEnv()
		fakeTerminal()
		_, err := newCAPassphrase("")
		Ω(err).To(MatchError(ContainSubstring("failed to read the passphrase")))
	})

	It("refuses a short passphrase", func() {
		noPassphraseEnv()
		fakeTerminal("short", "short")
		_, err := newCAPassphrase("")
		Ω(err).To(MatchError("the passphrase for the CA keys (from the terminal) must be at least 8 characters"))
	})

	It("needs a passphrase without a terminal", func() {
		noPassphraseEnv()
		_, err := newCAPassphrase("")
		Ω(err).To(MatchError(ContainSubstring("a passphrase for the CA keys is required")))
	})
})

var _ = Describe("pki init with a passphrase", func() {
	It("encrypts the CA keys with the passphrase from stdin", func() {
		cliDir := GinkgoT().TempDir()
		usePKIDir(cliDir)
		_, _, err := execute(context.Background(), "a passphrase from stdin\n", "pki", "init",
			"--api-dir", GinkgoT().TempDir(), "--ca-passphrase-file", "-")
		Ω(err).NotTo(HaveOccurred())

		key := oriontls.PrivateKey{Path: filepath.Join(cliDir, pki.CLICADir, "root.key")}
		Ω(key.Load()).To(MatchError(oriontls.ErrPassphraseRequired))
		key.Passphrase = []byte("a passphrase from stdin")
		Ω(key.Load()).To(Succeed())
	})

	It("writes nothing without a passphrase", func() {
		noPassphraseEnv()
		cliDir := GinkgoT().TempDir()
		usePKIDir(cliDir)
		_, _, err := run("pki", "init", "--api-dir", GinkgoT().TempDir())
		expectError(err, "a passphrase for the CA keys is required", "")
		Ω(filepath.Join(cliDir, pki.CLICADir)).NotTo(BeADirectory())
	})
})
