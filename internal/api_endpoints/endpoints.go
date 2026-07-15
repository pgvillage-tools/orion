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

package api_endpoints

import (
	"fmt"
	"net/http"
	"strings"
)

const (
	// KeeperEndPoint will promote a ReplicaCluster to primary (or return OK when it already is)
	KeeperEndPoint EndPoint = "cluster/keepers/{id}"
	// ClusterEndPoint is the endpoint config for Cluster
	ClusterEndPoint EndPoint = "cluster"
	// ClusterSpecEndPoint is the endpoint config for Cluster spec
	ClusterSpecEndPoint EndPoint = "cluster/spec"
	// FailKeeperEndPoint is the config for the failkeeper endpoint
	FailKeeperEndPoint EndPoint = "cluster/keepers/{id}/fail"
	// HealthEndPoint defines the endpoint to check on health (including consensus)
	HealthEndPoint EndPoint = "healthz"
	// ReadyEndPoint defines the endpoint to check on ready (only that we are serving)
	ReadyEndPoint EndPoint = "readyz"
	// PromoteReplicaSetEndPoint will promote a ReplicaCluster to primary (or return OK when it already is)
	PromoteReplicaSetEndPoint EndPoint = "cluster/promote"
	// StatusEndPoint configures how to handle status requests
	StatusEndPoint EndPoint = "cluster/status"
	// ProxyStatusEndPoint configures how to handle proxy status requests
	ProxyStatusEndPoint EndPoint = "cluster/proxies/status"
	// SentinelStatusEndPoint configures how to handle sentinel status requests
	SentinelStatusEndPoint EndPoint = "cluster/sentinels/status"
)

// Protocol defines a protocol (basically either http or https in our case)
type Protocol string

const (
	// HTTPS is used to specify the https protocol
	HTTPS Protocol = "https"
	// HTTP is used to specify the http protocol
	HTTP Protocol = "http"
	// APIVersion specifies the pi version prefixed to the endpoints
	APIVersion = "v1"
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

// Specific returns a specific endpoint created from a generic endpoint and filled in vars
func (ep EndPoint) Specific(vars map[string]string) (specific EndPoint, err error) {
	strEP := string(ep)
	for key, value := range vars {
		placeholder := fmt.Sprintf("{%s}", key)
		if !strings.Contains(strEP, placeholder) {
			return EndPoint(""), fmt.Errorf("ep %s has no placeholder %s", ep, placeholder)
		}
		strings.ReplaceAll(strEP, placeholder, value)
	}
	if strings.Contains(strEP, "{") {
		return EndPoint(""), fmt.Errorf("not all placeholders in %s where replaced", strEP)
	}
	return EndPoint(strEP), nil
}

func (ep EndPoint) clean() string {
	return APIVersion + "/" + strings.Trim(string(ep), "/")
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
