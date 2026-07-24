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
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
	"github.com/pgvillage-tools/orion/internal/logging"
)

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

// EndpointURL calculates and returns the url for a specific endpoint on this Connection
func (c Connection) EndpointURL(ep endpoints.EndPoint) string {
	return ep.URL(c.protocol, c.host, c.port)
}

// Get will issue a GET request on the API
func (c Connection) Get(ep endpoints.EndPoint, o any) (httpError int, err error) {
	url := c.EndpointURL(ep)

	getReq, getReqErr := http.NewRequest(http.MethodGet, url, nil)
	if getReqErr != nil {
		return httpClientError, getReqErr
	}
	getResp, getRespErr := c.client.Do(getReq)

	if getRespErr != nil {
		return httpClientError, getRespErr
	}
	defer func() {
		if err := getResp.Body.Close(); err != nil {
			_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
			logger.Error().AnErr("error", err).Msg("body close failed")
		}
	}()
	if getResp.StatusCode < http.StatusOK || getResp.StatusCode >= http.StatusMultipleChoices {
		return getResp.StatusCode, fmt.Errorf("GET Cluster Spec: expected %d, received %d", http.StatusOK,
			getResp.StatusCode)
	}
	if o != nil {
		decodeErr := json.NewDecoder(getResp.Body).Decode(o)
		if decodeErr != nil {
			return httpClientError, decodeErr
		}
	}
	return getResp.StatusCode, nil
}

// Put will issue a PUT request on the API
func (c Connection) Put(ep endpoints.EndPoint, o any) (httpErr int, err error) {
	jsonData, encodingErr := json.Marshal(o)
	if encodingErr != nil {
		return httpClientError, encodingErr
	}

	url := c.EndpointURL(ep)

	putReq, putReqErr := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(jsonData))
	if putReqErr != nil {
		return httpClientError, putReqErr
	}
	putReq.Header.Set("Content-Type", "application/json")
	putResp, putRespErr := c.client.Do(putReq)
	if putRespErr != nil {
		return httpClientError, putRespErr
	}
	defer func() {
		if err := putResp.Body.Close(); err != nil {
			_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
			logger.Error().AnErr("error", err).Msg("body close failed")
		}
	}()
	if putResp.StatusCode < http.StatusOK || putResp.StatusCode >= http.StatusMultipleChoices {
		return putResp.StatusCode, fmt.Errorf("POST Cluster: expected %d, received %d", http.StatusOK,
			putResp.StatusCode)
	}
	return putResp.StatusCode, nil
}

// Post will issue a Post request on the API
func (c Connection) Post(ep endpoints.EndPoint, o any) (httpErr int, err error) {
	jsonData, encodingErr := json.Marshal(o)
	if encodingErr != nil {
		return httpClientError, encodingErr
	}

	url := c.EndpointURL(ep)
	postReq, postReqErr := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonData))
	if postReqErr != nil {
		return httpClientError, postReqErr
	}
	postReq.Header.Set("Content-Type", "application/json")
	postResp, postRespErr := c.client.Do(postReq)
	if postRespErr != nil {
		return httpClientError, postRespErr
	}
	defer func() {
		if err := postResp.Body.Close(); err != nil {
			_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
			logger.Error().AnErr("error", err).Msg("body close failed")
		}
	}()
	if postResp.StatusCode < http.StatusOK || postResp.StatusCode >= http.StatusMultipleChoices {
		return postResp.StatusCode, fmt.Errorf("POST Cluster: expected %d, received %d", http.StatusOK,
			postResp.StatusCode)
	}
	return postResp.StatusCode, nil
}

// Patch will issue a POST request on the API
func (c Connection) Patch(ep endpoints.EndPoint, patch []byte) (httpCode int, err error) {
	url := c.EndpointURL(ep)

	patchReq, patchReqErr := http.NewRequest(http.MethodPatch, url, bytes.NewBuffer(patch))
	if patchReqErr != nil {
		return httpClientError, patchReqErr
	}
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, patchRespErr := c.client.Do(patchReq)
	if patchRespErr != nil {
		return httpClientError, patchRespErr
	}
	defer func() {
		if err := patchResp.Body.Close(); err != nil {
			_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
			logger.Error().AnErr("error", err).Msg("body close failed")
		}
	}()
	if patchResp.StatusCode < http.StatusOK || patchResp.StatusCode >= http.StatusMultipleChoices {
		return patchResp.StatusCode, fmt.Errorf("PATCH Cluster Spec: expected %d, received %d", http.StatusOK,
			patchResp.StatusCode)
	}
	return patchResp.StatusCode, nil
}

// Delete will issue a DELETE request on the API
func (c Connection) Delete(ep endpoints.EndPoint) (httpCode int, err error) {
	url := c.EndpointURL(ep)

	deleteReq, deleteReqErr := http.NewRequest(http.MethodDelete, url, nil)
	if deleteReqErr != nil {
		return httpClientError, deleteReqErr
	}
	deleteResp, deleteRespErr := c.client.Do(deleteReq)
	if deleteRespErr != nil {
		return httpClientError, deleteRespErr
	}
	defer func() {
		if err := deleteResp.Body.Close(); err != nil {
			_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
			logger.Error().AnErr("error", err).Msg("body close failed")
		}
	}()
	if deleteResp.StatusCode < http.StatusOK || deleteResp.StatusCode >= http.StatusMultipleChoices {
		return deleteResp.StatusCode, fmt.Errorf("DELETE Keeper: expected %d, received %d", http.StatusOK,
			deleteResp.StatusCode)
	}
	return deleteResp.StatusCode, nil
}
