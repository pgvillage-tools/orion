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

package api_client

import (
	"encoding/json"
	"net/http"

	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// RequestAPICertificate makes the api create a new private key, and returns its certificate request
func (c Connection) RequestAPICertificate(req endpoints.APICertificateRequestRequest) (
	*endpoints.APICertificateRequestResponse, int, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, httpClientError, err
	}
	var resp endpoints.APICertificateRequestResponse
	code, err := c.do(http.MethodPost, endpoints.APICertificateRequestEndPoint, body, contentTypeJSON, &resp)
	if err != nil {
		return nil, code, err
	}
	return &resp, code, nil
}

// PutAPICertificate gives the api the signed certificate for its last request
func (c Connection) PutAPICertificate(cert endpoints.APICertificate) (*endpoints.APICertificateResponse, int, error) {
	body, err := json.Marshal(cert)
	if err != nil {
		return nil, httpClientError, err
	}
	var resp endpoints.APICertificateResponse
	code, err := c.do(http.MethodPut, endpoints.APICertificateEndPoint, body, contentTypeJSON, &resp)
	if err != nil {
		return nil, code, err
	}
	return &resp, code, nil
}
