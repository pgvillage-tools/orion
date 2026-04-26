package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	v1 "github.com/pgvillage-tools/orion/api/v1"
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
	ctx, logger := logging.GetLogComponent(context.Background(), logging.WebApiComponent)
	ctx, cancelFunc := context.WithDeadline(ctx, time.Now().Add(time.Second))
	defer cancelFunc()

	if cd, _, err := h.client.GetClusterData(ctx); err != nil {
		logger.Error().AnErr("error", err).Msg("failed to get cluster data")
		if _, err := w.Write([]byte("ERROR")); err != nil {
			logger.Error().AnErr("error", err).Msg("unable to return cluster data")
		}
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		h.writeJSON(ctx, w, cd)
	}
}

// PutClusterDataHandler endpoint
func (h *Handlers) PutClusterDataHandler(w http.ResponseWriter, r *http.Request) {
	ctx, logger := logging.GetLogComponent(context.Background(), logging.WebApiComponent)
	ctx, cancelFunc := context.WithDeadline(ctx, time.Now().Add(time.Second))
	defer cancelFunc()

	if cd, _, err := h.client.GetClusterData(ctx); err != nil {
		logger.Error().AnErr("error", err).Msg("failed to get cluster data")
		if _, err := w.Write([]byte("ERROR")); err != nil {
			logger.Error().AnErr("error", err).Msg("unable to return cluster data")
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	} else if cd != nil {
		logger.Error().Msg("cluster data cannot be set unless it is nil")
		if _, err := w.Write([]byte("ERROR")); err != nil {
			logger.Error().AnErr("error", err).Msg("cluster data is already set")
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	newCDBytes, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error().AnErr("error", err).Msg("failed to get cluster data definition")
		if _, err := w.Write([]byte("ERROR")); err != nil {
			logger.Error().AnErr("error", err).Msg("unable to get new cluster data definition")
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()
	var newCD v1.Data
	err = json.Unmarshal(newCDBytes, &newCD)
	if err != nil {
		logger.Error().AnErr("error", err).Msg("failed to parse new cluster data definition")
		if _, err := w.Write([]byte("ERROR")); err != nil {
			logger.Error().AnErr("error", err).Msg("unable to parse new cluster data definition")
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	err = h.client.PutClusterData(ctx, &newCD)
	if err != nil {
		logger.Error().AnErr("error", err).Msg("failed to write new cluster data definition")
		if _, err := w.Write([]byte("ERROR")); err != nil {
			logger.Error().AnErr("error", err).Msg("unable to write new cluster data definition")
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
