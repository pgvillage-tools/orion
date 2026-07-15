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

// GetClusterSpec uses the API to fetch the cluster spec
func (c Connection) GetClusterSpec() (cd *apiv1.Data, httpcode int, err error) {
	_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
	clusterSpecUrl := c.EndpointURL(endpoints.ClusterSpecEndPoint)
	clusterSpecResp, clusterSpecErr := http.Get(clusterSpecUrl)
	defer func() {
		if err := clusterSpecResp.Body.Close(); err != nil {
			logger.Error().AnErr("error", err).Msg("body close failed")
		}
	}()
	if clusterSpecErr != nil {
		return nil, httpClientError, clusterSpecErr
	}
	if clusterSpecResp.StatusCode != http.StatusOK {
		return nil, clusterSpecResp.StatusCode, fmt.Errorf("GET Cluster Spec: expected %d, received %d", http.StatusOK,
			clusterSpecResp.StatusCode)
	}
	var myClusterData apiv1.Data
	decodeErr := json.NewDecoder(clusterSpecResp.Body).Decode(&myClusterData)
	if decodeErr != nil {
		return nil, httpClientError, decodeErr
	}
	return &myClusterData, clusterSpecResp.StatusCode, nil
}

// PutClusterSpec uses the API to write the cluster spec
func (c Connection) PutClusterSpec(cd *apiv1.Spec) (httpCode int, err error) {
	jsonData, encodingErr := json.Marshal(cd)
	if encodingErr != nil {
		return httpClientError, encodingErr
	}

	cdURL := c.EndpointURL(endpoints.ClusterSpecEndPoint)

	// If you want to something else then POST or GET:
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
		return putResp.StatusCode, fmt.Errorf("PUT Cluster Spec: expected %d, received %d", http.StatusOK,
			putResp.StatusCode)
	}
	return putResp.StatusCode, nil
}

// TODO: SHould there be a post?

// PostClusterSpec uses the API to write the cluster spec
func (c Connection) PostClusterSpec(cd *apiv1.Spec) (httpCode int, err error) {
	jsonData, encodingErr := json.Marshal(cd)
	if encodingErr != nil {
		return httpClientError, encodingErr
	}

	cdURL := c.EndpointURL(endpoints.ClusterSpecEndPoint)
	postResp, postErr := http.Post(cdURL, "application/json", bytes.NewBuffer(jsonData))
	defer func() { _ = postResp.Body.Close() }()
	if postErr != nil {
		return httpClientError, postErr
	}
	if postResp.StatusCode != http.StatusOK {
		return postResp.StatusCode, fmt.Errorf("POST ClusterSpec: expected %d, received %d", http.StatusOK,
			postResp.StatusCode)
	}
	return postResp.StatusCode, nil
}

// PatchClusterSpec uses the API to update the cluster spec
func (c Connection) PatchClusterSpec(patch []byte) (httpCode int, err error) {
	cdURL := c.EndpointURL(endpoints.ClusterSpecEndPoint)

	// If you want to something else then POST or GET:
	patchReq, patchReqErr := http.NewRequest("PATCH", cdURL, bytes.NewBuffer(patch))
	if patchReqErr != nil {
		return httpClientError, patchReqErr
	}
	patchReq.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	patchResp, patchRespErr := client.Do(patchReq)
	defer func() { _ = patchResp.Body.Close() }()
	if patchRespErr != nil {
		return httpClientError, patchRespErr
	}
	if patchResp.StatusCode != http.StatusOK {
		return patchResp.StatusCode, fmt.Errorf("PATCH Cluster Spec: expected %d, received %d", http.StatusOK,
			patchResp.StatusCode)
	}
	return patchResp.StatusCode, nil
}
