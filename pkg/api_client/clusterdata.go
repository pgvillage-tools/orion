package api_client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
	"github.com/pgvillage-tools/orion/internal/logging"
)

// GetCluster uses the API to fetch cluster data and return
func (c Connection) GetCluster() (cd *apiv1.Data, httpcode int, err error) {
	_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
	clusterDataUrl := c.EndpointURL(endpoints.ClusterEndPoint)
	clusterDataResp, clusterDataErr := http.Get(clusterDataUrl)
	defer func() {
		if err := clusterDataResp.Body.Close(); err != nil {
			logger.Error().AnErr("error", err).Msg("body close failed")
		}
	}()
	if clusterDataErr != nil {
		return nil, httpClientError, clusterDataErr
	}
	if clusterDataResp.StatusCode != http.StatusOK {
		return nil, clusterDataResp.StatusCode, fmt.Errorf("GET Cluster: expected %d, received %d", http.StatusOK,
			clusterDataResp.StatusCode)
	}
	var myClusterData apiv1.Data
	decodeErr := json.NewDecoder(clusterDataResp.Body).Decode(&myClusterData)
	if decodeErr != nil {
		return nil, httpClientError, decodeErr
	}
	return &myClusterData, clusterDataResp.StatusCode, nil
}

// PutCluster uses the API to write cluster data
func (c Connection) PutCluster(cd *apiv1.Data) (httpCode int, err error) {
	jsonData, encodingErr := json.Marshal(cd)
	if encodingErr != nil {
		return httpClientError, encodingErr
	}

	cdURL := c.EndpointURL(endpoints.ClusterEndPoint)

	putReq, putReqErr := http.NewRequest("PUT", cdURL, bytes.NewBuffer(jsonData))
	if putReqErr != nil {
		return httpClientError, putReqErr
	}
	putReq.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	putResp, putRespErr := client.Do(putReq)
	defer func() { _ = putResp.Body.Close() }()
	if putRespErr != nil {
		return httpClientError, putRespErr
	}
	if putResp.StatusCode != http.StatusOK {
		return putResp.StatusCode, fmt.Errorf("POST Cluster: expected %d, received %d", http.StatusOK,
			putResp.StatusCode)
	}
	return putResp.StatusCode, nil
}

// TODO should there be a POST?

// PostCluster uses the API to write cluster data
func (c Connection) PostCluster(cd *apiv1.Data) (htpCode int, err error) {
	jsonData, encodingErr := json.Marshal(cd)
	if encodingErr != nil {
		return httpClientError, encodingErr
	}

	cdURL := c.EndpointURL(endpoints.ClusterEndPoint)
	postResp, postErr := http.Post(cdURL, "application/json", bytes.NewBuffer(jsonData))
	defer func() { _ = postResp.Body.Close() }()
	if postErr != nil {
		return httpClientError, postErr
	}
	if postResp.StatusCode != http.StatusOK {
		return postResp.StatusCode, fmt.Errorf("POST Cluster: expected %d, received %d", http.StatusOK,
			postResp.StatusCode)
	}
	return postResp.StatusCode, nil
}
