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
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// Healthy checks the API to be in healthy state (includes and etcd roundtrip). Use it on a connection to the probe
// port.
func (c Connection) Healthy() (httpCode int, err error) {
	httpCode, err = c.Get(endpoints.HealthEndPoint, nil)
	if err != nil {
		return httpCode, err
	}
	return httpClientNoError, nil
}

// Mode returns the mode of the API. Use it on a connection to the probe port.
func (c Connection) Mode() (mode endpoints.ModeResponse, httpCode int, err error) {
	httpCode, err = c.Get(endpoints.ModeEndPoint, &mode)
	return mode, httpCode, err
}

// WhoAmI returns the identity of the client, as the API sees it
func (c Connection) WhoAmI() (identity endpoints.WhoAmIResponse, httpCode int, err error) {
	httpCode, err = c.Get(endpoints.WhoAmIEndPoint, &identity)
	return identity, httpCode, err
}
