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
	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
)

// PutFailKeeper uses the API to trigger a Failover for a Keeper
func (c Connection) PutFailKeeper(keeper string) (httpCode int, err error) {
	specificEP, specError := endpoints.FailKeeperEndPoint.Specific(map[string]string{"id": keeper})
	if specError != nil {
		return httpClientError, specError
	}
	return c.Put(specificEP, nil)
}

// DeleteKeeper uses the API to delete all info regarding a Keeper
func (c Connection) DeleteKeeper(keeper string) (httpCode int, err error) {
	specificEP, specError := endpoints.KeeperEndPoint.Specific(map[string]string{"id": keeper})
	if specError != nil {
		return httpClientError, specError
	}
	return c.Delete(specificEP)
}
