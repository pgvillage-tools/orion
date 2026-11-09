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
	"net"
	"net/http"
	"strconv"
	"strings"
)

const (
	// KeeperEndPoint is a keeper (DELETE removes it from the cluster data)
	KeeperEndPoint EndPoint = "cluster/keepers/{uid}"
	// ClusterEndPoint is the endpoint config for Cluster
	ClusterEndPoint EndPoint = "cluster"
	// ClusterSpecEndPoint is the endpoint config for Cluster spec
	ClusterSpecEndPoint EndPoint = "cluster/spec"
	// FailKeeperEndPoint is the action that fails a keeper (POST), so the sentinel fails over when it runs the primary
	FailKeeperEndPoint EndPoint = "cluster/keepers/{uid}/actions/fail"
	// WhoAmIEndPoint returns the identity of the client
	WhoAmIEndPoint EndPoint = "whoami"
	// PromoteClusterEndPoint is the action that promotes a standby cluster to a primary cluster (POST); a primary
	// cluster stays as it is (200)
	PromoteClusterEndPoint EndPoint = "cluster/actions/promote"
	// StatusEndPoint configures how to handle status requests
	StatusEndPoint EndPoint = "cluster/status"
	// ProxiesEndPoint is the list of proxies, sorted on uid
	ProxiesEndPoint EndPoint = "cluster/proxies"
	// ProxyEndPoint is a single proxy, by uid
	ProxyEndPoint EndPoint = "cluster/proxies/{uid}"
	// SentinelsEndPoint is the list of sentinels, sorted on uid
	SentinelsEndPoint EndPoint = "cluster/sentinels"
	// SentinelEndPoint is a single sentinel, by uid
	SentinelEndPoint EndPoint = "cluster/sentinels/{uid}"
	// KeepersEndPoint is the list of keepers, sorted on uid
	KeepersEndPoint EndPoint = "cluster/keepers"
)

const (
	// LiveEndPoint is the liveness endpoint on the probe port: 200 as long as the process serves requests (it doesn't
	// check the store)
	LiveEndPoint ProbeEndPoint = "livez"
	// ReadyEndPoint is the readiness endpoint on the probe port: 200 when the api serves, isn't shutting down and its
	// store answers; 503 otherwise
	ReadyEndPoint ProbeEndPoint = "readyz"
	// ModeEndPoint defines the endpoint on the probe port that returns the mode of the api
	ModeEndPoint ProbeEndPoint = "status"
	// MetricsEndPoint defines the endpoint on the probe port that returns the Prometheus metrics of the api
	MetricsEndPoint ProbeEndPoint = "metrics"
)

const (
	// ModeNormal is the mode of the api when it serves the api with mTLS
	ModeNormal = "normal"
	// ModeBootstrap is the mode of the api while it bootstraps its certificates
	ModeBootstrap = "bootstrap"
)

// ModeResponse is returned by the ModeEndPoint
type ModeResponse struct {
	Mode string `json:"mode"`
}

// WhoAmIResponse is returned by the WhoAmIEndPoint
type WhoAmIResponse struct {
	Name   string   `json:"name"`
	Groups []string `json:"groups"`
	Method string   `json:"method"`
}

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
		strEP = strings.ReplaceAll(strEP, placeholder, value)
	}
	if strings.Contains(strEP, "{") {
		return EndPoint(""), fmt.Errorf("not all placeholders in %s were replaced", strEP)
	}
	return EndPoint(strEP), nil
}

func (ep EndPoint) clean() string {
	return APIVersion + "/" + strings.Trim(string(ep), "/")
}

// URL can be used to retrieve the URL to use this endpoint
func (ep EndPoint) URL(protocol Protocol, ip string, port uint16) string {
	return fmt.Sprintf("%s://%s/%s", protocol, hostPort(ip, port), ep.clean())
}

// hostPort joins host and port, and brackets IPv6 addresses (unless they already are)
func hostPort(host string, port uint16) string {
	return net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(int(port)))
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

// ProbeEndPoint defines an endpoint on the probe port. Unlike an EndPoint, it has no version prefix.
type ProbeEndPoint string

// URL can be used to retrieve the URL to use this endpoint
func (ep ProbeEndPoint) URL(protocol Protocol, ip string, port uint16) string {
	return fmt.Sprintf("%s://%s/%s", protocol, hostPort(ip, port), strings.Trim(string(ep), "/"))
}

// Route can be used to get the route definition for adding this endpoint to the probe server
func (ep ProbeEndPoint) Route(method Method) string {
	return fmt.Sprintf("%s /%s", method, strings.Trim(string(ep), "/"))
}
