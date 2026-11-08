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
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	mockconsensus "github.com/pgvillage-tools/orion/internal/mock/consensus"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

func TestLivez(t *testing.T) {
	// liveness doesn't look at readiness or the store: a nil store would panic if it did
	var notReady atomic.Bool
	w := httptest.NewRecorder()
	NewHandlers(nil, &notReady).LivezHandler(w, httptest.NewRequest(http.MethodGet, "/livez", nil))
	if w.Code != http.StatusOK || w.Body.String() != "OK" {
		t.Errorf("expected 200 OK, got %d %q", w.Code, w.Body.String())
	}
}

func TestReadyz(t *testing.T) {
	for name, tc := range map[string]struct {
		ready    bool
		storeErr error
		status   int
		body     string
	}{
		"starting or shutting down": {ready: false, status: http.StatusServiceUnavailable, body: "Not Ready"},
		"ready, the store answers":  {ready: true, status: http.StatusOK, body: "Ready"},
		"ready, the store doesn't answer": {ready: true, storeErr: errors.New("context deadline exceeded"),
			status: http.StatusServiceUnavailable, body: "Not Ready"},
	} {
		t.Run(name, func(t *testing.T) {
			var ready atomic.Bool
			ready.Store(tc.ready)
			store := mockconsensus.NewMockStore(gomock.NewController(t))
			if tc.ready {
				store.EXPECT().Healthy(gomock.Any()).Return(tc.storeErr)
			}
			w := httptest.NewRecorder()
			NewHandlers(store, &ready).ReadyzHandler(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			if w.Code != tc.status || w.Body.String() != tc.body {
				t.Errorf("expected %d %q, got %d %q", tc.status, tc.body, w.Code, w.Body.String())
			}
		})
	}
}

func TestShutdown(t *testing.T) {
	delay := cfg.shutdownDelay
	cfg.shutdownDelay = 200 * time.Millisecond
	defer func() { cfg.shutdownDelay = delay }()
	ready.Store(true)
	defer ready.Store(false)

	store := mockconsensus.NewMockStore(gomock.NewController(t))
	store.EXPECT().Healthy(gomock.Any()).Return(nil).AnyTimes()
	h := NewHandlers(store, &ready)
	probe := httptest.NewServer(newMux(t.Context(), h.ProbeRoutes(http.NotFoundHandler())))
	defer probe.Close()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	readyz := func() (int, error) {
		resp, err := http.Get(probe.URL + "/" + string(endpoints.ReadyEndPoint))
		if err != nil {
			return 0, err
		}
		_ = resp.Body.Close()
		return resp.StatusCode, nil
	}
	done := make(chan struct{})
	go func() {
		shutdown(t.Context(), srv.Config, probe.Config)
		close(done)
	}()
	// during the delay, readyz returns 503 and the servers still run
	time.Sleep(50 * time.Millisecond)
	if code, err := readyz(); err != nil || code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 during the shutdown delay, got %d %v", code, err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown didn't finish")
	}
	if _, err := readyz(); err == nil {
		t.Error("expected the probe server to be stopped after the shutdown")
	}
}
