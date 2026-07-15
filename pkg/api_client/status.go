package api_client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
	"github.com/pgvillage-tools/orion/internal/logging"
)

// GetStatus uses the API to fetch status for the entire cluster
func (c Connection) GetStatus() (ic *apiv1.InfoCluster, httpCode int, err error) {
	_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
	statusUrl := c.EndpointURL(endpoints.StatusEndPoint)
	statusResp, statusErr := http.Get(statusUrl)
	defer func() {
		if err := statusResp.Body.Close(); err != nil {
			logger.Error().AnErr("error", err).Msg("body close failed")
		}
	}()
	if statusErr != nil {
		return nil, httpClientError, statusErr
	}
	if statusResp.StatusCode != http.StatusOK {
		return nil, statusResp.StatusCode, fmt.Errorf("GET Status: expected %d, received %d", http.StatusOK,
			statusResp.StatusCode)
	}
	var myStatus apiv1.InfoCluster
	decodeErr := json.NewDecoder(statusResp.Body).Decode(&myStatus)
	if decodeErr != nil {
		return nil, httpClientError, decodeErr
	}
	return &myStatus, statusResp.StatusCode, nil
}
