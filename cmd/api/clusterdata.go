package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/logging"
)

func (h *Handlers) ClusterDataRoutes() []Route {
	return []Route{
		{"GET /clusterdata", h.GetClusterDataHandler},
		{"PUT /clusterdata", h.PutClusterDataHandler},
	}
}

// GetClusterDataHandler endpoint
func (h *Handlers) GetClusterDataHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(context.TODO(), time.Now().Add(time.Second))
	defer cancelFunc()

	if cd, _, err := h.client.GetClusterData(ctx); err != nil {
		handleError(ctx, err, w, "failed to get cluster data")
	} else {
		h.writeJSON(ctx, w, cd)
	}
}

// PutClusterDataHandler endpoint
func (h *Handlers) PutClusterDataHandler(w http.ResponseWriter, r *http.Request) {
	ctx, logger := logging.GetLogComponent(context.TODO(), logging.WebApiComponent)
	ctx, cancelFunc := context.WithDeadline(ctx, time.Now().Add(time.Second))
	defer cancelFunc()

	if cd, _, err := h.client.GetClusterData(ctx); err != nil {
		handleError(ctx, err, w, "failed to get cluster data")
		return
	} else if cd != nil {
		handleError(ctx, err, w, "cluster data cannot be set unless it is nil")
		return
	}
	newCDBytes, err := io.ReadAll(r.Body)
	if err != nil {
		handleError(ctx, err, w, "failed to get cluster data definition")
		return
	}
	defer func() {
		if err := r.Body.Close(); err != nil {
			logger.Error().AnErr("error", err).Msg("Failed to close Body")
		}
	}()
	var newCD apiv1.Data
	err = json.Unmarshal(newCDBytes, &newCD)
	if err != nil {
		handleError(ctx, err, w, "failed to parse new cluster data definition")
		return
	}
	err = h.client.PutClusterData(ctx, &newCD)
	if err != nil {
		handleError(ctx, err, w, "failed to write new cluster data definition")
	}
}
