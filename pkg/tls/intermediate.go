package tls

import (
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"regexp"
)

var (
	reHostName = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
	reFQDN     = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	rePgUser   = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_$%]{0,62}$`)
	reToken    = regexp.MustCompile(`^[a-zA-Z0-9\-._~+/]+=*$`)
)

// Intermediates holds all intermediates that are configured
type Intermediates map[string]Intermediate

// Initialize can be used to generate, build and save certificates and private
// keys for all servers and clients of all intermediates
func (i Intermediates) Initialize(
	signer Pair,
) (Intermediates, error) {
	for iName, intermediate := range i {
		if err := intermediate.InitializeIntermediate(iName, signer); err != nil {
			return nil, err
		}
		if err := intermediate.InitializeClients(); err != nil {
			return i, err
		}
		if err := intermediate.InitializeServers(); err != nil {
			return i, err
		}
		i[iName] = intermediate
	}
	return i, nil
}

// Intermediate holds the config of an intermediate, which can be either Server
// or Client (or both)
type Intermediate struct {
	Cert     Pair `json:"cert"`
	children Pairs
	Servers  Servers  `json:"servers"`
	Clients  []string `json:"clients"`
}

// InitializeIntermediate can be used to initialize the intermediate
func (i *Intermediate) InitializeIntermediate(
	name string,
	signer Pair,
) error {
	if signer.Cert.Subject == nil {
		return errors.New("signer not properly initialized")
	}
	i.Cert.Cert.SetDefaults(
		signer.Cert.Subject.SetCommonName(name),
		signer.Cert.Expiry,
		signer.Cert.KeyUsage,
		signer.Cert.ExtKeyUsage,
	)
	i.Cert.Cert.IsCa = true
	// Enable cert sign for intermediates
	i.Cert.Cert.KeyUsage |= x509.KeyUsageCertSign
	return i.Cert.Process(signer)
}

func isValidServer(input string) bool {
	if net.ParseIP(input) != nil {
		return true
	}
	if reHostName.MatchString(input) {
		return true
	}
	return reFQDN.MatchString(input)
}

// InitializeServers can be used to generate, build and save certificates and
// private keys for all servers an intermediate
func (i *Intermediate) InitializeServers() error {
	if i.children == nil {
		i.children = Pairs{}
	}
	for server, addresses := range i.Servers {
		if _, exists := i.children["server"]; exists {
			return errors.New("cannot define client multiple times")
		}
		for _, a := range append([]string{server}, addresses...) {
			if !isValidServer(a) {
				return fmt.Errorf("invalid server or address: %s", a)
			}
		}
		subject := i.Cert.Cert.Subject.SetCommonName(server)
		pair := Pair{
			Cert: Cert{
				Subject:        &subject,
				Expiry:         i.Cert.Cert.Expiry,
				KeyUsage:       i.Cert.Cert.KeyUsage,
				ExtKeyUsage:    i.Cert.Cert.ExtKeyUsage,
				AlternateNames: addresses,
			},
		}
		pair.Cert.KeyUsage &^= x509.KeyUsageCertSign
		err := pair.Process(i.Cert)
		if err != nil {
			return err
		}
		i.children[server] = pair
	}
	return nil
}

func IsValidClient(input string) bool {
	if rePgUser.MatchString(input) {
		return true
	}
	return reToken.MatchString(input)
}

// InitializeClients can be used to generate, build and save certificates and
// private keys for all clients of an intermediate
func (i *Intermediate) InitializeClients() error {
	if i.children == nil {
		i.children = Pairs{}
	}
	i.children = Pairs{}
	for _, client := range i.Clients {
		if !IsValidClient(client) {
			return fmt.Errorf("client is not valid: %s", client)
		}
		subject := i.Cert.Cert.Subject.SetCommonName(client)
		pair := Pair{
			Cert: Cert{
				Subject:     &subject,
				Expiry:      i.Cert.Cert.Expiry,
				KeyUsage:    i.Cert.Cert.KeyUsage,
				ExtKeyUsage: i.Cert.Cert.ExtKeyUsage,
			},
		}
		// disable cert-sign for certs
		pair.Cert.KeyUsage &^= x509.KeyUsageCertSign
		err := pair.Process(i.Cert)
		if err != nil {
			return err
		}
		i.children[client] = pair
	}
	return nil
}

// Servers is a map holding servers, with addresses. The key will be used for
// the CommonName
type Servers map[string]ServerAddresses

// ServerAddresses is a list of DNS names and/or ip addresses to be used in the
// SAN field
type ServerAddresses []string
