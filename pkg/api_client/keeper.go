package api_client

import (
	"fmt"
	"net/http"

	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
)

func (c Connection) PutFailKeeper(keeper string) (httpCode int, err error) {
	specificEP, specError := endpoints.FailKeeperEndPoint.Specific(map[string]string{"id": keeper})
	if specError != nil {
		return HttpClientError, specError
	}
	cdURL := c.EndpointUrl(specificEP)

	putReq, putReqErr := http.NewRequest("PUT", cdURL, nil)
	if putReqErr != nil {
		return HttpClientError, putReqErr
	}
	client := &http.Client{}
	putResp, putRespErr := client.Do(putReq)
	defer func() { _ = putResp.Body.Close() }()
	if putRespErr != nil {
		return HttpClientError, putRespErr
	}
	if putResp.StatusCode != http.StatusOK {
		return putResp.StatusCode, fmt.Errorf("PUT Fail Keeper: expected %d, received %d", http.StatusOK,
			putResp.StatusCode)
	}
	return putResp.StatusCode, nil
}

func (c Connection) DeleteKeeper(keeper string) (httpCode int, err error) {
	specificEP, specError := endpoints.KeeperEndPoint.Specific(map[string]string{"id": keeper})
	if specError != nil {
		return HttpClientError, specError
	}
	cdURL := c.EndpointUrl(specificEP)

	putReq, putReqErr := http.NewRequest("DELETE", cdURL, nil)
	if putReqErr != nil {
		return HttpClientError, putReqErr
	}
	client := &http.Client{}
	putResp, putRespErr := client.Do(putReq)
	defer func() { _ = putResp.Body.Close() }()
	if putRespErr != nil {
		return HttpClientError, putRespErr
	}
	if putResp.StatusCode != http.StatusOK {
		return putResp.StatusCode, fmt.Errorf("DELETE Keeper: expected %d, received %d", http.StatusOK,
			putResp.StatusCode)
	}
	return putResp.StatusCode, nil
}
