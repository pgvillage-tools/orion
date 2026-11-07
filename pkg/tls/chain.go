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
	"path/filepath"
)

const (
	rootName = "root"
	caName   = "ca"
)

// Chain holds a root CA and its intermediates
type Chain struct {
	Root          Pair          `json:"root"`
	Intermediates Intermediates `json:"intermediates"`
	// Store is the directory where all files are stored. When set, files that have no path get one:
	//   - <Store>/root.crt and root.key for the root,
	//   - <Store>/<intermediate>/ca.crt and ca.key for an intermediate,
	//   - <Store>/<intermediate>/<name>.crt and <name>.key for a server or client.
	Store string `json:"store"`
	// KeyAlgorithm is used for all keys that have no algorithm set (DefaultKeyAlgorithm when not set)
	KeyAlgorithm KeyAlgorithm `json:"key_algorithm"`
}

func setPaths(pair *Pair, dir, name string) {
	if pair.Cert.Path == "" {
		pair.Cert.Path = filepath.Join(dir, name+".crt")
	}
	if pair.PrivateKey.Path == "" {
		pair.PrivateKey.Path = filepath.Join(dir, name+".key")
	}
}

// InitializeCA generates, signs and saves the root CA cert and private key
func (c *Chain) InitializeCA() error {
	c.Root.Cert.SetDefaults(
		DefaultSubject,
		DefaultRootExpiry,
		DefaultCAKeyUsage,
		nil,
	)
	c.Root.Cert.IsCa = true
	c.Root.Cert.MaxPathLen = rootMaxPathLen
	c.Root.Cert.AlternateNames = nil
	if c.Root.PrivateKey.Algorithm == "" {
		c.Root.PrivateKey.Algorithm = c.KeyAlgorithm
	}
	if c.Store != "" {
		setPaths(&c.Root, c.Store, rootName)
	}
	return c.Root.SelfSign()
}

// InitializeIntermediates initializes all intermediates belonging to this chain
func (c *Chain) InitializeIntermediates() (err error) {
	if c.Store != "" {
		for name, intermediate := range c.Intermediates {
			dir := filepath.Join(c.Store, name)
			setPaths(&intermediate.Cert, dir, caName)
			if intermediate.Store == "" {
				intermediate.Store = dir
			}
			c.Intermediates[name] = intermediate
		}
	}
	c.Intermediates, err = c.Intermediates.Initialize(c.Root)
	return err
}

// ChainStructure is a type that will be returned by the chain.Structure method
type ChainStructure struct {
	// Chains holds the chain (intermediate and root) per intermediate
	Chains map[string]string `json:"chains"`
	// Certs holds the server and client certificates per intermediate
	Certs map[string]map[string]string `json:"certs"`
	// Keys holds the private keys of the server and client certificates per intermediate
	Keys map[string]map[string]string `json:"private_keys"`
}

// Structure converts a chain into a structure that is easy to convert to YAML
func (c *Chain) Structure() ChainStructure {
	structure := ChainStructure{
		Chains: map[string]string{},
		Certs:  map[string]map[string]string{},
		Keys:   map[string]map[string]string{},
	}
	for iName, intermediate := range c.Intermediates {
		structure.Chains[iName] = string(intermediate.Cert.Cert.PEM) + string(c.Root.Cert.PEM)
		certs := map[string]string{}
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
