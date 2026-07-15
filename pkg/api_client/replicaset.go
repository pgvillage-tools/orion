package api_client

import (
	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
)

// PutPromoteReplicaSet uses the API to write replicaset config
func (c Connection) PutPromoteReplicaSet() (httpCode int, err error) {
	return c.Put(endpoints.PromoteReplicaSetEndPoint, nil)
}
