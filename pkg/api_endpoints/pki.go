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

package api_endpoints

import "time"

// The endpoints of the online renewal of the api certificate (see docs/api/certificates.md). Only the cli (the client
// certificate with CommonName orion-cli) can use them. The private key of the api stays on the api.
const (
	// APICertificateRequestEndPoint makes the api create a new private key, which it keeps in memory, and return a
	// certificate request for it (POST, APICertificateRequestRequest, returns APICertificateRequestResponse). A new
	// request replaces the one before.
	APICertificateRequestEndPoint EndPoint = "pki/api/csr"
	// APICertificateEndPoint takes the signed certificate for the last request (PUT, APICertificate, returns
	// APICertificateResponse): the api stores it with the new key, and serves new connections with it
	APICertificateEndPoint EndPoint = "pki/api/certificate"
)

// APICertificateRequestRequest is sent to the APICertificateRequestEndPoint
type APICertificateRequestRequest struct {
	// KeyAlgorithm is the algorithm of the new private key (e.g. ecdsa-p256); that of the current key when empty
	KeyAlgorithm string `json:"keyAlgorithm,omitempty"`
}

// APICertificateRequestResponse is returned by the APICertificateRequestEndPoint
type APICertificateRequestResponse struct {
	// Request is the PEM encoded certificate request (base64 in JSON)
	Request []byte `json:"request"`
}

// APICertificate is sent to the APICertificateEndPoint
type APICertificate struct {
	// Chain is the new certificate followed by the server intermediate that signed it, PEM encoded (base64 in JSON)
	Chain []byte `json:"chain"`
}

// APICertificateResponse is returned by the APICertificateEndPoint
type APICertificateResponse struct {
	// Serial is the serial number of the certificate the api serves with now (hex)
	Serial string `json:"serial"`
	// NotAfter is its expiry
	NotAfter time.Time `json:"notAfter"`
}
