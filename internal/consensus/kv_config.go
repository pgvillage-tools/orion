// Copyright 2026 PgVillage
// Copyright 2018 Sorint.lab
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

package consensus

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/kvtools/consul"
	"github.com/kvtools/etcdv2"
	"github.com/kvtools/etcdv3"
	"github.com/pgvillage-tools/orion/internal/common"
)

// BackendType represents a type of KV Store BackendType
type BackendType string

const (
	// CONSUL means that consul is used as backend
	CONSUL BackendType = consul.StoreName
	// ETCDV2 means that etcd is used as backend and that the v2 api is used
	ETCDV2 BackendType = etcdv2.StoreName
	// ETCDV3 means that etcd is used as backend and that the v3 api is used
	ETCDV3 BackendType = etcdv3.StoreName
)

var storeTypes = []string{
	consul.StoreName,
	etcdv2.StoreName,
	etcdv3.StoreName,
}

func (bt BackendType) string() string {
	return string(bt)
}

const (
	// DefaultEtcdEndpoints defines the default endpoints when using etcd
	DefaultEtcdEndpoints = "http://127.0.0.1:2379"
	// DefaultConsulEndpoints defines the default endpoints when using consul
	DefaultConsulEndpoints = "http://127.0.0.1:8500"
)

var urlSchemeRegexp = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9+-.]*)://`)

// Config defines the config to be used for the endpoint
type Config struct {
	Backend       BackendType
	Endpoints     string
	Timeout       time.Duration
	BasePath      string
	CertFile      string
	KeyFile       string
	CAFile        string
	SkipTLSVerify bool
}

// TLSConfig creates and returns a TLSConfig from a Config if applicable
func (c Config) TLSConfig() (*tls.Config, error) {
	scheme, err := c.EndpointScheme()
	if err != nil {
		return nil, err
	}
	if scheme != "http" && scheme != "https" {
		return nil, errors.New("endpoints scheme must be http or https")
	}
	if scheme != "https" {
		return nil, nil
	}

	tlsConfig, err := common.NewTLSConfig(c.CertFile, c.KeyFile, c.CAFile, c.SkipTLSVerify)
	if err != nil {
		return nil, fmt.Errorf("cannot create store tls config: %v", err)
	}
	return tlsConfig, nil
}

// EndpointScheme returns the scheme of all endpoints
// (they should all have the same scheme, or an error occurs)
func (c Config) EndpointScheme() (string, error) {
	endpoints, err := c.EndPoints()
	if err != nil {
		return "", err
	}
	var scheme string
	for _, e := range endpoints {
		var curscheme string
		if urlSchemeRegexp.Match([]byte(e)) {
			u, err := url.Parse(e)
			if err != nil {
				return "", fmt.Errorf("cannot parse endpoint %q: %v", e, err)
			}
			curscheme = u.Scheme
		} else {
			// Assume it's a schemeless endpoint
			curscheme = "http"
		}
		if scheme == "" {
			scheme = curscheme
		}
		if scheme != curscheme {
			return "", errors.New("all the endpoints must have the same scheme")
		}
	}
	return scheme, nil
}

// EndpointAddrs returns the addresses of the endpoints
func (c Config) EndpointAddrs() ([]string, error) {
	endpoints, err := c.EndPoints()
	if err != nil {
		return nil, err
	}
	addrs := []string{}
	var scheme string
	for _, e := range endpoints {
		var curscheme, addr string
		if urlSchemeRegexp.Match([]byte(e)) {
			u, err := url.Parse(e)
			if err != nil {
				return nil, fmt.Errorf("cannot parse endpoint %q: %v", e, err)
			}
			curscheme = u.Scheme
			addr = u.Host
		} else {
			// Assume it's a schemeless endpoint
			curscheme = "http"
			addr = e
		}
		if scheme == "" {
			scheme = curscheme
		}
		if scheme != curscheme {
			return nil, errors.New("all the endpoints must have the same scheme")
		}
		addrs = append(addrs, addr)
	}
	return addrs, nil
}

// EndPoints returns a list of endpoints as strings
func (c Config) EndPoints() ([]string, error) {
	endpointsStr := c.Endpoints
	if endpointsStr == "" {
		switch c.Backend {
		case CONSUL:
			endpointsStr = DefaultConsulEndpoints
		case ETCDV2, ETCDV3:
			endpointsStr = DefaultEtcdEndpoints
		default:
			return nil, fmt.Errorf(
				"unexpected store '%s', should be any of %v",
				c.Backend,
				strings.Join(storeTypes, ","),
			)
		}
	}
	return strings.Split(endpointsStr, ","), nil
}
