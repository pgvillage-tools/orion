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

// GetCluster uses the API to fetch cluster data and return
func (c Connection) GetCluster() (cd *apiv1.Data, httpCode int, err error) {
	var myClusterData apiv1.Data
	httpCode, err = c.Get(endpoints.ClusterEndPoint, &myClusterData)
	if err != nil {
		return nil, httpCode, err
	}
	return &myClusterData, httpClientNoError, nil
}

// PutCluster uses the API to write cluster data
func (c Connection) PutCluster(cd *apiv1.Data) (httpCode int, err error) {
	return c.Put(endpoints.ClusterEndPoint, cd)
}

// TODO should there be a POST?

// PostCluster uses the API to write cluster data
func (c Connection) PostCluster(cd *apiv1.Data) (htpCode int, err error) {
	return c.Post(endpoints.ClusterEndPoint, cd)
}
