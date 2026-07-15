package api_client

import (
	"context"
	"fmt"
	"net/http"

	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
	"github.com/pgvillage-tools/orion/internal/logging"
)

// Healthy checks the API to be in healthy state (includes and etcd roundtrip)
func (c Connection) Healthy() (httpcode int, err error) {
	_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
	healthUrl := c.EndpointURL(endpoints.HealthEndPoint)
	healthResp, healthErr := http.Get(healthUrl)
	defer func() {
		if err := healthResp.Body.Close(); err != nil {
			logger.Error().AnErr("error", err).Msg("body close failed")
		}
	}()
	if healthErr != nil {
		return httpClientError, healthErr
	}
	if healthResp.StatusCode != http.StatusOK {
		return healthResp.StatusCode, fmt.Errorf("GET Healthz: expected %d, received %d", http.StatusOK,
			healthResp.StatusCode)
	}
	return healthResp.StatusCode, nil
}
