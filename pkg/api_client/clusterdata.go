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
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// GetCluster uses the API to fetch cluster data and return
func (c Connection) GetCluster() (cd *apiv1.Data, httpCode int, err error) {
	var myClusterData apiv1.Data
	httpCode, err = c.Get(endpoints.ClusterEndPoint, &myClusterData)
	if err != nil {
		return nil, httpCode, err
	}
	return &myClusterData, httpCode, nil
}

// PutCluster uses the API to write cluster data
func (c Connection) PutCluster(cd *apiv1.Data) (httpCode int, err error) {
	return c.Put(endpoints.ClusterEndPoint, cd)
}

// PostCluster uses the API to create the cluster with spec cs (201). The API refuses it (409) when there already is a
// cluster.
func (c Connection) PostCluster(cs *apiv1.Spec) (httpCode int, err error) {
	return c.Post(endpoints.ClusterEndPoint, cs)
}

// GetClusterWithETag returns the cluster data and its ETag (to send back as If-Match)
func (c Connection) GetClusterWithETag() (cd *apiv1.Data, etag string, httpCode int, err error) {
	var d apiv1.Data
	httpCode, etag, err = c.GetWithETag(endpoints.ClusterEndPoint, &d)
	if err != nil {
		return nil, etag, httpCode, err
	}
	return &d, etag, httpCode, nil
}

// PutClusterIfMatch replaces the cluster data when it still has etag (412 otherwise; an empty etag replaces it
// unconditionally), and returns the new ETag
func (c Connection) PutClusterIfMatch(cd *apiv1.Data, etag string) (httpCode int, newETag string, err error) {
	return c.PutIfMatch(endpoints.ClusterEndPoint, cd, etag)
}
