package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path"
)

// PrivateKey can hold all information regarding a private key
type PrivateKey struct {
	key   *rsa.PrivateKey
	PEM   []byte `json:"pem"`
	Path  string `json:"path"`
	dirty bool
}

// Generate is a method that can generate a Private key.
func (pk *PrivateKey) Generate() error {
	if pk.key != nil {
		return nil
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %v", err)
	}
	pk.key = priv
	pk.PEM = nil
	return nil
}

// Encode will encode the rsa.PrivateKey to a PEM byte array and store it in the
// PEM field
func (pk *PrivateKey) Encode() error {
	if pk.PEM != nil {
		return nil
	}
	pk.PEM = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(pk.key),
	})
	pk.dirty = true
	return nil
}

// Save can be used to save a Private Key PEM to disk
func (pk *PrivateKey) Save() error {
	if !pk.dirty || pk.Path == "" || len(pk.PEM) == 0 {
		return nil
	}
	dir := path.Dir(pk.Path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create path %s: %v", dir, err)
	}
	if err := os.WriteFile(pk.Path, pk.PEM, 0600); err != nil {
		return fmt.Errorf("failed to write key file: %v", err)
	}
	pk.dirty = false
	return nil
}

// PublicKey will return the public key belonging to the private key.
// PublicKey raises an error when the Private key is not properly initialized
func (pk PrivateKey) PublicKey() (rsa.PublicKey, error) {
	if pk.key == nil {
		return rsa.PublicKey{}, errors.New(
			"can't get public key from uninitialized private key")
	}
	return pk.key.PublicKey, nil
}
