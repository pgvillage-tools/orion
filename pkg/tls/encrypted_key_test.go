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
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The files in testdata are created by OpenSSL 3:
//
//	openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out k.pem
//	openssl pkcs8 -topk8 -v2 aes-256-cbc -in k.pem -passout pass:orion-test-passphrase -out openssl-aes256-sha256.key
//	openssl pkcs8 -topk8 -v2 aes-128-cbc -v2prf hmacWithSHA1 -iter 2048 -in k.pem \
//	    -passout pass:orion-test-passphrase -out openssl-aes128-sha1.key
//	openssl pkey -in k.pem -pubout -out openssl.pub
const testPassphrase = "orion-test-passphrase"

var _ = Describe("Encrypted private keys", func() {
	var pk PrivateKey

	BeforeEach(func() {
		pk = PrivateKey{Passphrase: []byte(testPassphrase)}
		Ω(pk.Generate()).To(Succeed())
		Ω(pk.Encode()).To(Succeed())
	})

	It("encodes as an ENCRYPTED PRIVATE KEY with PBES2, PBKDF2-HMAC-SHA256 and AES-256-CBC", func() {
		block, _ := pem.Decode(pk.PEM)
		Ω(block).NotTo(BeNil())
		Ω(block.Type).To(Equal(pemTypeEncryptedPrivateKey))
		var info encryptedPrivateKeyInfo
		Ω(unmarshalAll(block.Bytes, &info)).To(Succeed())
		Ω(info.Algorithm.Algorithm).To(Equal(oidPBES2))
		var params pbes2Params
		Ω(unmarshalAll(info.Algorithm.Parameters.FullBytes, &params)).To(Succeed())
		Ω(params.EncryptionScheme.Algorithm).To(Equal(oidAES256CBC))
		var kdf pbkdf2Params
		Ω(unmarshalAll(params.KeyDerivationFunc.Parameters.FullBytes, &kdf)).To(Succeed())
		Ω(kdf.IterationCount).To(Equal(pbkdf2Iterations))
		Ω(kdf.Salt).To(HaveLen(pbkdf2SaltSize))
		Ω(kdf.PRF.Algorithm).To(Equal(oidHMACWithSHA256))
	})

	It("decodes with the passphrase into the same key", func() {
		decoded := PrivateKey{PEM: pk.PEM, Passphrase: []byte(testPassphrase)}
		Ω(decoded.Decode()).To(Succeed())
		Ω(decoded.Algorithm).To(Equal(KeyAlgorithmECDSAP256))
		want, _ := pk.PublicKey()
		got, _ := decoded.PublicKey()
		Ω(got).To(Equal(want))
	})

	It("saves and loads an encrypted key", func() {
		pk.Path = filepath.Join(GinkgoT().TempDir(), "ca.key")
		Ω(pk.Save()).To(Succeed())
		loaded := PrivateKey{Path: pk.Path, Passphrase: []byte(testPassphrase)}
		Ω(loaded.Load()).To(Succeed())
		want, _ := pk.PublicKey()
		got, _ := loaded.PublicKey()
		Ω(got).To(Equal(want))
	})

	It("uses a new salt and IV every time", func() {
		other := PrivateKey{key: pk.key, Passphrase: []byte(testPassphrase)}
		Ω(other.Encode()).To(Succeed())
		Ω(other.PEM).NotTo(Equal(pk.PEM))
	})

	It("requires a passphrase", func() {
		decoded := PrivateKey{PEM: pk.PEM}
		Ω(decoded.Decode()).To(MatchError(ErrPassphraseRequired))
	})

	It("refuses a wrong passphrase", func() {
		decoded := PrivateKey{PEM: pk.PEM, Passphrase: []byte("wrong")}
		Ω(decoded.Decode()).To(MatchError(ErrWrongPassphrase))
	})

	It("refuses a wrong passphrase that ends in valid padding", func() {
		// what a wrong passphrase decrypts to, now and then: random data with valid padding, so no key
		der, err := encryptPKCS8([]byte("not a private key"), []byte(testPassphrase))
		Ω(err).NotTo(HaveOccurred())
		decoded := PrivateKey{
			PEM:        pem.EncodeToMemory(&pem.Block{Type: pemTypeEncryptedPrivateKey, Bytes: der}),
			Passphrase: []byte(testPassphrase),
		}
		Ω(decoded.Decode()).To(MatchError(ErrWrongPassphrase))
	})

	It("ignores the passphrase for an unencrypted key", func() {
		plain := PrivateKey{key: pk.key}
		Ω(plain.Encode()).To(Succeed())
		decoded := PrivateKey{PEM: plain.PEM, Passphrase: []byte(testPassphrase)}
		Ω(decoded.Decode()).To(Succeed())
	})

	DescribeTable("decodes keys encrypted by OpenSSL",
		func(file string) {
			data, err := os.ReadFile(filepath.Join("testdata", file))
			Ω(err).NotTo(HaveOccurred())
			decoded := PrivateKey{PEM: data, Passphrase: []byte(testPassphrase)}
			Ω(decoded.Decode()).To(Succeed())

			pubPEM, err := os.ReadFile(filepath.Join("testdata", "openssl.pub"))
			Ω(err).NotTo(HaveOccurred())
			block, _ := pem.Decode(pubPEM)
			want, err := x509.ParsePKIXPublicKey(block.Bytes)
			Ω(err).NotTo(HaveOccurred())
			got, _ := decoded.PublicKey()
			Ω(got).To(Equal(want))
		},
		Entry("AES-256-CBC with HMAC-SHA256 (the OpenSSL 3 default)", "openssl-aes256-sha256.key"),
		Entry("AES-128-CBC with HMAC-SHA1", "openssl-aes128-sha1.key"),
	)

	DescribeTable("refuses broken encrypted keys",
		func(der []byte, msg string) {
			_, err := decryptPKCS8(der, []byte(testPassphrase))
			Ω(err).To(MatchError(ContainSubstring(msg)))
		},
		Entry("garbage", []byte("garbage"), "failed to parse the encrypted private key"),
		Entry("not PBES2", mustEncryptedInfo(oidPBKDF2, nil), "only PBES2"),
		Entry("broken PBES2 parameters", mustEncryptedInfo(oidPBES2, []byte{0x30, 0x00}),
			"failed to parse the PBES2 parameters"),
		Entry("not PBKDF2", mustPBES2(oidPBES2, oidHMACWithSHA256, oidAES256CBC, 16, 16),
			"only PBKDF2"),
		Entry("an unsupported hash", mustPBES2(oidPBKDF2, oidAES256CBC, oidAES256CBC, 16, 16),
			"unsupported PBKDF2 hash"),
		Entry("an unsupported cipher", mustPBES2(oidPBKDF2, oidHMACWithSHA256, oidHMACWithSHA256, 16, 16),
			"only AES-CBC"),
		Entry("a short IV", mustPBES2(oidPBKDF2, oidHMACWithSHA256, oidAES256CBC, 8, 16), "invalid IV"),
		Entry("data that isn't a multiple of the block size", mustPBES2(oidPBKDF2, oidHMACWithSHA256, oidAES256CBC, 16, 15),
			"wrong passphrase"),
	)
})

// mustEncryptedInfo returns an EncryptedPrivateKeyInfo with another algorithm
func mustEncryptedInfo(oid asn1.ObjectIdentifier, params []byte) []byte {
	info := encryptedPrivateKeyInfo{EncryptedData: []byte{1}}
	info.Algorithm.Algorithm = oid
	if params != nil {
		info.Algorithm.Parameters.FullBytes = params
	}
	der, err := asn1.Marshal(info)
	if err != nil {
		panic(err)
	}
	return der
}

// mustPBES2 returns an EncryptedPrivateKeyInfo with PBES2, the kdf, prf and cipher, an IV of ivSize and dataSize bytes
// of encrypted data
func mustPBES2(kdf, prf, cipher asn1.ObjectIdentifier, ivSize, dataSize int) []byte {
	must := func(der []byte, err error) []byte {
		if err != nil {
			panic(err)
		}
		return der
	}
	kdfParams := must(asn1.Marshal(pbkdf2Params{
		Salt:           make([]byte, pbkdf2SaltSize),
		IterationCount: 1,
		PRF:            pkix.AlgorithmIdentifier{Algorithm: prf, Parameters: asn1.NullRawValue},
	}))
	params := must(asn1.Marshal(pbes2Params{
		KeyDerivationFunc: pkix.AlgorithmIdentifier{Algorithm: kdf, Parameters: asn1.RawValue{FullBytes: kdfParams}},
		EncryptionScheme: pkix.AlgorithmIdentifier{
			Algorithm:  cipher,
			Parameters: asn1.RawValue{FullBytes: must(asn1.Marshal(make([]byte, ivSize)))},
		},
	}))
	return must(asn1.Marshal(encryptedPrivateKeyInfo{
		Algorithm:     pkix.AlgorithmIdentifier{Algorithm: oidPBES2, Parameters: asn1.RawValue{FullBytes: params}},
		EncryptedData: make([]byte, dataSize),
	}))
}
