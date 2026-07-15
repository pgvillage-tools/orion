package api_client

import (
	"context"
	"fmt"
	"net/http"

	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
	"github.com/pgvillage-tools/orion/internal/logging"
)

// PutFailKeeper uses the API to trigger a Failover for a Keeper
func (c Connection) PutFailKeeper(keeper string) (httpCode int, err error) {
	_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
	specificEP, specError := endpoints.FailKeeperEndPoint.Specific(map[string]string{"id": keeper})
	if specError != nil {
		return httpClientError, specError
	}
	cdURL := c.EndpointURL(specificEP)

	putReq, putReqErr := http.NewRequest("PUT", cdURL, nil)
	if putReqErr != nil {
		return httpClientError, putReqErr
	}
	client := &http.Client{}
	putResp, putRespErr := client.Do(putReq)
	defer func() {
		if err := putResp.Body.Close(); err != nil {
			logger.Error().AnErr("error", err).Msg("body close failed")
		}
	}()
	if putRespErr != nil {
		return httpClientError, putRespErr
	}
	if putResp.StatusCode != http.StatusOK {
		return putResp.StatusCode, fmt.Errorf("PUT Fail Keeper: expected %d, received %d", http.StatusOK,
			putResp.StatusCode)
	}
	return putResp.StatusCode, nil
}

// DeleteKeeper uses the API to delete all info regarding a Keeper
func (c Connection) DeleteKeeper(keeper string) (httpCode int, err error) {
	specificEP, specError := endpoints.KeeperEndPoint.Specific(map[string]string{"id": keeper})
	if specError != nil {
		return httpClientError, specError
	}
	cdURL := c.EndpointURL(specificEP)

	putReq, putReqErr := http.NewRequest("DELETE", cdURL, nil)
	if putReqErr != nil {
		return httpClientError, putReqErr
	}
	client := &http.Client{}
	putResp, putRespErr := client.Do(putReq)
	defer func() { _ = putResp.Body.Close() }()
	if putRespErr != nil {
		return httpClientError, putRespErr
	}
	if putResp.StatusCode != http.StatusOK {
		return putResp.StatusCode, fmt.Errorf("DELETE Keeper: expected %d, received %d", http.StatusOK,
			putResp.StatusCode)
	}
	return putResp.StatusCode, nil
}
