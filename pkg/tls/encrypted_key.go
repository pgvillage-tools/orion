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
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // only to read keys encrypted with the PBKDF2 default PRF (hmacWithSHA1)
	"crypto/sha256"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"hash"
)

// Encrypted private keys are PKCS#8 EncryptedPrivateKeyInfo (RFC 5958) with PBES2 (RFC 8018): PBKDF2 with
// HMAC-SHA256 derives an AES-256-CBC key from the passphrase. This is what `openssl pkcs8 -topk8 -v2 aes-256-cbc`
// writes (the OpenSSL 3 default), so openssl and other tools can read the keys with the passphrase.

const (
	pemTypeEncryptedPrivateKey = "ENCRYPTED PRIVATE KEY"
	// pbkdf2Iterations follows the OWASP advice for PBKDF2-HMAC-SHA256
	pbkdf2Iterations = 600_000
	pbkdf2SaltSize   = 16
	aes256KeySize    = 32
	aes128KeySize    = 16
)

var (
	oidPBES2          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 13}
	oidPBKDF2         = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 12}
	oidHMACWithSHA1   = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 7}
	oidHMACWithSHA256 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 9}
	oidAES128CBC      = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 2}
	oidAES256CBC      = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 42}

	// ErrPassphraseRequired is returned when an encrypted private key is decoded without a passphrase
	ErrPassphraseRequired = errors.New("the private key is encrypted: a passphrase is required")
	// ErrWrongPassphrase is returned when an encrypted private key can't be decrypted with the passphrase
	ErrWrongPassphrase = errors.New("can't decrypt the private key: wrong passphrase, or a corrupt key")
)

type encryptedPrivateKeyInfo struct {
	Algorithm     pkix.AlgorithmIdentifier
	EncryptedData []byte
}

type pbes2Params struct {
	KeyDerivationFunc pkix.AlgorithmIdentifier
	EncryptionScheme  pkix.AlgorithmIdentifier
}

type pbkdf2Params struct {
	Salt           []byte
	IterationCount int
	KeyLength      int                      `asn1:"optional"`
	PRF            pkix.AlgorithmIdentifier `asn1:"optional"`
}

// encryptPKCS8 encrypts a PKCS#8 private key (DER) with passphrase, and returns the EncryptedPrivateKeyInfo (DER)
func encryptPKCS8(der, passphrase []byte) ([]byte, error) {
	salt := make([]byte, pbkdf2SaltSize)
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	key, err := pbkdf2.Key(sha256.New, string(passphrase), salt, pbkdf2Iterations, aes256KeySize)
	if err != nil {
		return nil, fmt.Errorf("failed to derive a key from the passphrase: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	padding := aes.BlockSize - len(der)%aes.BlockSize
	data := append(bytes.Clone(der), bytes.Repeat([]byte{byte(padding)}, padding)...)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(data, data)

	kdfParams, err := asn1.Marshal(pbkdf2Params{
		Salt:           salt,
		IterationCount: pbkdf2Iterations,
		PRF:            pkix.AlgorithmIdentifier{Algorithm: oidHMACWithSHA256, Parameters: asn1.NullRawValue},
	})
	if err != nil {
		return nil, err
	}
	ivParam, err := asn1.Marshal(iv)
	if err != nil {
		return nil, err
	}
	params, err := asn1.Marshal(pbes2Params{
		KeyDerivationFunc: pkix.AlgorithmIdentifier{
			Algorithm:  oidPBKDF2,
			Parameters: asn1.RawValue{FullBytes: kdfParams},
		},
		EncryptionScheme: pkix.AlgorithmIdentifier{
			Algorithm:  oidAES256CBC,
			Parameters: asn1.RawValue{FullBytes: ivParam},
		},
	})
	if err != nil {
		return nil, err
	}
	return asn1.Marshal(encryptedPrivateKeyInfo{
		Algorithm:     pkix.AlgorithmIdentifier{Algorithm: oidPBES2, Parameters: asn1.RawValue{FullBytes: params}},
		EncryptedData: data,
	})
}

// decryptPKCS8 decrypts an EncryptedPrivateKeyInfo (DER) with passphrase, and returns the PKCS#8 private key (DER).
// It supports PBES2 with PBKDF2 (HMAC-SHA256 or HMAC-SHA1) and AES-256-CBC or AES-128-CBC.
func decryptPKCS8(der, passphrase []byte) ([]byte, error) {
	if len(passphrase) == 0 {
		return nil, ErrPassphraseRequired
	}
	var info encryptedPrivateKeyInfo
	if err := unmarshalAll(der, &info); err != nil {
		return nil, fmt.Errorf("failed to parse the encrypted private key: %w", err)
	}
	if !info.Algorithm.Algorithm.Equal(oidPBES2) {
		return nil, fmt.Errorf("unsupported encryption %s for the private key (only PBES2)", info.Algorithm.Algorithm)
	}
	var params pbes2Params
	if err := unmarshalAll(info.Algorithm.Parameters.FullBytes, &params); err != nil {
		return nil, fmt.Errorf("failed to parse the PBES2 parameters: %w", err)
	}
	if !params.KeyDerivationFunc.Algorithm.Equal(oidPBKDF2) {
		return nil, fmt.Errorf("unsupported key derivation %s (only PBKDF2)", params.KeyDerivationFunc.Algorithm)
	}
	var kdf pbkdf2Params
	if err := unmarshalAll(params.KeyDerivationFunc.Parameters.FullBytes, &kdf); err != nil {
		return nil, fmt.Errorf("failed to parse the PBKDF2 parameters: %w", err)
	}
	var prf func() hash.Hash
	switch {
	case kdf.PRF.Algorithm == nil, kdf.PRF.Algorithm.Equal(oidHMACWithSHA1):
		prf = sha1.New
	case kdf.PRF.Algorithm.Equal(oidHMACWithSHA256):
		prf = sha256.New
	default:
		return nil, fmt.Errorf("unsupported PBKDF2 hash %s", kdf.PRF.Algorithm)
	}
	var keySize int
	switch {
	case params.EncryptionScheme.Algorithm.Equal(oidAES256CBC):
		keySize = aes256KeySize
	case params.EncryptionScheme.Algorithm.Equal(oidAES128CBC):
		keySize = aes128KeySize
	default:
		return nil, fmt.Errorf("unsupported cipher %s for the private key (only AES-CBC)",
			params.EncryptionScheme.Algorithm)
	}
	var iv []byte
	if err := unmarshalAll(params.EncryptionScheme.Parameters.FullBytes, &iv); err != nil || len(iv) != aes.BlockSize {
		return nil, errors.New("invalid IV in the encrypted private key")
	}
	key, err := pbkdf2.Key(prf, string(passphrase), kdf.Salt, kdf.IterationCount, keySize)
	if err != nil {
		return nil, fmt.Errorf("failed to derive a key from the passphrase: %w", err)
	}
	data := info.EncryptedData
	if len(data) == 0 || len(data)%aes.BlockSize != 0 {
		return nil, ErrWrongPassphrase
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, data)
	padding := int(plain[len(plain)-1])
	if padding == 0 || padding > aes.BlockSize ||
		!bytes.Equal(plain[len(plain)-padding:], bytes.Repeat([]byte{byte(padding)}, padding)) {
		return nil, ErrWrongPassphrase
	}
	return plain[:len(plain)-padding], nil
}

// unmarshalAll parses DER data into v, and fails on trailing data
func unmarshalAll(data []byte, v any) error {
	rest, err := asn1.Unmarshal(data, v)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return errors.New("trailing data")
	}
	return nil
}
