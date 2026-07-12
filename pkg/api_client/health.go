package api_client

import (
	"fmt"
	"net/http"

	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
)

func (c Connection) Healthy() (httpcode int, err error) {
	healthUrl := c.EndpointUrl(endpoints.HealthEndPoint)
	healthResp, healthErr := http.Get(healthUrl)
	defer func() { healthResp.Body.Close() }()
	if healthErr != nil {
		return HttpClientError, healthErr
	}
	if healthResp.StatusCode != http.StatusOK {
		return healthResp.StatusCode, fmt.Errorf("GET Healthz: expected %d, received %d", http.StatusOK,
			healthResp.StatusCode)
	}
	return healthResp.StatusCode, nil
}
