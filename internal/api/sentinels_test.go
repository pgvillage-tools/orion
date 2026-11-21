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
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/consensus"
	mockconsensus "github.com/pgvillage-tools/orion/internal/mock/consensus"
)

var _ = Describe("sentinels", func() {
	errBoom := errors.New("boom")
	sentinels := apiv1.SentinelsInfo{{UID: "sentinel2"}, {UID: "sentinel1"}}

	type sentinelCase struct {
		sentinels apiv1.SentinelsInfo
		storeErr  error
		leader    string
		leaderErr error
		// readsLeader is false when the handler must stop before it asks for the leader
		readsLeader bool
		status      int
		list, one   string // body of GET /v1/cluster/sentinels and of GET /v1/cluster/sentinels/sentinel1
	}

	DescribeTable("SentinelsHandler and SentinelHandler",
		func(tc sentinelCase) {
			ctrl := gomock.NewController(GinkgoT())
			store := mockconsensus.NewMockStore(ctrl)
			store.EXPECT().GetSentinelsInfo(gomock.Any()).Return(tc.sentinels, tc.storeErr).Times(2)
			election := mockconsensus.NewMockElection(ctrl)
			if tc.readsLeader {
				// the election client is shared: every request reads the leader through it, nothing else
				election.EXPECT().Leader(gomock.Any()).Times(2).DoAndReturn(func(ctx context.Context) (string, error) {
					// the leader is read within the deadline of the request (the store timeout of the handlers)
					_, ok := ctx.Deadline()
					Expect(ok).To(BeTrue(), "expected the leader to be read with the request deadline")
					return tc.leader, tc.leaderErr
				})
			}
			var ready atomic.Bool
			h := NewHandlers(store, election, &ready)

			for _, req := range []struct {
				handler http.HandlerFunc
				uid     string
				body    string
			}{
				{h.SentinelsHandler, "", tc.list},
				{h.SentinelHandler, "sentinel1", tc.one},
			} {
				r := httptest.NewRequest(http.MethodGet, "/v1/cluster/sentinels", nil)
				r.SetPathValue("uid", req.uid)
				w := httptest.NewRecorder()
				req.handler(w, r)

				status := tc.status
				if status == http.StatusOK && req.uid != "" && req.body == "" {
					status = http.StatusNotFound
				}
				Expect(w.Code).To(Equal(status), "body: %s", w.Body)
				if status == http.StatusOK {
					Expect(strings.TrimSpace(w.Body.String())).To(Equal(req.body))
				}
			}
		},
		Entry("a leader", sentinelCase{
			sentinels: sentinels, leader: "sentinel1", readsLeader: true, status: http.StatusOK,
			list: `[{"uid":"sentinel1","leader":true},{"uid":"sentinel2","leader":false}]`,
			one:  `{"uid":"sentinel1","leader":true}`,
		}),
		Entry("no leader", sentinelCase{
			sentinels: sentinels, leaderErr: consensus.ErrElectionNoLeader, readsLeader: true, status: http.StatusOK,
			list: `[{"uid":"sentinel1","leader":false},{"uid":"sentinel2","leader":false}]`,
			one:  `{"uid":"sentinel1","leader":false}`,
		}),
		Entry("a leader that is not a known sentinel", sentinelCase{
			sentinels: sentinels, leader: "gone", readsLeader: true, status: http.StatusOK,
			list: `[{"uid":"sentinel1","leader":false},{"uid":"sentinel2","leader":false}]`,
			one:  `{"uid":"sentinel1","leader":false}`,
		}),
		Entry("no sentinels", sentinelCase{
			readsLeader: true, status: http.StatusOK, list: `[]`,
		}),
		Entry("the leader cannot be read", sentinelCase{
			sentinels: sentinels, leaderErr: errBoom, readsLeader: true, status: http.StatusInternalServerError,
		}),
		Entry("the sentinels cannot be read", sentinelCase{
			storeErr: errBoom, status: http.StatusInternalServerError,
		}),
	)
})
