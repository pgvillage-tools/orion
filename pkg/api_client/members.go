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
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// GetKeepers returns the keepers, sorted on uid
func (c Connection) GetKeepers() (keepers apiv1.InfoKeepers, httpCode int, err error) {
	httpCode, err = c.Get(endpoints.KeepersEndPoint, &keepers)
	return keepers, httpCode, err
}

// GetKeeper returns the keeper with uid (404 when it doesn't exist)
func (c Connection) GetKeeper(uid string) (keeper apiv1.InfoKeeper, httpCode int, err error) {
	httpCode, err = getMember(c, endpoints.KeeperEndPoint, "uid", uid, &keeper)
	return keeper, httpCode, err
}

// GetProxy returns the proxy with uid (404 when it doesn't exist)
func (c Connection) GetProxy(uid string) (proxy apiv1.InfoProxy, httpCode int, err error) {
	httpCode, err = getMember(c, endpoints.ProxyEndPoint, "uid", uid, &proxy)
	return proxy, httpCode, err
}

// GetSentinel returns the sentinel with uid (404 when it doesn't exist)
func (c Connection) GetSentinel(uid string) (sentinel apiv1.InfoSentinel, httpCode int, err error) {
	httpCode, err = getMember(c, endpoints.SentinelEndPoint, "uid", uid, &sentinel)
	return sentinel, httpCode, err
}

// getMember gets ep, with the placeholder {name} filled in with uid, into o
func getMember(c Connection, ep endpoints.EndPoint, name, uid string, o any) (int, error) {
	specific, err := ep.Specific(map[string]string{name: uid})
	if err != nil {
		return httpClientError, err
	}
	return c.Get(specific, o)
}
