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
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
)

// GetStatus uses the API to fetch status for the entire cluster
func (c Connection) GetStatus() (ic *apiv1.InfoCluster, httpCode int, err error) {
	var myStatus apiv1.InfoCluster

	httpCode, err = c.Get(endpoints.StatusEndPoint, &myStatus)
	if err != nil {
		return nil, httpCode, err
	}
	return &myStatus, httpCode, nil
}
