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
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"time"
)

var (
	reHostName = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
	reFQDN     = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	rePgUser   = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_$%]{0,62}$`)
	reToken    = regexp.MustCompile(`^[a-zA-Z0-9\-._~+/]+=*$`)
)

// Intermediates holds all intermediates that are configured
type Intermediates map[string]Intermediate

// Initialize generates, signs and saves the certificates and private keys of all intermediates, and of all their
// servers and clients
func (i Intermediates) Initialize(signer Pair) (Intermediates, error) {
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

// Intermediate holds the config of an intermediate, which can sign server and client certificates. Use separate
// intermediates for servers and clients, and a separate client intermediate per service: a server that trusts a
// client intermediate trusts all client certificates it signed.
type Intermediate struct {
	Cert     Pair     `json:"cert"`
	Servers  Servers  `json:"servers"`
	Clients  []string `json:"clients"`
	children Pairs
	// LeafExpiry is the validity of the server and client certificates (DefaultExpiry when not set)
	LeafExpiry time.Duration `json:"leaf_expiry"`
	// Store is the directory for the files of the server and client certificates. When set, files that have no
	// path get <Store>/<name>.crt and <Store>/<name>.key.
	Store string `json:"store"`
}

// InitializeIntermediate generates, signs and saves the intermediate
func (i *Intermediate) InitializeIntermediate(name string, signer Pair) error {
	if signer.Cert.Subject == nil || signer.Cert.X509() == nil {
		return errors.New("signer not properly initialized")
	}
	i.Cert.Cert.SetDefaults(
		signer.Cert.Subject.SetCommonName(name),
		DefaultIntermediateExpiry,
		DefaultCAKeyUsage,
		nil,
	)
	i.Cert.Cert.IsCa = true
	i.Cert.Cert.MaxPathLen = 0
	i.Cert.Cert.AlternateNames = nil
	if i.Cert.PrivateKey.Algorithm == "" {
		i.Cert.PrivateKey.Algorithm = signer.PrivateKey.Algorithm
	}
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

// IsValidClient returns true when input can be used as the name of a client (a PostgreSQL user or a token)
func IsValidClient(input string) bool {
	if rePgUser.MatchString(input) {
		return true
	}
	return reToken.MatchString(input)
}

func (i *Intermediate) newChild(name string) (Pair, error) {
	if i.children == nil {
		i.children = Pairs{}
	}
	if _, exists := i.children[name]; exists {
		return Pair{}, fmt.Errorf("cannot define %s multiple times", name)
	}
	if i.Cert.Cert.X509() == nil {
		return Pair{}, errors.New("intermediate needs to be initialized first")
	}
	expiry := i.LeafExpiry
	if expiry == 0 {
		expiry = DefaultExpiry
	}
	subject := i.Cert.Cert.Subject.SetCommonName(name)
	pair := Pair{
		Cert: Cert{
			Subject:  &subject,
			Expiry:   expiry,
			KeyUsage: DefaultKeyUsage,
		},
		PrivateKey: PrivateKey{Algorithm: i.Cert.PrivateKey.Algorithm},
	}
	if i.Store != "" {
		pair.Cert.Path = filepath.Join(i.Store, name+".crt")
		pair.PrivateKey.Path = filepath.Join(i.Store, name+".key")
	}
	return pair, nil
}

// InitializeServers generates, signs and saves the certificates and private keys of all servers of the
// intermediate. The name of a server is its CommonName, and is also added as an alternate name.
func (i *Intermediate) InitializeServers() error {
	for server, addresses := range i.Servers {
		for _, a := range append([]string{server}, addresses...) {
			if !isValidServer(a) {
				return fmt.Errorf("invalid server or address: %s", a)
			}
		}
		pair, err := i.newChild(server)
		if err != nil {
			return err
		}
		pair.Cert.ExtKeyUsage = DefaultServerExtKeyUsages
		pair.Cert.AlternateNames = uniqueStrings(append([]string{server}, addresses...))
		if err = pair.Process(i.Cert); err != nil {
			return err
		}
		i.children[server] = pair
	}
	return nil
}

// InitializeClients generates, signs and saves the certificates and private keys of all clients of the
// intermediate. The name of a client is its CommonName.
func (i *Intermediate) InitializeClients() error {
	for _, client := range i.Clients {
		if !IsValidClient(client) {
			return fmt.Errorf("client is not valid: %s", client)
		}
		pair, err := i.newChild(client)
		if err != nil {
			return err
		}
		pair.Cert.ExtKeyUsage = DefaultClientExtKeyUsages
		if err = pair.Process(i.Cert); err != nil {
			return err
		}
		i.children[client] = pair
	}
	return nil
}

// Children returns the server and client certificates (and private keys) of the intermediate
func (i Intermediate) Children() Pairs {
	return i.children
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, v := range values {
		if seen[v] {
			continue
		}
		seen[v] = true
		result = append(result, v)
	}
	return result
}

// Servers is a map holding servers, with addresses. The key will be used for the CommonName
type Servers map[string]ServerAddresses

// ServerAddresses is a list of DNS names and/or ip addresses to be used in the SAN field
type ServerAddresses []string
