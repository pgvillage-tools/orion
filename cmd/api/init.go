package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
)

func (h *Handlers) InitRoutes() []Route {
	return []Route{
		{"POST /clusterdata/spec", h.PostInitHandler},
	}
}

// PostInitHandler endpoint
func (h *Handlers) PostInitHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(context.TODO(), time.Now().Add(time.Second))
	defer cancelFunc()

	cd, _, err := h.client.GetClusterData(ctx)
	if err != nil {
		handleError(ctx, err, w, "failed to get cluster data")
	} else if cd.Cluster != nil {
		handleError(ctx, err, w, "cluster is already set")
	}

	var cs *apiv1.Spec

	newCSBytes, err := io.ReadAll(r.Body)
	if err != nil {
		handleError(ctx, err, w, "failed to get spec definition")
		return
	}
	defer func() {
		if err := r.Body.Close(); err != nil {
			handleError(ctx, err, w, "Failed to close Body")
		}
	}()
	if len(newCSBytes) == 0 {
		cs = &apiv1.Spec{}
		newCluster := apiv1.New
		cs.InitMode = &newCluster
	} else {
		err = json.Unmarshal(newCSBytes, &cs)
		if err != nil {
			handleError(ctx, err, w, "failed to parse spec definition")
			return
		}
	}

	if err := cs.Validate(); err != nil {
		handleError(ctx, err, w, "invalid spec")
		return
	}

	c := apiv1.NewCluster(common.UID(), cs)
	cd = apiv1.NewClusterData(c)

	// We ignore if cd has been modified between reading and writing
	if err := h.client.PutClusterData(ctx, cd); err != nil {
		handleError(ctx, err, w, "cannot update spec")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
