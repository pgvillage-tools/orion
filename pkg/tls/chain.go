package tls

import (
	"crypto/x509"
	"fmt"
)

// Chain can hold all configuration for a chain.
type Chain struct {
	Root          Pair          `json:"root"`
	Intermediates Intermediates `json:"intermediates"`
	// Path where all files are stored
	Store string `json:"store"`
	Keys  Key    `json:"keys"`
}

// Key represents a pair of private and public key used to encrypt and decrypt
// the private keys belonging to the certificates
type Key struct {
	// To decrypt
	PrivateKey string `json:"private"`
	// To encrypt
	PublicKey string
}

// InitializeCA can be used to generate, build and save the CA cert and private
// key
func (c *Chain) InitializeCA() error {
	c.Root.Cert.SetDefaults(
		DefaultSubject,
		DefaultExpiry,
		DefaultKeyUsage,
		DefaultExtendedKeyUsages,
	)
	// Enable cert sign for intermediates
	c.Root.Cert.KeyUsage |= x509.KeyUsageCertSign
	c.Root.Cert.IsCa = true
	c.Root.Cert.AlternateNames = nil
	if err := c.Root.Generate(); err != nil {
		return err
	} else if err := c.Root.Sign(c.Root); err != nil {
		return err
	} else if err := c.Root.Encode(); err != nil {
		return err
	}
	return c.Root.Save()
}

// InitializeIntermediates can be used to initialize all intermediates
// belonging to this chain
func (c *Chain) InitializeIntermediates() (err error) {
	c.Intermediates, err = c.Intermediates.Initialize(c.Root)
	return err
}

// ChainStructure is a type that will be returned by the chain.Structure method
type ChainStructure struct {
	Certs map[string]map[string]string `json:"certs"`
	Keys  map[string]map[string]string `json:"private_keys"`
}

// Structure will convert a chain into a structure that is easy convertible to
// YAML
func (c *Chain) Structure() ChainStructure {
	structure := ChainStructure{
		Certs: map[string]map[string]string{},
		Keys:  map[string]map[string]string{},
	}
	for iName, intermediate := range c.Intermediates {
		certs := map[string]string{
			"chain": fmt.Sprintf(
				"%s%s",
				intermediate.Cert.Cert.PEM,
				c.Root.Cert.PEM,
			),
		}
		keys := map[string]string{}
		for cName, child := range intermediate.children {
			certs[cName] = string(child.Cert.PEM)
			keys[cName] = string(child.PrivateKey.PEM)
		}
		structure.Certs[iName] = certs
		structure.Keys[iName] = keys
	}
	return structure
}
