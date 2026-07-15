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
