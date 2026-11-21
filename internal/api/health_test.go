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

package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mockconsensus "github.com/pgvillage-tools/orion/internal/mock/consensus"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

var _ = Describe("health", func() {
	It("livez returns 200 OK", func() {
		// liveness doesn't look at readiness or the store: a nil store would panic if it did
		var notReady atomic.Bool
		w := httptest.NewRecorder()
		NewHandlers(nil, nil, &notReady).LivezHandler(w, httptest.NewRequest(http.MethodGet, "/livez", nil))
		Expect(w.Code).To(Equal(http.StatusOK))
		Expect(w.Body.String()).To(Equal("OK"))
	})

	DescribeTable("readyz",
		func(ready bool, storeErr error, status int, body string) {
			var isReady atomic.Bool
			isReady.Store(ready)
			store := mockconsensus.NewMockStore(gomock.NewController(GinkgoT()))
			if ready {
				store.EXPECT().Healthy(gomock.Any()).Return(storeErr)
			}
			w := httptest.NewRecorder()
			NewHandlers(store, nil, &isReady).ReadyzHandler(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			Expect(w.Code).To(Equal(status))
			Expect(w.Body.String()).To(Equal(body))
		},
		Entry("starting or shutting down", false, nil, http.StatusServiceUnavailable, "Not Ready"),
		Entry("ready, the store answers", true, nil, http.StatusOK, "Ready"),
		Entry("ready, the store doesn't answer", true, errors.New("context deadline exceeded"),
			http.StatusServiceUnavailable, "Not Ready"),
	)

	It("shuts down after the shutdown delay, returning 503 on readyz during the delay", func(ctx SpecContext) {
		const delay = 200 * time.Millisecond
		var ready atomic.Bool
		ready.Store(true)

		store := mockconsensus.NewMockStore(gomock.NewController(GinkgoT()))
		store.EXPECT().Healthy(gomock.Any()).Return(nil).AnyTimes()
		h := NewHandlers(store, nil, &ready)
		probe := httptest.NewServer(newMux(ctx, h.ProbeRoutes(http.NotFoundHandler())))
		DeferCleanup(probe.Close)
		srv := httptest.NewServer(http.NotFoundHandler())
		DeferCleanup(srv.Close)

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
			defer GinkgoRecover()
			shutdown(ctx, &ready, delay, srv.Config, probe.Config)
			close(done)
		}()
		// during the delay, readyz returns 503 and the servers still run
		time.Sleep(50 * time.Millisecond)
		code, err := readyz()
		Expect(err).NotTo(HaveOccurred(), "expected 503 during the shutdown delay")
		Expect(code).To(Equal(http.StatusServiceUnavailable), "expected 503 during the shutdown delay")
		Eventually(done).WithTimeout(5*time.Second).Should(BeClosed(), "shutdown didn't finish")
		_, err = readyz()
		Expect(err).To(HaveOccurred(), "expected the probe server to be stopped after the shutdown")
	})
})
