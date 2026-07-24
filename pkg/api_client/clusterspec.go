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

// GetClusterSpec uses the API to fetch the cluster spec
func (c Connection) GetClusterSpec() (cd *apiv1.Spec, httpCode int, err error) {
	var myClusterSpec apiv1.Spec
	httpCode, err = c.Get(endpoints.ClusterSpecEndPoint, &myClusterSpec)
	if err != nil {
		return nil, httpCode, err
	}
	return &myClusterSpec, httpClientNoError, nil
}

// PutClusterSpec uses the API to write the cluster spec
func (c Connection) PutClusterSpec(cd *apiv1.Spec) (httpCode int, err error) {
	return c.Put(endpoints.ClusterSpecEndPoint, cd)
}

// TODO: Should there be a post?

// PostClusterSpec uses the API to write the cluster spec
func (c Connection) PostClusterSpec(cd *apiv1.Spec) (httpCode int, err error) {
	return c.Post(endpoints.ClusterSpecEndPoint, cd)
}

// PatchClusterSpec uses the API to update the cluster spec
func (c Connection) PatchClusterSpec(patch []byte) (httpCode int, err error) {
	return c.Patch(endpoints.ClusterSpecEndPoint, patch)
}
