package tls

// Pairs is a collection of `certificate and private key` pairs
type Pairs map[string]Pair

// Generate will generate a cert and private key.
// We use copy on write and return  the copy
func (p Pairs) Generate() (Pairs, error) {
	for name, pair := range p {
		err := pair.Generate()
		if err != nil {
			return p, err
		}
		p[name] = pair
	}
	return p, nil
}

// Sign will sign a cert
func (p Pairs) Sign(signer Pair) (Pairs, error) {
	for name, pair := range p {
		err := pair.Sign(signer)
		if err != nil {
			return p, err
		}
		p[name] = pair
	}
	return p, nil
}

// Encode will encode the Private Key into a PEM
func (p Pairs) Encode() (Pairs, error) {
	for name, pair := range p {
		err := pair.Encode()
		if err != nil {
			return p, err
		}
		p[name] = pair
	}
	return p, nil
}

// Save will store the cert and private key in files
func (p Pairs) Save() (Pairs, error) {
	for name, pair := range p {
		err := pair.Save()
		if err != nil {
			return p, err
		}
		p[name] = pair
	}
	return p, nil
}

// A Pair is a combination of a cert and the Private key that belongs to the
// cert
type Pair struct {
	Cert       Cert       `json:"cert"`
	PrivateKey PrivateKey `json:"private_key"`
}

// Process will do all that is required for a pair, e.a. generate, sign,
// encode and save
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

// Generate will generate a cert and private key
func (p *Pair) Generate() error {
	if err := p.PrivateKey.Generate(); err != nil {
		return nil
	}
	if err := p.Cert.Generate(); err != nil {
		return err
	}
	return nil
}

// Sign will sign a cert
func (p *Pair) Sign(signer Pair) error {
	return p.Cert.Sign(p.PrivateKey, signer)
}

// Encode will encode the Private Key into a PEM
func (p *Pair) Encode() error {
	return p.PrivateKey.Encode()
}

// Save will store the cert and private key in files
func (p *Pair) Save() error {
	if err := p.PrivateKey.Save(); err != nil {
		return nil
	}
	return p.Cert.Save()
}
