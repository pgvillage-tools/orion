// Copyright 2026 PgVillage
// Copyright 2015 Sorint.lab
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

// Package proxy holds the orion proxy: it sends the connections it accepts to the current primary, and stops
// serving when it can't check the cluster in the store
package proxy

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Run runs a proxy with cfg until ctx is done or the proxy fails
func Run(ctx context.Context, cfg Config) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	uid := common.UID()
	logger.Info().Str("uid", uid).Msg("proxy uid")

	if cfg.MetricsListenAddress != "" {
		http.Handle("/metrics", promhttp.Handler())
		go func() {
			err := http.ListenAndServe(cfg.MetricsListenAddress, nil)
			if err != nil {
				logger.Fatal().AnErr(logging.KeyErr, err).Msg("metrics http server error")
			}
		}()
	}

	clusterChecker, err := NewClusterChecker(ctx, uid, cfg)
	if err != nil {
		return fmt.Errorf("cannot create cluster checker: %v", err)
	}
	if err = clusterChecker.Start(ctx); err != nil {
		return fmt.Errorf("cluster checker ended with error: %v", err)
	}
	return nil
}
