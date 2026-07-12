package api_client

import (
	"encoding/json"
	"fmt"
	"net/http"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
)

func (c Connection) GetStatus() (ic *apiv1.InfoCluster, httpCode int, err error) {
	statusUrl := c.EndpointUrl(endpoints.StatusEndPoint)
	statusResp, statusErr := http.Get(statusUrl)
	defer func() { statusResp.Body.Close() }()
	if statusErr != nil {
		return nil, HttpClientError, statusErr
	}
	if statusResp.StatusCode != http.StatusOK {
		return nil, statusResp.StatusCode, fmt.Errorf("GET Status: expected %d, received %d", http.StatusOK, statusResp.StatusCode)
	}
	var myStatus apiv1.InfoCluster
	decodeErr := json.NewDecoder(statusResp.Body).Decode(&myStatus)
	if decodeErr != nil {
		return nil, HttpClientError, decodeErr
	}
	return &myStatus, statusResp.StatusCode, nil
}
