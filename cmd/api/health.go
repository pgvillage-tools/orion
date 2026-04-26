package main

import (
	"context"
	"errors"
	"net/http"
	"time"
)

func (h *Handlers) HealthRoutes() []Route {
	return []Route{
		{"GET /healthz", h.HealthzHandler},
		{"GET /readyz", h.ReadyzHandler},
	}
}

// Health endpoints
func (h *Handlers) HealthzHandler(w http.ResponseWriter, r *http.Request) {
	var (
		status = http.StatusOK
		msg    = []byte("OK")
	)
	ctx, cancelFunc := context.WithDeadline(context.TODO(), time.Now().Add(time.Second))
	defer cancelFunc()

	if err := h.client.Healthy(ctx); err != nil {
		handleError(ctx, err, w, "store is not healthy")
	}
	w.WriteHeader(status)
	if _, err := w.Write(msg); err != nil {
		handleError(ctx, err, w, "unable to return healthz response")
	}
}

func (h *Handlers) ReadyzHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.TODO()
	if !h.Ready.Load() {
		handleError(ctx, errors.New("Service not ready yet"), w, "Not Ready")
		return
	}

	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("Ready"))

	if err != nil {
		handleError(ctx, err, w, "unable to return readyz response")
	}
}
