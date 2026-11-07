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

// Package tls creates and manages a CA: a root, intermediates, and server and client certificates
package tls

import (
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"regexp"
	"time"
)

const (
	pemTypeCertificate = "CERTIFICATE"
	serialNumberBits   = 128
)

var (
	reMail = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	reDNS  = regexp.MustCompile(
		`^(\*\.)?([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]{0,61}[a-zA-Z0-9])` +
			`(\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]{0,61}[a-zA-Z0-9]))*$`)
)

// Certs is a collection of Cert objects
type Certs []Cert

// Cert is an object representing a certificate
type Cert struct {
	// cert is the signed certificate, or the template before it is signed
	cert           *x509.Certificate
	signed         bool
	Subject        *Subject           `json:"subject"`
	Expiry         time.Duration      `json:"expiry"`
	KeyUsage       x509.KeyUsage      `json:"key_usage"`
	ExtKeyUsage    []x509.ExtKeyUsage `json:"extended_key_usage"`
	IsCa           bool               `json:"is_ca"`
	MaxPathLen     int                `json:"max_path_len"`
	AlternateNames []string           `json:"subject_alternate_names"`
	PEM            []byte             `json:"pem"`
	Path           string             `json:"path"`
	dirty          bool
}

type altNames struct {
	dnsNames       []string
	eMailAddresses []string
	ipAddresses    []net.IP
	uris           []*url.URL
}

func parseURI(alternateName string) (*url.URL, bool) {
	uri, err := url.Parse(alternateName)
	if err != nil || uri.Scheme == "" || (uri.Host == "" && uri.Opaque == "") {
		return nil, false
	}
	return uri, true
}

func splitAlternateNames(alternateNames []string) (*altNames, error) {
	subjectAltNames := &altNames{}
	for _, alternateName := range alternateNames {
		if ip := net.ParseIP(alternateName); ip != nil {
			subjectAltNames.ipAddresses = append(subjectAltNames.ipAddresses, ip)
		} else if reMail.MatchString(alternateName) {
			subjectAltNames.eMailAddresses = append(subjectAltNames.eMailAddresses, alternateName)
		} else if reDNS.MatchString(alternateName) {
			subjectAltNames.dnsNames = append(subjectAltNames.dnsNames, alternateName)
		} else if uri, ok := parseURI(alternateName); ok {
			subjectAltNames.uris = append(subjectAltNames.uris, uri)
		} else {
			return nil, fmt.Errorf("%s is not a known format for a dns name, email address, ip address or uri",
				alternateName)
		}
	}
	return subjectAltNames, nil
}

// SetDefaults sets default values for the fields that are not set
func (c *Cert) SetDefaults(
	defaultSubject Subject,
	defaultExpiry time.Duration,
	defaultKeyUsage x509.KeyUsage,
	defaultExtKeyUsage []x509.ExtKeyUsage,
) {
	if c.KeyUsage == x509.KeyUsage(0) {
		c.KeyUsage = defaultKeyUsage
	}
	if len(c.ExtKeyUsage) == 0 {
		c.ExtKeyUsage = defaultExtKeyUsage
	}
	if c.Expiry == 0 {
		c.Expiry = defaultExpiry
	}
	if c.Subject == nil {
		subject := defaultSubject
		c.Subject = &subject
	}
}

// Generate generates the template of the certificate, which still needs to be signed
func (c *Cert) Generate() error {
	if c.Subject == nil {
		return errors.New("can't generate a certificate without a subject")
	}
	if c.Expiry <= 0 {
		return fmt.Errorf("invalid expiry %s for certificate %s", c.Expiry, c.Subject.CommonName)
	}
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), serialNumberBits))
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %w", err)
	}

	sans, err := splitAlternateNames(c.AlternateNames)
	if err != nil {
		return err
	}

	now := time.Now()
	c.cert = &x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               c.Subject.AsPkixName(),
		NotBefore:             now.Add(-ClockSkew),
		NotAfter:              now.Add(c.Expiry),
		KeyUsage:              c.KeyUsage,
		ExtKeyUsage:           c.ExtKeyUsage,
		BasicConstraintsValid: true,
		IsCA:                  c.IsCa,
		DNSNames:              sans.dnsNames,
		EmailAddresses:        sans.eMailAddresses,
		IPAddresses:           sans.ipAddresses,
		URIs:                  sans.uris,
	}
	if c.IsCa {
		c.cert.MaxPathLen = c.MaxPathLen
		c.cert.MaxPathLenZero = c.MaxPathLen == 0
	}
	c.signed = false
	c.PEM = nil
	return nil
}

// Sign signs the certificate with the private key of the signer, and stores it in the PEM field. The certificate
// never outlives the signer: its expiry is capped at the expiry of the signer. A root signs itself, with its own pair
// as the signer.
func (c *Cert) Sign(privateKey PrivateKey, signer Pair) error {
	if c.cert == nil {
		return errors.New("certificate needs to be generated before it can be signed")
	}
	if signer.PrivateKey.key == nil || signer.Cert.cert == nil {
		return errors.New("signer not properly initialized")
	}
	pubKey, err := privateKey.PublicKey()
	if err != nil {
		return err
	}
	parent := signer.Cert.cert
	selfSigned := parent == c.cert
	if !selfSigned {
		if !signer.Cert.signed {
			return errors.New("signer needs to be signed before it can sign other certificates")
		}
		if c.cert.NotAfter.After(parent.NotAfter) {
			c.cert.NotAfter = parent.NotAfter
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, c.cert, parent, pubKey, signer.PrivateKey.key)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}
	signed, err := x509.ParseCertificate(der)
	if err != nil {
		return fmt.Errorf("failed to parse created certificate: %w", err)
	}
	c.cert = signed
	c.signed = true
	c.PEM = pem.EncodeToMemory(&pem.Block{Type: pemTypeCertificate, Bytes: der})
	c.dirty = true
	return nil
}

// Decode parses the (first certificate in the) PEM field
func (c *Cert) Decode() error {
	block, _ := pem.Decode(c.PEM)
	if block == nil || block.Type != pemTypeCertificate {
		return errors.New("no PEM encoded certificate found")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}
	c.cert = cert
	c.signed = true
	c.IsCa = cert.IsCA
	c.MaxPathLen = cert.MaxPathLen
	return nil
}

// Load reads the certificate from Path
func (c *Cert) Load() error {
	data, err := os.ReadFile(c.Path)
	if err != nil {
		return fmt.Errorf("failed to read certificate: %w", err)
	}
	c.PEM = data
	c.dirty = false
	return c.Decode()
}

// Save writes the certificate PEM to Path
func (c *Cert) Save() error {
	if !c.dirty || c.Path == "" || len(c.PEM) == 0 {
		return nil
	}
	if err := writeFile(c.Path, c.PEM, publicFileMode); err != nil {
		return fmt.Errorf("failed to write certificate: %w", err)
	}
	c.dirty = false
	return nil
}

// X509 returns the signed certificate (nil when it is not signed yet)
func (c Cert) X509() *x509.Certificate {
	if !c.signed {
		return nil
	}
	return c.cert
}
