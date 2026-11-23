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

package bootstrap

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hpke"
	"crypto/rand"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
	"golang.org/x/crypto/argon2"
)

// The key pair of the cli: the api encrypts the archive (the CA keys and the private key of the cli certificate) to its
// public key with HPKE, so only the cli can read it. See docs/api/bootstrap-design.md, "The key pair of the cli".

const (
	// PrivateKeyFile is the file of the encrypted private key in the directory of the cli (e.g. ~/.orion/cli)
	PrivateKeyFile = "bootstrap.key"
	// PublicKeyFile is the file of the public key in the directory of the cli
	PublicKeyFile = "bootstrap.pub"

	pemTypePublicKey           = "ORION BOOTSTRAP PUBLIC KEY"
	pemTypeEncryptedPrivateKey = "ORION BOOTSTRAP ENCRYPTED PRIVATE KEY"

	headerKEM           = "KEM"
	headerKDF           = "KDF"
	headerArgon2Time    = "Argon2-Time"
	headerArgon2Memory  = "Argon2-Memory"
	headerArgon2Threads = "Argon2-Threads"
	headerSalt          = "Salt"
	headerNonce         = "Nonce"
	kdfArgon2id         = "argon2id"

	// argon2Time, argon2Memory (KiB) and argon2Threads are the Argon2id parameters for new keys: the second
	// recommendation of RFC 9106, section 4
	argon2Time    = 3
	argon2Memory  = 64 * 1024
	argon2Threads = 4
	// maxArgon2Time, maxArgon2Memory and maxArgon2Threads limit the parameters of a key file, so a crafted file can't
	// make the cli use gigabytes of memory
	maxArgon2Time    = 16
	maxArgon2Memory  = 1024 * 1024
	maxArgon2Threads = 16

	saltSize   = 16
	aesKeySize = 32
	// privateKeyAAD binds the encrypted private key to its purpose
	privateKeyAAD = "orion bootstrap private key"
)

// ErrWrongPassphrase is returned when the private key can't be decrypted with the passphrase
var ErrWrongPassphrase = errors.New("can't decrypt the bootstrap private key: wrong passphrase, or a corrupt file")

// KEM returns the KEM of the key pair of the cli: ML-KEM-768 with X25519 (X-Wing), safe as long as either holds
func KEM() hpke.KEM {
	return hpke.MLKEM768X25519()
}

// GenerateKey returns a new key pair for the cli
func GenerateKey() (hpke.PrivateKey, error) {
	return KEM().GenerateKey()
}

// EncodePublicKey returns the public key as PEM
func EncodePublicKey(pub hpke.PublicKey) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:    pemTypePublicKey,
		Headers: map[string]string{headerKEM: kemID(pub.KEM())},
		Bytes:   pub.Bytes(),
	})
}

// DecodePublicKey reads a public key from PEM
func DecodePublicKey(data []byte) (hpke.PublicKey, error) {
	block, err := decodePEM(data, pemTypePublicKey)
	if err != nil {
		return nil, err
	}
	return KEM().NewPublicKey(block.Bytes)
}

// EncodePrivateKey returns the private key as PEM, encrypted with passphrase: Argon2id derives an AES-256-GCM key from
// the passphrase. The parameters, the salt and the nonce are in the PEM headers.
func EncodePrivateKey(priv hpke.PrivateKey, passphrase []byte) ([]byte, error) {
	if len(passphrase) == 0 {
		return nil, errors.New("a passphrase is required for the bootstrap private key")
	}
	plain, err := priv.Bytes()
	if err != nil {
		return nil, err
	}
	salt := make([]byte, saltSize)
	if _, err = rand.Read(salt); err != nil {
		return nil, err
	}
	aead, err := newAEAD(passphrase, salt, argon2Time, argon2Memory, argon2Threads)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{
		Type: pemTypeEncryptedPrivateKey,
		Headers: map[string]string{
			headerKEM:           kemID(priv.KEM()),
			headerKDF:           kdfArgon2id,
			headerArgon2Time:    strconv.Itoa(argon2Time),
			headerArgon2Memory:  strconv.Itoa(argon2Memory),
			headerArgon2Threads: strconv.Itoa(argon2Threads),
			headerSalt:          base64.StdEncoding.EncodeToString(salt),
			headerNonce:         base64.StdEncoding.EncodeToString(nonce),
		},
		Bytes: aead.Seal(nil, nonce, plain, []byte(privateKeyAAD)),
	}), nil
}

// DecodePrivateKey decrypts a private key from PEM with passphrase
func DecodePrivateKey(data, passphrase []byte) (hpke.PrivateKey, error) {
	block, err := decodePEM(data, pemTypeEncryptedPrivateKey)
	if err != nil {
		return nil, err
	}
	if kdf := block.Headers[headerKDF]; kdf != kdfArgon2id {
		return nil, fmt.Errorf("unsupported key derivation %q for the bootstrap private key", kdf)
	}
	params := map[string]int{}
	for header, maxValue := range map[string]int{
		headerArgon2Time: maxArgon2Time, headerArgon2Memory: maxArgon2Memory, headerArgon2Threads: maxArgon2Threads,
	} {
		v, convErr := strconv.Atoi(block.Headers[header])
		if convErr != nil || v < 1 || v > maxValue {
			return nil, fmt.Errorf("invalid %s in the bootstrap private key (1 to %d)", header, maxValue)
		}
		params[header] = v
	}
	salt, err := base64.StdEncoding.DecodeString(block.Headers[headerSalt])
	if err != nil || len(salt) < saltSize {
		return nil, errors.New("invalid salt in the bootstrap private key")
	}
	nonce, err := base64.StdEncoding.DecodeString(block.Headers[headerNonce])
	if err != nil {
		return nil, errors.New("invalid nonce in the bootstrap private key")
	}
	aead, err := newAEAD(passphrase, salt, uint32(params[headerArgon2Time]), //nolint:gosec // limited above
		uint32(params[headerArgon2Memory]), uint8(params[headerArgon2Threads])) //nolint:gosec // limited above
	if err != nil {
		return nil, err
	}
	if len(nonce) != aead.NonceSize() {
		return nil, errors.New("invalid nonce in the bootstrap private key")
	}
	plain, err := aead.Open(nil, nonce, block.Bytes, []byte(privateKeyAAD))
	if err != nil {
		return nil, ErrWrongPassphrase
	}
	return KEM().NewPrivateKey(plain)
}

// WriteKeyPair writes priv (encrypted with passphrase) and its public key to dir: the private key only readable by the
// owner
func WriteKeyPair(dir string, priv hpke.PrivateKey, passphrase []byte) error {
	data, err := EncodePrivateKey(priv, passphrase)
	if err != nil {
		return err
	}
	if err = oriontls.WriteFile(filepath.Join(dir, PrivateKeyFile), data, true); err != nil {
		return err
	}
	return oriontls.WriteFile(filepath.Join(dir, PublicKeyFile), EncodePublicKey(priv.PublicKey()), false)
}

// ReadPublicKey reads the public key in dir; the error matches fs.ErrNotExist when there is none
func ReadPublicKey(dir string) (hpke.PublicKey, error) {
	data, err := os.ReadFile(filepath.Join(dir, PublicKeyFile))
	if err != nil {
		return nil, err
	}
	return DecodePublicKey(data)
}

// ReadPrivateKey reads the private key in dir, and decrypts it with passphrase
func ReadPrivateKey(dir string, passphrase []byte) (hpke.PrivateKey, error) {
	data, err := os.ReadFile(filepath.Join(dir, PrivateKeyFile))
	if err != nil {
		return nil, err
	}
	return DecodePrivateKey(data, passphrase)
}

// KeyPairExists returns whether dir has a key pair: both files, or neither (an error for one of them)
func KeyPairExists(dir string) (bool, error) {
	var found int
	for _, name := range []string{PrivateKeyFile, PublicKeyFile} {
		_, err := os.Stat(filepath.Join(dir, name))
		switch {
		case err == nil:
			found++
		case !errors.Is(err, fs.ErrNotExist):
			return false, err
		}
	}
	switch found {
	case 0:
		return false, nil
	case 2:
		return true, nil
	default:
		return false, fmt.Errorf("only one of %s and %s exists in %s: remove it to create a new key pair",
			PrivateKeyFile, PublicKeyFile, dir)
	}
}

// newAEAD returns AES-256-GCM with the key Argon2id derives from passphrase and salt
func newAEAD(passphrase, salt []byte, time, memory uint32, threads uint8) (cipher.AEAD, error) {
	block, err := aes.NewCipher(argon2.IDKey(passphrase, salt, time, memory, threads, aesKeySize))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// decodePEM decodes the first PEM block of data, which must have pemType and the KEM of the key pair
func decodePEM(data []byte, pemType string) (*pem.Block, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != pemType {
		return nil, fmt.Errorf("no %s found", pemType)
	}
	if kem := block.Headers[headerKEM]; kem != kemID(KEM()) {
		return nil, fmt.Errorf("unsupported KEM %q for the bootstrap key (only %s)", kem, kemID(KEM()))
	}
	return block, nil
}

// kemID returns the HPKE id of kem, as in the PEM headers (e.g. 0x647A)
func kemID(kem hpke.KEM) string {
	return fmt.Sprintf("0x%04X", kem.ID())
}
