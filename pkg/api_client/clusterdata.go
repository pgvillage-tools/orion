package api_client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
)

func (c Connection) GetCluster() (cd *apiv1.Data, httpcode int, err error) {
	statusUrl := c.EndpointUrl(endpoints.ClusterEndPoint)
	statusResp, statusErr := http.Get(statusUrl)
	defer func() { statusResp.Body.Close() }()
	if statusErr != nil {
		return nil, HttpClientError, statusErr
	}
	if statusResp.StatusCode != http.StatusOK {
		return nil, statusResp.StatusCode, fmt.Errorf("GET Cluster: expected %d, received %d", http.StatusOK,
			statusResp.StatusCode)
	}
	var myClusterData apiv1.Data
	decodeErr := json.NewDecoder(statusResp.Body).Decode(&myClusterData)
	if decodeErr != nil {
		return nil, HttpClientError, decodeErr
	}
	return &myClusterData, statusResp.StatusCode, nil
}

func (c Connection) PutCluster(cd *apiv1.Data) (httpCode int, err error) {
	jsonData, encodingErr := json.Marshal(cd)
	if encodingErr != nil {
		return HttpClientError, encodingErr
	}

	cdURL := c.EndpointUrl(endpoints.ClusterEndPoint)

	putReq, putReqErr := http.NewRequest("PUT", cdURL, bytes.NewBuffer(jsonData))
	if putReqErr != nil {
		return HttpClientError, putReqErr
	}
	putReq.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	putResp, putRespErr := client.Do(putReq)
	defer func() { _ = putResp.Body.Close() }()
	if putRespErr != nil {
		return HttpClientError, putRespErr
	}
	if putResp.StatusCode != http.StatusOK {
		return putResp.StatusCode, fmt.Errorf("POST Cluster: expected %d, received %d", http.StatusOK, putResp.StatusCode)
	}
	return putResp.StatusCode, nil
}

func (c Connection) PostCluster(cd *apiv1.Data) (htpCode int, err error) {
	jsonData, encodingErr := json.Marshal(cd)
	if encodingErr != nil {
		return HttpClientError, encodingErr
	}

	cdURL := c.EndpointUrl(endpoints.ClusterEndPoint)
	postResp, postErr := http.Post(cdURL, "application/json", bytes.NewBuffer(jsonData))
	defer func() { _ = postResp.Body.Close() }()
	if postErr != nil {
		return HttpClientError, postErr
	}
	if postResp.StatusCode != http.StatusOK {
		return postResp.StatusCode, fmt.Errorf("POST Cluster: expected %d, received %d", http.StatusOK, postResp.StatusCode)
	}
	return postResp.StatusCode, nil
}
