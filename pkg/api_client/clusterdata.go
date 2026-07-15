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
