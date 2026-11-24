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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/pki"
	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
)

// The stored state of the bootstrap, so the api can continue after a restart: see docs/api/bootstrap-design.md,
// "State and recovery"

const (
	// stateBootstrapping: bootstrap mode; the secret is valid until it expires
	stateBootstrapping = "bootstrapping"
	// stateHandedOver: the cli stored the archive and the api is in normal mode, until the cli confirms
	stateHandedOver = "handed-over"
	// stateConfirmed: the cli confirmed with its certificate, and the api deleted the archive
	stateConfirmed = "confirmed"
)

// bootstrapState is the stored state of the bootstrap (pki.APIBootstrapStateFile in the api directory, mode 0600)
type bootstrapState struct {
	State string `json:"state"`
	// Secret is a generated secret, while bootstrapping: the api logs it only once
	Secret string `json:"secret,omitempty"`
	// SecretSHA256 identifies the secret while bootstrapping (also one that was set, which isn't stored itself)
	SecretSHA256 string `json:"secretSHA256,omitempty"`
	// Expires is when the secret expires, while bootstrapping
	Expires time.Time `json:"expires,omitzero"`
}

// secretHash returns the hash of secret in the state
func secretHash(secret []byte) string {
	sum := sha256.Sum256(secret)
	return hex.EncodeToString(sum[:])
}

// stateFile returns the state file in the api directory dir
func stateFile(dir string) string {
	return filepath.Join(dir, pki.APIBootstrapStateFile)
}

// readBootstrapState returns the state in dir; the zero state when there is none
func readBootstrapState(dir string) (bootstrapState, error) {
	var state bootstrapState
	data, err := os.ReadFile(stateFile(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("failed to read the bootstrap state: %w", err)
	}
	if err = json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("invalid bootstrap state in %s: %w", stateFile(dir), err)
	}
	switch state.State {
	case stateBootstrapping, stateHandedOver, stateConfirmed:
		return state, nil
	default:
		return state, fmt.Errorf("invalid bootstrap state %q in %s", state.State, stateFile(dir))
	}
}

// writeBootstrapState stores state in dir, only readable by the owner
func writeBootstrapState(dir string, state bootstrapState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err = oriontls.WriteFile(stateFile(dir), data, true); err != nil {
		return fmt.Errorf("failed to store the bootstrap state: %w", err)
	}
	return nil
}

// removeFiles removes names in dir that exist, and returns the ones it removed
func removeFiles(dir string, names ...string) ([]string, error) {
	var removed []string
	for _, name := range names {
		p := filepath.Join(dir, name)
		err := os.Remove(p)
		switch {
		case err == nil:
			removed = append(removed, p)
		case !errors.Is(err, fs.ErrNotExist):
			return removed, err
		}
	}
	return removed, nil
}

// apiFileNames are the files of the api that the bootstrap creates, with the archive
var apiFileNames = []string{pki.APIServerCertFile, pki.APIServerKeyFile, pki.APIClientCAFile,
	pki.APIBootstrapArchiveFile}

// prepareBootstrap acts on the stored state in the api directory dir before the api decides on its mode, and
// returns the state:
//   - with rebootstrap, it removes the files of an unconfirmed bootstrap (and its state), so the api starts a new one;
//     after a confirmed bootstrap, or without any (your own certificates), it changes nothing.
//   - an archive while bootstrapping means storing the files after `stored` was interrupted: it removes them, so the
//     bootstrap continues.
func prepareBootstrap(ctx context.Context, dir string, rebootstrap bool) (bootstrapState, error) {
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	state, err := readBootstrapState(dir)
	if err != nil {
		return state, err
	}
	if rebootstrap {
		switch state.State {
		case stateBootstrapping, stateHandedOver:
			removed, rmErr := removeFiles(dir, append(apiFileNames, pki.APIBootstrapStateFile)...)
			if rmErr != nil {
				return state, fmt.Errorf("--rebootstrap failed to remove the files of the bootstrap: %w", rmErr)
			}
			logger.Warn().Strs("removed", removed).Str("state", state.State).
				Msg("--rebootstrap: removed the files of the bootstrap, starting a new one")
			return bootstrapState{}, nil
		case stateConfirmed:
			logger.Warn().Str("dir", dir).Msg("--rebootstrap: the bootstrap was confirmed, so it changes nothing: " +
				"remove the files in the api directory yourself to bootstrap again")
		default:
			logger.Warn().Str("dir", dir).Msg("--rebootstrap: there was no bootstrap, so it changes nothing")
		}
		return state, nil
	}
	if state.State == stateBootstrapping {
		if _, statErr := os.Stat(filepath.Join(dir, pki.APIBootstrapArchiveFile)); statErr == nil {
			removed, rmErr := removeFiles(dir, apiFileNames...)
			if rmErr != nil {
				return state, fmt.Errorf("failed to remove the files of an interrupted bootstrap: %w", rmErr)
			}
			logger.Warn().Strs("removed", removed).
				Msg("the api stopped while it stored its certificates: continuing the bootstrap (move the files " +
					"the cli stored away before bootstrapping again)")
		}
	}
	return state, nil
}

// resumeSecret returns the secret and its expiry for bootstrap mode from the stored state: the stored one when it is
// still valid (for a set secret: when it is the same one), otherwise secret (or a new one, when it is nil) with a new
// expiry. It logs a generated secret once.
func resumeSecret(ctx context.Context, state bootstrapState, secret []byte, ttl time.Duration, now time.Time) (
	[]byte, time.Time, error) {
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	valid := state.State == stateBootstrapping && now.Before(state.Expires)
	switch {
	case secret != nil && valid && state.SecretSHA256 == secretHash(secret):
		logger.Info().Time("expires", state.Expires).Msg("continuing the bootstrap")
		return secret, state.Expires, nil
	case secret == nil && valid && state.Secret != "" && state.SecretSHA256 == secretHash([]byte(state.Secret)):
		logger.Info().Time("expires", state.Expires).
			Msg("continuing the bootstrap with the generated secret it logged before (also in " +
				pki.APIBootstrapStateFile + ")")
		return []byte(state.Secret), state.Expires, nil
	case secret != nil:
		return secret, now.Add(ttl), nil
	}
	generated, err := generateSecret(ctx)
	return generated, now.Add(ttl), err
}

// checkStateInNormalMode acts on the state in dir when the api starts in normal mode: it warns while the cli didn't
// confirm the bootstrap, and it removes the state of a bootstrap that didn't finish (the certificates came from
// elsewhere), which can hold a generated secret
func checkStateInNormalMode(ctx context.Context, dir string, state bootstrapState) error {
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	switch state.State {
	case stateHandedOver:
		logger.Warn().Msg("the cli didn't confirm the bootstrap yet: run `orion-cli bootstrap confirm`, or restart " +
			"the api with --rebootstrap when the cli lost its files")
	case stateBootstrapping:
		if _, err := removeFiles(dir, pki.APIBootstrapStateFile); err != nil {
			return fmt.Errorf("failed to remove the state of an unfinished bootstrap: %w", err)
		}
		logger.Info().Msg("the api has its certificates: removed the state of the unfinished bootstrap")
	}
	return nil
}
