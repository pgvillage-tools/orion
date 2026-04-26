package main

import (
	"context"
	"net/http"
	"time"
)

func (h *Handlers) FailKeeperRoutes() []Route {
	return []Route{
		{"PUT /clusterdata/keepers/{id}", h.PutFailKeeperHandlerHandler},
	}
}

// PutFailKeeperHandlerHandler endpoint
func (h *Handlers) PutFailKeeperHandlerHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(context.TODO(), time.Now().Add(time.Second))
	defer cancelFunc()

	keeperID := r.PathValue("id")

	cd, pair, err := h.client.GetClusterData(ctx)
	if err != nil {
		handleError(ctx, err, w, "failed to get cluster data")
	} else if cd.Cluster == nil {
		handleError(ctx, err, w, "cluster is not set")
	} else if cd.Cluster.Spec == nil {
		handleError(ctx, err, w, "cluster spec is not set")
	}
	newCd := cd.DeepCopy()
	keeperInfo := newCd.Keepers[keeperID]
	if keeperInfo == nil {
		handleError(ctx, err, w, "keeper doesn't exist")
	}

	keeperInfo.Status.ForceFail = true

	_, err = h.client.AtomicPutClusterData(ctx, newCd, pair)
	if err != nil {
		handleError(ctx, err, w, "failed to store cluster data")
	}
	w.WriteHeader(http.StatusAccepted)
}
