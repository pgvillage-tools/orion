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
	"bytes"
	"errors"
	"fmt"
	"os"

	"golang.org/x/term"
)

// envCAPassphrase holds the passphrase for the private keys of the CA, for scripts (a terminal is prompted)
const envCAPassphrase = "ORIONCLI_CA_PASSPHRASE"

// minPassphraseLength is the minimum length of a new passphrase
const minPassphraseLength = 8

// The terminal, replaced in the tests
var (
	stdinIsTerminal = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
	readPassword    = func() ([]byte, error) { return term.ReadPassword(int(os.Stdin.Fd())) }
)

// newCAPassphrase returns the passphrase to encrypt new CA keys with: from file (when set), else from
// ORIONCLI_CA_PASSPHRASE, else it prompts for it twice on the terminal.
func newCAPassphrase(file string) ([]byte, error) {
	var (
		passphrase []byte
		source     string
	)
	switch {
	case file != "":
		data, err := readInput(file)
		if err != nil {
			return nil, err
		}
		passphrase, source = bytes.TrimSuffix(bytes.TrimSuffix(data, []byte("\n")), []byte("\r")), file
	case os.Getenv(envCAPassphrase) != "":
		passphrase, source = []byte(os.Getenv(envCAPassphrase)), envCAPassphrase
	case stdinIsTerminal():
		var err error
		if passphrase, err = promptPassphrase("Passphrase for the CA keys: "); err != nil {
			return nil, err
		}
		again, err := promptPassphrase("Repeat the passphrase: ")
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(passphrase, again) {
			return nil, errors.New("the passphrases don't match")
		}
		source = "the terminal"
	default:
		return nil, fmt.Errorf("a passphrase for the CA keys is required: set --ca-passphrase-file or %s, or run "+
			"in a terminal", envCAPassphrase)
	}
	if len(passphrase) < minPassphraseLength {
		return nil, fmt.Errorf("the passphrase for the CA keys (from %s) must be at least %d characters",
			source, minPassphraseLength)
	}
	return passphrase, nil
}

// promptPassphrase writes prompt to stderr, and reads a passphrase from the terminal without echoing it
func promptPassphrase(prompt string) ([]byte, error) {
	// nowhere left to report a failed write to stderr
	_, _ = fmt.Fprint(CmdCLI.ErrOrStderr(), prompt)
	passphrase, err := readPassword()
	_, _ = fmt.Fprintln(CmdCLI.ErrOrStderr())
	if err != nil {
		return nil, fmt.Errorf("failed to read the passphrase: %w", err)
	}
	return passphrase, nil
}
