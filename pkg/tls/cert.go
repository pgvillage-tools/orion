// Package tls takes care of all tls actions for a chain
package tls

import (
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path"
	"regexp"
	"time"
)

// Certs is a collection of Cert objects
type Certs []Cert

// Cert is an object representing a certificate
type Cert struct {
	cert           *x509.Certificate
	Subject        *Subject           `json:"subject"`
	Expiry         time.Duration      `json:"expiry"`
	KeyUsage       x509.KeyUsage      `json:"key_usage"`
	ExtKeyUsage    []x509.ExtKeyUsage `json:"extended_key_usage"`
	IsCa           bool               `json:"is_ca"`
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

func splitAlternateNames(alternateNames []string) (
	*altNames,
	error,
) {
	subjectAltNames := &altNames{}
	mailRE := regexp.MustCompile(
		`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	dnsRE := regexp.MustCompile(
		`^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]{0,61}[a-zA-Z0-9])(\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]{0,61}[a-zA-Z0-9]))*$`)
	for _, alternateName := range alternateNames {
		if ip := net.ParseIP(alternateName); ip != nil {
			subjectAltNames.ipAddresses = append(subjectAltNames.ipAddresses, ip)
		} else if mailRE.Match([]byte(alternateName)) {
			subjectAltNames.eMailAddresses = append(
				subjectAltNames.eMailAddresses, alternateName)
		} else if dnsRE.Match([]byte(alternateName)) {
			subjectAltNames.dnsNames = append(
				subjectAltNames.dnsNames, alternateName)
		} else if uri, err := url.Parse(alternateName); err == nil {
			subjectAltNames.uris = append(subjectAltNames.uris, uri)
		} else {
			return nil, fmt.Errorf(
				"%s is not a known format for a dns name, email address, ip address or uri",
				alternateName,
			)
		}
	}
	return subjectAltNames, nil
}

// SetDefaults will set default values when none is set
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
	if c.Expiry < 24*time.Hour {
		c.Expiry = defaultExpiry
	}
	if c.Subject == nil {
		c.Subject = &defaultSubject
	}
}

// Generate will generate a Certificate which still needs to be signed (a CSR)
func (c *Cert) Generate() error {
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1),
		128))
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %v", err)
	}

	altNames, err := splitAlternateNames(c.AlternateNames)
	if err != nil {
		return err
	}

	now := time.Now()
	var maxPathLen int
	if c.IsCa {
		maxPathLen = 1
	}
	c.cert = &x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               c.Subject.AsPkixName(),
		NotBefore:             now,
		NotAfter:              now.Add(c.Expiry),
		KeyUsage:              c.KeyUsage,
		ExtKeyUsage:           c.ExtKeyUsage,
		BasicConstraintsValid: true,
		IsCA:                  c.IsCa,
		MaxPathLen:            maxPathLen,
		DNSNames:              altNames.dnsNames,
		EmailAddresses:        altNames.eMailAddresses,
		IPAddresses:           altNames.ipAddresses,
		URIs:                  altNames.uris,
	}
	c.PEM = nil
	return nil
}

// Sign can be used to sign the cert (and will write to the PEM byte array)
func (c *Cert) Sign(privateKey PrivateKey, signer Pair) error {
	pubKey, err := privateKey.PublicKey()
	if err != nil {
		return err
	}
	certDER, err := x509.CreateCertificate(rand.Reader, c.cert, signer.Cert.cert,
		&pubKey, signer.PrivateKey.key)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %v", err)
	}

	c.PEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	c.dirty = true
	return nil
}

// Save can be used to save a Cert to disk
func (c *Cert) Save() error {
	if !c.dirty || c.Path == "" || len(c.PEM) == 0 {
		return nil
	}
	dir := path.Dir(c.Path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create path %s: %v", dir, err)
	}
	if err := os.WriteFile(c.Path, c.PEM, 0600); err != nil {
		return fmt.Errorf("failed to write key file: %v", err)
	}
	c.dirty = false
	return nil
}
