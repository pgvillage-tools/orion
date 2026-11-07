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

package tls

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

const (
	pemTypePrivateKey = "PRIVATE KEY"
	rsa2048Bits       = 2048
	rsa3072Bits       = 3072
	rsa4096Bits       = 4096
)

// KeyAlgorithm defines the algorithm (and size) of a private key
type KeyAlgorithm string

const (
	// KeyAlgorithmECDSAP256 generates ECDSA keys on the NIST P-256 curve
	KeyAlgorithmECDSAP256 KeyAlgorithm = "ecdsa-p256"
	// KeyAlgorithmRSA2048 generates 2048 bit RSA keys
	KeyAlgorithmRSA2048 KeyAlgorithm = "rsa-2048"
	// KeyAlgorithmRSA3072 generates 3072 bit RSA keys
	KeyAlgorithmRSA3072 KeyAlgorithm = "rsa-3072"
	// KeyAlgorithmRSA4096 generates 4096 bit RSA keys
	KeyAlgorithmRSA4096 KeyAlgorithm = "rsa-4096"
	// DefaultKeyAlgorithm is used when no algorithm is set
	DefaultKeyAlgorithm = KeyAlgorithmECDSAP256
)

// KeyAlgorithms returns all supported key algorithms
func KeyAlgorithms() []KeyAlgorithm {
	return []KeyAlgorithm{KeyAlgorithmECDSAP256, KeyAlgorithmRSA2048, KeyAlgorithmRSA3072, KeyAlgorithmRSA4096}
}

// IsRSA returns true for the RSA algorithms
func (ka KeyAlgorithm) IsRSA() bool {
	switch ka {
	case KeyAlgorithmRSA2048, KeyAlgorithmRSA3072, KeyAlgorithmRSA4096:
		return true
	default:
		return false
	}
}

// PrivateKey holds a private key, and its PEM encoded (PKCS#8) form
type PrivateKey struct {
	key       crypto.Signer
	Algorithm KeyAlgorithm `json:"algorithm"`
	PEM       []byte       `json:"pem"`
	Path      string       `json:"path"`
	dirty     bool
}

// Generate generates a private key, unless one was generated (or loaded) before
func (pk *PrivateKey) Generate() error {
	if pk.key != nil {
		return nil
	}
	if pk.Algorithm == "" {
		pk.Algorithm = DefaultKeyAlgorithm
	}
	var (
		key crypto.Signer
		err error
	)
	switch pk.Algorithm {
	case KeyAlgorithmECDSAP256:
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case KeyAlgorithmRSA2048:
		key, err = rsa.GenerateKey(rand.Reader, rsa2048Bits)
	case KeyAlgorithmRSA3072:
		key, err = rsa.GenerateKey(rand.Reader, rsa3072Bits)
	case KeyAlgorithmRSA4096:
		key, err = rsa.GenerateKey(rand.Reader, rsa4096Bits)
	default:
		return fmt.Errorf("unsupported key algorithm %q", pk.Algorithm)
	}
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}
	pk.key = key
	pk.PEM = nil
	return nil
}

// Encode encodes the private key as PKCS#8 in PEM format, and stores it in the PEM field
func (pk *PrivateKey) Encode() error {
	if pk.PEM != nil {
		return nil
	}
	if pk.key == nil {
		return errors.New("can't encode an uninitialized private key")
	}
	der, err := x509.MarshalPKCS8PrivateKey(pk.key)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}
	pk.PEM = pem.EncodeToMemory(&pem.Block{Type: pemTypePrivateKey, Bytes: der})
	pk.dirty = true
	return nil
}

// Decode parses the PEM field into the private key
func (pk *PrivateKey) Decode() error {
	block, _ := pem.Decode(pk.PEM)
	if block == nil {
		return errors.New("no PEM data found for private key")
	}
	var (
		key any
		err error
	)
	switch block.Type {
	case pemTypePrivateKey:
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		key, err = x509.ParseECPrivateKey(block.Bytes)
	default:
		return fmt.Errorf("unsupported PEM type %q for private key", block.Type)
	}
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return errors.New("private key can't be used for signing")
	}
	pk.key = signer
	return nil
}

// Load reads the private key from Path
func (pk *PrivateKey) Load() error {
	data, err := os.ReadFile(pk.Path)
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}
	pk.PEM = data
	pk.dirty = false
	return pk.Decode()
}

// Save writes the private key PEM to Path, readable for the owner only
func (pk *PrivateKey) Save() error {
	if !pk.dirty || pk.Path == "" || len(pk.PEM) == 0 {
		return nil
	}
	if err := writeFile(pk.Path, pk.PEM, privateFileMode); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}
	pk.dirty = false
	return nil
}

// PublicKey returns the public key belonging to the private key.
// PublicKey returns an error when the private key is not initialized.
func (pk PrivateKey) PublicKey() (crypto.PublicKey, error) {
	if pk.key == nil {
		return nil, errors.New("can't get public key from uninitialized private key")
	}
	return pk.key.Public(), nil
}

// Signer returns the private key as a crypto.Signer (nil when not initialized)
func (pk PrivateKey) Signer() crypto.Signer {
	return pk.key
}
