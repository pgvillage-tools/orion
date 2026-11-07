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

import "crypto/x509"

// Pairs is a collection of `certificate and private key` pairs
type Pairs map[string]Pair

// Generate generates a cert and private key for all pairs.
// We use copy on write and return the copy.
func (p Pairs) Generate() (Pairs, error) {
	for name, pair := range p {
		if err := pair.Generate(); err != nil {
			return p, err
		}
		p[name] = pair
	}
	return p, nil
}

// Sign signs the certs of all pairs
func (p Pairs) Sign(signer Pair) (Pairs, error) {
	for name, pair := range p {
		if err := pair.Sign(signer); err != nil {
			return p, err
		}
		p[name] = pair
	}
	return p, nil
}

// Encode encodes the private keys of all pairs into a PEM
func (p Pairs) Encode() (Pairs, error) {
	for name, pair := range p {
		if err := pair.Encode(); err != nil {
			return p, err
		}
		p[name] = pair
	}
	return p, nil
}

// Save stores the certs and private keys of all pairs in files
func (p Pairs) Save() (Pairs, error) {
	for name, pair := range p {
		if err := pair.Save(); err != nil {
			return p, err
		}
		p[name] = pair
	}
	return p, nil
}

// A Pair is a combination of a cert and the private key that belongs to the cert
type Pair struct {
	Cert       Cert       `json:"cert"`
	PrivateKey PrivateKey `json:"private_key"`
}

// Process does all that is required for a pair: generate, sign, encode and save
func (p *Pair) Process(signer Pair) error {
	if err := p.Generate(); err != nil {
		return err
	}
	if err := p.Sign(signer); err != nil {
		return err
	}
	if err := p.Encode(); err != nil {
		return err
	}
	return p.Save()
}

// SelfSign does all that is required for a root: generate, sign with its own key, encode and save
func (p *Pair) SelfSign() error {
	if err := p.Generate(); err != nil {
		return err
	}
	if err := p.Sign(*p); err != nil {
		return err
	}
	if err := p.Encode(); err != nil {
		return err
	}
	return p.Save()
}

// Generate generates a private key and a cert. RSA keys of server and client certificates also get the
// KeyEncipherment key usage.
func (p *Pair) Generate() error {
	if err := p.PrivateKey.Generate(); err != nil {
		return err
	}
	if p.PrivateKey.Algorithm.IsRSA() && !p.Cert.IsCa {
		p.Cert.KeyUsage |= x509.KeyUsageKeyEncipherment
	}
	return p.Cert.Generate()
}

// Sign signs the cert
func (p *Pair) Sign(signer Pair) error {
	return p.Cert.Sign(p.PrivateKey, signer)
}

// Encode encodes the private key into a PEM
func (p *Pair) Encode() error {
	return p.PrivateKey.Encode()
}

// Save stores the cert and private key in files
func (p *Pair) Save() error {
	if err := p.PrivateKey.Save(); err != nil {
		return err
	}
	return p.Cert.Save()
}

// Load reads the cert and private key from their files
func (p *Pair) Load() error {
	if err := p.PrivateKey.Load(); err != nil {
		return err
	}
	return p.Cert.Load()
}
