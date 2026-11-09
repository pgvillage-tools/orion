// Copyright 2026 PgVillage
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	"context"
	"net/http"
	"time"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// MemberRoutes returns the routes for the keepers (as a list), and for a single keeper, proxy or sentinel by uid
func (h *Handlers) MemberRoutes() []Route {
	return []Route{
		{endpoints.KeepersEndPoint.Route(endpoints.MethodGet), h.KeepersHandler},
		{endpoints.KeeperEndPoint.Route(endpoints.MethodGet), h.KeeperHandler},
		{endpoints.ProxyEndPoint.Route(endpoints.MethodGet), h.ProxyHandler},
		{endpoints.SentinelEndPoint.Route(endpoints.MethodGet), h.SentinelHandler},
	}
}

// keepers returns the keepers of the cluster data, sorted on uid (an empty list when there is no cluster data)
func (h *Handlers) keepers(ctx context.Context) (apiv1.InfoKeepers, error) {
	cd, _, err := h.client.GetClusterData(ctx)
	if err != nil {
		return nil, err
	}
	if cd == nil {
		return apiv1.InfoKeepers{}, nil
	}
	keepers, err := h.keepersInfo(ctx, cd)
	if keepers == nil {
		keepers = apiv1.InfoKeepers{}
	}
	return keepers, err
}

// KeepersHandler returns the keepers, sorted on uid (the same list as in the cluster status)
func (h *Handlers) KeepersHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(cfg.StoreTimeout))
	defer cancelFunc()

	keepers, err := h.keepers(ctx)
	if err != nil {
		handleError(ctx, err, w, "failed to get the keepers")
		return
	}
	h.writeJSON(ctx, w, keepers)
}

// KeeperHandler returns the keeper with the uid in the path, or 404
func (h *Handlers) KeeperHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(cfg.StoreTimeout))
	defer cancelFunc()

	keepers, err := h.keepers(ctx)
	if err != nil {
		handleError(ctx, err, w, "failed to get the keepers")
		return
	}
	writeMember(ctx, h, w, "keeper", r.PathValue("id"), keepers, func(k apiv1.InfoKeeper) string { return k.UID })
}

// ProxyHandler returns the proxy with the uid in the path, or 404
func (h *Handlers) ProxyHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(cfg.StoreTimeout))
	defer cancelFunc()

	proxies, err := h.proxyInfo(ctx)
	if err != nil {
		handleError(ctx, err, w, "failed to get the proxies")
		return
	}
	writeMember(ctx, h, w, "proxy", r.PathValue("uid"), proxies, func(p apiv1.InfoProxy) string { return p.UID })
}

// SentinelHandler returns the sentinel with the uid in the path, or 404
func (h *Handlers) SentinelHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancelFunc := context.WithDeadline(r.Context(), time.Now().Add(cfg.StoreTimeout))
	defer cancelFunc()

	sentinels, err := h.sentinelInfo(ctx)
	if err != nil {
		handleError(ctx, err, w, "failed to get the sentinels")
		return
	}
	writeMember(ctx, h, w, "sentinel", r.PathValue("uid"), sentinels,
		func(s apiv1.InfoSentinel) string { return s.UID })
}

// writeMember responds with the member of members whose uid is uid, or with 404 when there is none
func writeMember[T any](ctx context.Context, h *Handlers, w http.ResponseWriter, kind, uid string, members []T,
	uidOf func(T) string) {
	for _, m := range members {
		if uidOf(m) == uid {
			h.writeJSON(ctx, w, m)
			return
		}
	}
	endpoints.WriteProblem(w, http.StatusNotFound, kind+" "+uid+" doesn't exist")
}
