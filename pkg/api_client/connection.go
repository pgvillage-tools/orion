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
// See the License for the specific lang

package api_client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pgvillage-tools/orion/internal/logging"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

const (
	contentTypeJSON = "application/json"
	// maxErrorBody is the maximum size of an error response that is read
	maxErrorBody = 64 << 10
)

// endpoint is an endpoint on the api port (endpoints.EndPoint) or on the probe port (endpoints.ProbeEndPoint)
type endpoint interface {
	URL(protocol endpoints.Protocol, ip string, port uint16) string
}

// Connection is a resource to connect to the orion API
type Connection struct {
	protocol endpoints.Protocol
	host     string
	port     uint16
	client   http.Client
}

// NewConnection returns an initialized Connection
func NewConnection(protocol endpoints.Protocol, host string, port uint16,
	timeout time.Duration) Connection {
	return Connection{
		protocol: protocol,
		host:     host,
		port:     port,
		client:   http.Client{Timeout: timeout},
	}
}

// NewTLSConnection returns an initialized Connection that uses https with tlsConfig (e.g. with a client
// certificate)
func NewTLSConnection(host string, port uint16, timeout time.Duration, tlsConfig *tls.Config) Connection {
	return Connection{
		protocol: endpoints.HTTPS,
		host:     host,
		port:     port,
		client: http.Client{
			Timeout:   timeout,
			Transport: &http.Transport{TLSClientConfig: tlsConfig, Proxy: http.ProxyFromEnvironment},
		},
	}
}

// EndpointURL calculates and returns the url for a specific endpoint on this Connection
func (c Connection) EndpointURL(ep endpoint) string {
	return ep.URL(c.protocol, c.host, c.port)
}

// Get issues a GET request for ep, and decodes the response into o (when o isn't nil)
func (c Connection) Get(ep endpoint, o any) (httpCode int, err error) {
	return c.do(http.MethodGet, ep, nil, "", o)
}

// Put issues a PUT request for ep with o as the JSON body
func (c Connection) Put(ep endpoints.EndPoint, o any) (httpCode int, err error) {
	body, err := json.Marshal(o)
	if err != nil {
		return httpClientError, err
	}
	return c.do(http.MethodPut, ep, body, contentTypeJSON, nil)
}

// Post issues a POST request for ep with o as the JSON body
func (c Connection) Post(ep endpoints.EndPoint, o any) (httpCode int, err error) {
	body, err := json.Marshal(o)
	if err != nil {
		return httpClientError, err
	}
	return c.do(http.MethodPost, ep, body, contentTypeJSON, nil)
}

// Patch issues a PATCH request for ep with patch, a JSON merge patch (RFC 7386), as the body
func (c Connection) Patch(ep endpoints.EndPoint, patch []byte) (httpCode int, err error) {
	return c.do(http.MethodPatch, ep, patch, endpoints.ContentTypeMergePatch, nil)
}

// Delete issues a DELETE request for ep
func (c Connection) Delete(ep endpoints.EndPoint) (httpCode int, err error) {
	return c.do(http.MethodDelete, ep, nil, "", nil)
}

// Action issues a POST request without a body for an action endpoint (e.g. .../actions/fail)
func (c Connection) Action(ep endpoints.EndPoint) (httpCode int, err error) {
	return c.do(http.MethodPost, ep, nil, "", nil)
}

// do sends a request with method and body (when not nil) to ep, and decodes the response into o (when o isn't nil).
// It returns the status code of the response, or httpClientError when there is no response (or it can't be decoded).
// A response that isn't 2xx is returned as an error: the api's problem (RFC 9457), or a problem with the status and
// the body of the response when it isn't one.
func (c Connection) do(method string, ep endpoint, body []byte, contentType string, o any) (int, error) {
	httpCode, _, err := c.doWithHeaders(method, ep, body, contentType, nil, o)
	return httpCode, err
}

// doWithHeaders is do, with extra request headers, and it also returns the headers of the response
func (c Connection) doWithHeaders(method string, ep endpoint, body []byte, contentType string,
	headers map[string]string, o any) (int, http.Header, error) {
	url := c.EndpointURL(ep)
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return httpClientError, nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return httpClientError, nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
			logger.Error().AnErr("error", err).Msg("body close failed")
		}
	}()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return resp.StatusCode, resp.Header, fmt.Errorf("%s %s: %w", method, url, readProblem(resp))
	}
	if o != nil {
		if err := json.NewDecoder(resp.Body).Decode(o); err != nil {
			return httpClientError, resp.Header, fmt.Errorf("%s %s: failed to decode the response: %w", method, url,
				err)
		}
	}
	return resp.StatusCode, resp.Header, nil
}

// GetWithETag issues a GET request for ep, decodes the response into o, and returns its ETag
func (c Connection) GetWithETag(ep endpoint, o any) (httpCode int, etag string, err error) {
	httpCode, headers, err := c.doWithHeaders(http.MethodGet, ep, nil, "", nil, o)
	if headers != nil {
		etag = headers.Get("ETag")
	}
	return httpCode, etag, err
}

// PutIfMatch issues a PUT request for ep with o as the JSON body, that only succeeds when the resource still has
// etag (412 otherwise); an empty etag sends no If-Match. It returns the new ETag.
func (c Connection) PutIfMatch(ep endpoints.EndPoint, o any, etag string) (httpCode int, newETag string, err error) {
	body, err := json.Marshal(o)
	if err != nil {
		return httpClientError, "", err
	}
	return c.writeIfMatch(http.MethodPut, ep, body, contentTypeJSON, etag)
}

// PatchIfMatch issues a PATCH request (a JSON merge patch) for ep, that only succeeds when the resource still has
// etag (412 otherwise); an empty etag sends no If-Match. It returns the new ETag.
func (c Connection) PatchIfMatch(ep endpoints.EndPoint, patch []byte, etag string) (httpCode int, newETag string,
	err error) {
	return c.writeIfMatch(http.MethodPatch, ep, patch, endpoints.ContentTypeMergePatch, etag)
}

func (c Connection) writeIfMatch(method string, ep endpoints.EndPoint, body []byte, contentType, etag string) (int,
	string, error) {
	var headers map[string]string
	if etag != "" {
		headers = map[string]string{"If-Match": etag}
	}
	httpCode, respHeaders, err := c.doWithHeaders(method, ep, body, contentType, headers, nil)
	if respHeaders == nil {
		return httpCode, "", err
	}
	return httpCode, respHeaders.Get("ETag"), err
}

// readProblem returns the problem in the body of an error response. When the body isn't a problem (e.g. a 405 from
// the router), it returns a problem with the status, and the body (shortened) as the detail.
func readProblem(resp *http.Response) endpoints.Problem {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	if strings.HasPrefix(resp.Header.Get("Content-Type"), endpoints.ContentTypeProblem) {
		var p endpoints.Problem
		if err := json.Unmarshal(data, &p); err == nil && p.Status != 0 {
			return p
		}
	}
	return endpoints.NewProblem(resp.StatusCode, strings.TrimSpace(string(data)))
}
