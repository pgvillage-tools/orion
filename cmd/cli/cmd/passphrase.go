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
	passphrase, source, err := readCAPassphrase(file, true)
	if err != nil {
		return nil, err
	}
	if len(passphrase) < minPassphraseLength {
		return nil, fmt.Errorf("the passphrase for the CA keys (from %s) must be at least %d characters",
			source, minPassphraseLength)
	}
	return passphrase, nil
}

// existingCAPassphrase returns the passphrase of existing keys: from file (when set), else from
// ORIONCLI_CA_PASSPHRASE, else it prompts for it once on the terminal
func existingCAPassphrase(file string) ([]byte, error) {
	passphrase, _, err := readCAPassphrase(file, false)
	return passphrase, err
}

// readCAPassphrase reads the passphrase for the CA keys from file (when set), else from ORIONCLI_CA_PASSPHRASE, else
// from the terminal (twice when repeat is set), and returns it with where it came from
func readCAPassphrase(file string, repeat bool) (passphrase []byte, source string, err error) {
	switch {
	case file != "":
		data, readErr := readInput(file)
		if readErr != nil {
			return nil, "", readErr
		}
		return bytes.TrimSuffix(bytes.TrimSuffix(data, []byte("\n")), []byte("\r")), file, nil
	case os.Getenv(envCAPassphrase) != "":
		return []byte(os.Getenv(envCAPassphrase)), envCAPassphrase, nil
	case stdinIsTerminal():
		if passphrase, err = promptPassphrase("Passphrase for the CA keys: "); err != nil || !repeat {
			return passphrase, "the terminal", err
		}
		again, promptErr := promptPassphrase("Repeat the passphrase: ")
		if promptErr != nil {
			return nil, "", promptErr
		}
		if !bytes.Equal(passphrase, again) {
			return nil, "", errors.New("the passphrases don't match")
		}
		return passphrase, "the terminal", nil
	default:
		return nil, "", fmt.Errorf("a passphrase for the CA keys is required: set --ca-passphrase-file or %s, or "+
			"run in a terminal", envCAPassphrase)
	}
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
