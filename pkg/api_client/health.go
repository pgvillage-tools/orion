package api_client

import (
	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
)

// Healthy checks the API to be in healthy state (includes and etcd roundtrip)
func (c Connection) Healthy() (httpCode int, err error) {
	httpCode, err = c.Get(endpoints.HealthEndPoint, nil)
	if err != nil {
		return httpCode, err
	}
	return httpClientNoError, nil
}
