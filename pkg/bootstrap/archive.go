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
	"crypto/hpke"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// The archive: the api encrypts the private keys of the CA and of the cli certificate to the public key of the cli,
// with HPKE (RFC 9180) in base mode. See docs/api/bootstrap-design.md, "The archive".

const (
	// archiveInfo is the HPKE info of the archive: it binds the keys to their purpose and the version of the format
	archiveInfo = "orion-bootstrap archive v1"
	// archiveAADLabel starts the additional data of the archive
	archiveAADLabel = "orion-bootstrap archive"
)

// ArchiveKeys are the private keys of the archive, PEM encoded (PKCS#8, unencrypted: the archive encrypts them)
type ArchiveKeys struct {
	Root                  []byte `json:"root"`
	ServerIntermediate    []byte `json:"serverIntermediate"`
	APIClientIntermediate []byte `json:"apiClientIntermediate"`
	CLIClient             []byte `json:"cliClient"`
}

// ErrArchive is returned when the archive can't be decrypted: it isn't for the key pair of the cli, it belongs to
// another TLS connection, or its certificates were changed
var ErrArchive = errors.New("can't decrypt the bootstrap archive: it isn't for this key pair, or it was changed")

// archiveSuite returns the KDF and the AEAD of the archive (the KEM is the one of the key pair, KEM())
func archiveSuite() (hpke.KDF, hpke.AEAD) {
	return hpke.HKDFSHA256(), hpke.AES256GCM()
}

// archiveAAD returns the additional data of the archive: the label, the keying material of the TLS connection (so
// the archive can't be used on another connection) and a hash of the certificates (so they can't be changed)
func archiveAAD(ekm []byte, certs endpoints.BootstrapCertificates) ([]byte, error) {
	if len(ekm) != ExporterLength {
		return nil, ErrSize
	}
	h := sha256.New()
	for _, pemData := range [][]byte{certs.Root, certs.ServerIntermediate, certs.APIClientIntermediate,
		certs.APIServer, certs.CLIClient} {
		// each certificate with its length first, so the input is unambiguous
		_ = binary.Write(h, binary.BigEndian, uint64(len(pemData)))
		h.Write(pemData)
	}
	aad := append([]byte(archiveAADLabel), 0)
	aad = append(aad, ekm...)
	return h.Sum(aad), nil
}

// SealArchive returns the archive with certs, and keys encrypted to pub, bound to the TLS connection with keying
// material ekm (ExportKeyingMaterial)
func SealArchive(pub hpke.PublicKey, ekm []byte, certs endpoints.BootstrapCertificates, keys ArchiveKeys) (
	endpoints.BootstrapArchive, error) {
	if pub.KEM().ID() != KEM().ID() {
		return endpoints.BootstrapArchive{}, fmt.Errorf("unsupported KEM %s for the bootstrap key (only %s)",
			kemID(pub.KEM()), kemID(KEM()))
	}
	aad, err := archiveAAD(ekm, certs)
	if err != nil {
		return endpoints.BootstrapArchive{}, err
	}
	plain, err := json.Marshal(keys)
	if err != nil {
		return endpoints.BootstrapArchive{}, err
	}
	kdf, aead := archiveSuite()
	enc, sender, err := hpke.NewSender(pub, kdf, aead, []byte(archiveInfo))
	if err != nil {
		return endpoints.BootstrapArchive{}, err
	}
	sealed, err := sender.Seal(aad, plain)
	if err != nil {
		return endpoints.BootstrapArchive{}, err
	}
	return endpoints.BootstrapArchive{Certificates: certs, Enc: enc, SealedKeys: sealed}, nil
}

// OpenArchive decrypts the keys of archive with priv, for the TLS connection with keying material ekm
func OpenArchive(priv hpke.PrivateKey, ekm []byte, archive endpoints.BootstrapArchive) (ArchiveKeys, error) {
	aad, err := archiveAAD(ekm, archive.Certificates)
	if err != nil {
		return ArchiveKeys{}, err
	}
	kdf, aead := archiveSuite()
	recipient, err := hpke.NewRecipient(archive.Enc, priv, kdf, aead, []byte(archiveInfo))
	if err != nil {
		return ArchiveKeys{}, fmt.Errorf("%w: %v", ErrArchive, err)
	}
	plain, err := recipient.Open(aad, archive.SealedKeys)
	if err != nil {
		return ArchiveKeys{}, ErrArchive
	}
	var keys ArchiveKeys
	if err = json.Unmarshal(plain, &keys); err != nil {
		return ArchiveKeys{}, fmt.Errorf("invalid keys in the bootstrap archive: %w", err)
	}
	return keys, nil
}
