package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	v1 "github.com/pgvillage-tools/stolon/api/v1"
	"github.com/pgvillage-tools/stolon/internal/common"
	"github.com/pgvillage-tools/stolon/internal/logging"
)

func (h *Handlers) InitRoutes() []Route {
	return []Route{
		{"POST /clusterdata/spec", h.PostInitHandler},
	}
}

// PostInitHandler endpoint
func (h *Handlers) PostInitHandler(w http.ResponseWriter, r *http.Request) {
	ctx, logger := logging.GetLogComponent(context.Background(), logging.WebApiComponent)
	ctx, cancelFunc := context.WithDeadline(ctx, time.Now().Add(time.Second))
	defer cancelFunc()

	cd, _, err := h.client.GetClusterData(ctx)
	if err != nil {
		logger.Error().AnErr("error", err).Msg("failed to get cluster data")
		if _, err := w.Write([]byte("ERROR")); err != nil {
			logger.Error().AnErr("error", err).Msg("failed to return ERROR")
		}
		w.WriteHeader(http.StatusInternalServerError)
	} else if cd.Cluster != nil {
		logger.Error().AnErr("error", err).Msg("cluster is already set")
		if _, err := w.Write([]byte("ERROR")); err != nil {
			logger.Error().AnErr("error", err).Msg("failed to return ERROR")
		}
		w.WriteHeader(http.StatusInternalServerError)
	}

	var cs *v1.Spec

	newCSBytes, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error().AnErr("error", err).Msg("failed to get spec definition")
		if _, err := w.Write([]byte("ERROR")); err != nil {
			logger.Error().AnErr("error", err).Msg("failed to return ERROR")
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()
	if len(newCSBytes) == 0 {
		cs = &v1.Spec{}
		newCluster := v1.New
		cs.InitMode = &newCluster
	} else {
		err = json.Unmarshal(newCSBytes, &cs)
		if err != nil {
			logger.Error().AnErr("error", err).Msg("failed to parse spec definition")
			if _, err := w.Write([]byte("ERROR")); err != nil {
				logger.Error().AnErr("error", err).Msg("failed to return ERROR")
			}
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	if err := cs.Validate(); err != nil {
		logger.Error().AnErr("error", err).Msg("invalid spec")
		if _, err := w.Write([]byte("ERROR")); err != nil {
			logger.Error().AnErr("error", err).Msg("failed to return ERROR")
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	c := v1.NewCluster(common.UID(), cs)
	cd = v1.NewClusterData(c)

	// We ignore if cd has been modified between reading and writing
	if err := h.client.PutClusterData(ctx, cd); err != nil {
		logger.Error().AnErr("error", err).Msg("cannot update spec")
		if _, err := w.Write([]byte("ERROR")); err != nil {
			logger.Error().AnErr("error", err).Msg("failed to return ERROR")
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
