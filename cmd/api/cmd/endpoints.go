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

package cmd

import (
	"fmt"
	"net/http"
	"strings"
)

// Protocol defines a protocol (basically either http or https in our case)
type Protocol string

const (
	// HTTPS is used to specify the https protocol
	HTTPS Protocol = "https"
	// HTTP is used to specify the http protocol
	HTTP Protocol = "http"
)

// Method defines the HTTP
type Method string

const (
	// MethodGet is used to define GET requests
	MethodGet = http.MethodGet
	// MethodPatch is used to define PATCH requests
	MethodPatch = http.MethodPatch
	// MethodPost is used to define POST requests
	MethodPost = http.MethodPost
	// MethodPut is used to define PUT requests
	MethodPut = http.MethodPut
	// MethodDelete is used to define DELETE requests
	MethodDelete = http.MethodDelete
)

// EndPoint defines api endpoint configurations
type EndPoint string

func (ep EndPoint) clean() string {
	return strings.Trim(string(ep), "/")
}

// URL can be used to retrieve the URL to use this endpoint
func (ep EndPoint) URL(protocol Protocol, ip string, port uint16) string {
	return fmt.Sprintf("%s://%s:%d/%s", protocol, ip, port, ep.clean())
}

// Route can be used to get the route definition for adding this endpoint to the API
func (ep EndPoint) Route(method Method, vars ...string) string {
	r := fmt.Sprintf("%s /%s", method, ep.clean())
	for _, v := range vars {
		v = strings.Trim(v, `/{}`)
		r += fmt.Sprintf("/{%s}", v)
	}
	return r
}
