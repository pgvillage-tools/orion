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

	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// IsUnavailable tells if err means that the store didn't answer in time or can't serve requests right now (e.g. etcd
// timed out or has no leader, the kubernetes api server timed out or is overloaded): the same request may succeed
// when it is retried. A write that failed like this may still have been applied.
func IsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// etcd: the client turns gRPC errors into EtcdErrors, e.g. "etcdserver: request timed out" (Unavailable)
	var etcdErr rpctypes.EtcdError
	if errors.As(err, &etcdErr) {
		return unavailableCode(etcdErr.Code())
	}
	if s, ok := status.FromError(err); ok && s.Code() != codes.Unknown {
		return unavailableCode(s.Code())
	}
	return apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err) || apierrors.IsServiceUnavailable(err) ||
		apierrors.IsTooManyRequests(err)
}

func unavailableCode(code codes.Code) bool {
	return code == codes.Unavailable || code == codes.DeadlineExceeded
}
