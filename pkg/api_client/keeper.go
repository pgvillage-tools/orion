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
