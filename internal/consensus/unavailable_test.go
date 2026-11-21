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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ = Describe("IsUnavailable", func() {
	lease := schema.GroupResource{Group: "coordination.k8s.io", Resource: "leases"}

	DescribeTable("tells an unavailable consensus store from other errors",
		func(err error, want bool) {
			Ω(IsUnavailable(err)).Should(Equal(want), "IsUnavailable(%v)", err)
			// also when it is wrapped, as the api wraps store errors
			if err != nil {
				wrapped := fmt.Errorf("cannot store updated cluster spec: %w", err)
				Ω(IsUnavailable(wrapped)).Should(Equal(want), "IsUnavailable(wrapped %v)", err)
			}
		},
		Entry("no error", nil, false),
		Entry("another error", errors.New("boom"), false),
		Entry("deadline exceeded", context.DeadlineExceeded, true),
		Entry("canceled", context.Canceled, false),
		Entry("etcd request timed out", rpctypes.ErrTimeout, true),
		Entry("etcd no leader", rpctypes.ErrNoLeader, true),
		Entry("etcd leader changed", rpctypes.ErrLeaderChanged, true),
		Entry("etcd key not found", rpctypes.ErrKeyNotFound, false),
		Entry("etcd permission denied", rpctypes.ErrPermissionDenied, false),
		Entry("grpc unavailable", status.Error(codes.Unavailable, "connection refused"), true),
		Entry("grpc deadline exceeded", status.Error(codes.DeadlineExceeded, "slow"), true),
		Entry("grpc invalid argument", status.Error(codes.InvalidArgument, "bad"), false),
		Entry("kubernetes timeout", apierrors.NewTimeoutError("slow", 1), true),
		Entry("kubernetes server timeout", apierrors.NewServerTimeout(lease, "update", 1), true),
		Entry("kubernetes service unavailable", apierrors.NewServiceUnavailable("busy"), true),
		Entry("kubernetes too many requests", apierrors.NewTooManyRequests("busy", 1), true),
		Entry("kubernetes conflict", apierrors.NewConflict(lease, "orion", errors.New("modified")), false),
	)
})
