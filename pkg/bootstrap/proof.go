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

// Package bootstrap holds the parts of the bootstrap protocol that the api and the cli share: the proofs that both
// know the bootstrap secret, bound to their TLS session. See docs/api/bootstrap-design.md, "The bootstrap protocol".
//
// The api starts in bootstrap mode with a self-signed certificate, which the cli can't verify. Instead, both prove
// that they know the shared secret, without sending it: each sends an HMAC-SHA256 with the secret over both nonces
// and keying material exported from the TLS session (RFC 5705, RFC 9266). A man in the middle has a TLS session with
// each side, with different keying material, so he can't forward a proof from one session to the other.
package bootstrap

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"errors"
)

const (
	// ExporterLabel is the label of the keying material exported from the TLS session (RFC 5705)
	ExporterLabel = "EXPORTER-orion-bootstrap"
	// ExporterLength is the length of the exported keying material, in bytes
	ExporterLength = 32
	// NonceSize is the size of the nonces of the api and the cli, in bytes
	NonceSize = 32
	// ProofSize is the size of a proof (HMAC-SHA256), in bytes
	ProofSize = sha256.Size
	// MinSecretLength is the minimum length of a bootstrap secret you choose: with an active man in the middle, a weak
	// secret can be guessed offline
	MinSecretLength = 20

	// labelCLI and labelAPI separate the proof of the cli from the proof of the api, so one can't be replayed as the
	// other
	labelCLI = "orion-bootstrap cli"
	labelAPI = "orion-bootstrap api"
)

// ErrSize is returned for a nonce or keying material of the wrong size
var ErrSize = errors.New("wrong size of a bootstrap nonce or keying material")

// ExportKeyingMaterial returns the keying material of the TLS session in state that the proofs cover. TLS 1.3 is
// required: with TLS 1.2 the exporter is only safe with extended master secret, which the proofs can't check.
func ExportKeyingMaterial(state *tls.ConnectionState) ([]byte, error) {
	if state == nil || state.Version != tls.VersionTLS13 {
		return nil, errors.New("the bootstrap requires TLS 1.3")
	}
	return state.ExportKeyingMaterial(ExporterLabel, nil, ExporterLength)
}

// CLIProof returns the proof of the cli: HMAC-SHA256(secret, "orion-bootstrap cli" || 0x00 || apiNonce || cliNonce ||
// keyingMaterial)
func CLIProof(secret, apiNonce, cliNonce, keyingMaterial []byte) ([]byte, error) {
	return proof(labelCLI, secret, apiNonce, cliNonce, keyingMaterial)
}

// APIProof returns the proof of the api: HMAC-SHA256(secret, "orion-bootstrap api" || 0x00 || apiNonce || cliNonce ||
// keyingMaterial)
func APIProof(secret, apiNonce, cliNonce, keyingMaterial []byte) ([]byte, error) {
	return proof(labelAPI, secret, apiNonce, cliNonce, keyingMaterial)
}

// Equal compares two proofs in constant time
func Equal(a, b []byte) bool {
	return hmac.Equal(a, b)
}

// proof returns HMAC-SHA256(secret, label || 0x00 || apiNonce || cliNonce || keyingMaterial). All parts after the
// label have a fixed size, so the input is unambiguous.
func proof(label string, secret, apiNonce, cliNonce, keyingMaterial []byte) ([]byte, error) {
	if len(apiNonce) != NonceSize || len(cliNonce) != NonceSize || len(keyingMaterial) != ExporterLength {
		return nil, ErrSize
	}
	mac := hmac.New(sha256.New, secret)
	for _, part := range [][]byte{[]byte(label), {0}, apiNonce, cliNonce, keyingMaterial} {
		_, _ = mac.Write(part)
	}
	return mac.Sum(nil), nil
}
