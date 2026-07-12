package api_client

import (
	"fmt"
	"net/http"

	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
)

func (c Connection) PutPromoteReplicaSet() (httpCode int, err error) {
	cdURL := c.EndpointUrl(endpoints.PromoteReplicaSetEndPoint)

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
		return putResp.StatusCode, fmt.Errorf("PUT Promote Keeper: expected %d, received %d", http.StatusOK,
			putResp.StatusCode)
	}
	return putResp.StatusCode, nil
}
