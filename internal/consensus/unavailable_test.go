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

package consensus

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIsUnavailable(t *testing.T) {
	lease := schema.GroupResource{Group: "coordination.k8s.io", Resource: "leases"}
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "no error"},
		{name: "another error", err: errors.New("boom")},
		{name: "deadline exceeded", err: context.DeadlineExceeded, want: true},
		{name: "canceled", err: context.Canceled},
		{name: "etcd request timed out", err: rpctypes.ErrTimeout, want: true},
		{name: "etcd no leader", err: rpctypes.ErrNoLeader, want: true},
		{name: "etcd leader changed", err: rpctypes.ErrLeaderChanged, want: true},
		{name: "etcd key not found", err: rpctypes.ErrKeyNotFound},
		{name: "etcd permission denied", err: rpctypes.ErrPermissionDenied},
		{name: "grpc unavailable", err: status.Error(codes.Unavailable, "connection refused"), want: true},
		{name: "grpc deadline exceeded", err: status.Error(codes.DeadlineExceeded, "slow"), want: true},
		{name: "grpc invalid argument", err: status.Error(codes.InvalidArgument, "bad")},
		{name: "kubernetes timeout", err: apierrors.NewTimeoutError("slow", 1), want: true},
		{name: "kubernetes server timeout", err: apierrors.NewServerTimeout(lease, "update", 1), want: true},
		{name: "kubernetes service unavailable", err: apierrors.NewServiceUnavailable("busy"), want: true},
		{name: "kubernetes too many requests", err: apierrors.NewTooManyRequests("busy", 1), want: true},
		{name: "kubernetes conflict", err: apierrors.NewConflict(lease, "orion", errors.New("modified"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsUnavailable(tc.err); got != tc.want {
				t.Errorf("IsUnavailable(%v) = %v, want %v", tc.err, got, tc.want)
			}
			// also when it is wrapped, as the api wraps store errors
			if tc.err != nil {
				wrapped := fmt.Errorf("cannot store updated cluster spec: %w", tc.err)
				if got := IsUnavailable(wrapped); got != tc.want {
					t.Errorf("IsUnavailable(wrapped %v) = %v, want %v", tc.err, got, tc.want)
				}
			}
		})
	}
}
