package api_client

import (
	"context"
	"fmt"
	"net/http"

	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
	"github.com/pgvillage-tools/orion/internal/logging"
)

// PutPromoteReplicaSet uses the API to write replicaset config
func (c Connection) PutPromoteReplicaSet() (httpCode int, err error) {
	_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
	cdURL := c.EndpointURL(endpoints.PromoteReplicaSetEndPoint)

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
		return putResp.StatusCode, fmt.Errorf("PUT Promote Keeper: expected %d, received %d", http.StatusOK,
			putResp.StatusCode)
	}
	return putResp.StatusCode, nil
}
