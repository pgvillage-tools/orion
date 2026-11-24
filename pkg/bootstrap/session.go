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
	"bytes"
	"context"
	"crypto/hpke"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// maxResponseBody is the maximum size of a response of the api in bootstrap mode
const maxResponseBody = 1 << 20

// ErrAPIProof is returned when the api in bootstrap mode doesn't prove that it has the bootstrap secret
var ErrAPIProof = errors.New("the api didn't prove that it has the bootstrap secret: it may not be the api you " +
	"expect, or it was started with another secret")

// Session is a bootstrap on one TLS connection to the api in bootstrap mode, after the api and the cli proved that
// they have the secret
type Session struct {
	client  *http.Client
	baseURL string
	ekm     []byte
}

// NewHTTPClient returns a client for the api in bootstrap mode. It uses one TLS 1.3 connection, since the api binds the
// bootstrap to it, and it doesn't verify the self-signed certificate of the api: the proofs authenticate the api.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				// the proofs authenticate the api, bound to this TLS session
				InsecureSkipVerify: true, //nolint:gosec // see above
				MinVersion:         tls.VersionTLS13,
			},
			Proxy:               http.ProxyFromEnvironment,
			MaxConnsPerHost:     1,
			MaxIdleConnsPerHost: 1,
		},
	}
}

// Start proves to the api at baseURL (e.g. https://orion-api:8443) that the cli has secret, and checks that the api
// has it too. Use client (from NewHTTPClient) for all requests of the session.
func Start(ctx context.Context, client *http.Client, baseURL string, secret []byte) (*Session, error) {
	s := &Session{client: client, baseURL: strings.TrimSuffix(baseURL, "/")}
	var challenge endpoints.BootstrapChallengeResponse
	ekm, err := s.post(ctx, endpoints.BootstrapChallengeEndPoint, nil, &challenge)
	if err != nil {
		return nil, err
	}
	s.ekm = ekm
	cliNonce := make([]byte, NonceSize)
	if _, err = rand.Read(cliNonce); err != nil {
		return nil, err
	}
	cliProof, err := CLIProof(secret, challenge.Nonce, cliNonce, ekm)
	if err != nil {
		return nil, fmt.Errorf("invalid challenge of the api: %w", err)
	}
	var proof endpoints.BootstrapProofResponse
	if _, err = s.post(ctx, endpoints.BootstrapProofEndPoint,
		endpoints.BootstrapProofRequest{Nonce: cliNonce, Proof: cliProof}, &proof); err != nil {
		return nil, err
	}
	apiProof, err := APIProof(secret, challenge.Nonce, cliNonce, ekm)
	if err != nil {
		return nil, err
	}
	if !Equal(apiProof, proof.Proof) {
		return nil, ErrAPIProof
	}
	return s, nil
}

// Archive sends the public key of the cli (pub) and extra names for the api certificate (sans) to the api, and returns
// the certificates and the private keys of the archive, decrypted with priv (the private key of pub)
func (s *Session) Archive(ctx context.Context, priv hpke.PrivateKey, sans []string) (
	endpoints.BootstrapCertificates, ArchiveKeys, error) {
	var archive endpoints.BootstrapArchive
	if _, err := s.post(ctx, endpoints.BootstrapArchiveEndPoint,
		endpoints.BootstrapArchiveRequest{PublicKey: priv.PublicKey().Bytes(), SANs: sans}, &archive); err != nil {
		return endpoints.BootstrapCertificates{}, ArchiveKeys{}, err
	}
	keys, err := OpenArchive(priv, s.ekm, archive)
	if err != nil {
		return endpoints.BootstrapCertificates{}, ArchiveKeys{}, err
	}
	return archive.Certificates, keys, nil
}

// Stored tells the api that the cli stored the archive: the api stores its certificate and switches to normal mode
func (s *Session) Stored(ctx context.Context) error {
	var resp endpoints.BootstrapStatusResponse
	_, err := s.post(ctx, endpoints.BootstrapStoredEndPoint, nil, &resp)
	return err
}

// ProblemError is the error of a request that the api answered with a problem
type ProblemError struct {
	Endpoint endpoints.EndPoint
	Status   int
	Detail   string
}

func (e *ProblemError) Error() string {
	return fmt.Sprintf("%s: %d %s: %s", e.Endpoint, e.Status, http.StatusText(e.Status), e.Detail)
}

// post posts body (when not nil) as JSON to ep, decodes the response into out, and returns the keying material of the
// TLS session; it fails when the session isn't the one of the bootstrap
func (s *Session) post(ctx context.Context, ep endpoints.EndPoint, body, out any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	url := s.baseURL + "/" + endpoints.APIVersion + "/" + string(ep)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return nil, err
	}
	ekm, err := ExportKeyingMaterial(resp.TLS)
	if err != nil {
		return nil, err
	}
	if s.ekm != nil && !Equal(ekm, s.ekm) {
		return nil, errors.New("the connection to the api changed during the bootstrap: start again")
	}
	if resp.StatusCode != http.StatusOK {
		var problem endpoints.Problem
		_ = json.Unmarshal(data, &problem)
		detail := problem.Detail
		if detail == "" {
			detail = strings.TrimSpace(string(data))
		}
		return nil, &ProblemError{Endpoint: ep, Status: resp.StatusCode, Detail: detail}
	}
	if err = json.Unmarshal(data, out); err != nil {
		return nil, fmt.Errorf("invalid response of %s: %w", ep, err)
	}
	return ekm, nil
}
