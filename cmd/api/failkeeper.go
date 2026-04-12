package main

import (
	"context"
	"net/http"
	"time"

	"github.com/pgvillage-tools/stolon/internal/logging"
)

func (h *Handlers) FailKeeperRoutes() []Route {
	return []Route{
		{"PUT /clusterdata/keepers/{id}", h.PutFailKeeperHandlerHandler},
	}
}

// PutFailKeeperHandlerHandler endpoint
func (h *Handlers) PutFailKeeperHandlerHandler(w http.ResponseWriter, r *http.Request) {
	ctx, logger := logging.GetLogComponent(context.Background(), logging.WebApiComponent)
	ctx, cancelFunc := context.WithDeadline(ctx, time.Now().Add(time.Second))
	defer cancelFunc()

	keeperID := r.PathValue("id")

	cd, pair, err := h.client.GetClusterData(ctx)
	if err != nil {
		logger.Error().AnErr("error", err).Msg("failed to get cluster data")
		if _, err := w.Write([]byte("ERROR")); err != nil {
			logger.Error().AnErr("error", err).Msg("failed to return ERROR")
		}
		w.WriteHeader(http.StatusInternalServerError)
	} else if cd.Cluster == nil {
		logger.Error().AnErr("error", err).Msg("cluster is not set")
		if _, err := w.Write([]byte("ERROR")); err != nil {
			logger.Error().AnErr("error", err).Msg("failed to return ERROR")
		}
		w.WriteHeader(http.StatusInternalServerError)
	} else if cd.Cluster.Spec == nil {
		logger.Error().AnErr("error", err).Msg("cluster spec is not set")
		if _, err := w.Write([]byte("ERROR")); err != nil {
			logger.Error().AnErr("error", err).Msg("failed to return ERROR")
		}
		w.WriteHeader(http.StatusInternalServerError)
	}
	newCd := cd.DeepCopy()
	keeperInfo := newCd.Keepers[keeperID]
	if keeperInfo == nil {
		logger.Error().Msg("keeper doesn't exist")
		if _, err := w.Write([]byte("ERROR")); err != nil {
			logger.Error().AnErr("error", err).Msg("failed to return ERROR")
		}
		w.WriteHeader(http.StatusInternalServerError)
	}

	keeperInfo.Status.ForceFail = true

	_, err = h.client.AtomicPutClusterData(ctx, newCd, pair)
	if err != nil {
		logger.Error().Msg("failed to store cluster data")
		if _, err := w.Write([]byte("ERROR")); err != nil {
			logger.Error().AnErr("error", err).Msg("failed to return ERROR")
		}
		w.WriteHeader(http.StatusInternalServerError)
	}
	w.WriteHeader(http.StatusAccepted)
}
