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

package proxy

import (
	"context"
	"fmt"
	"time"

	"github.com/pgvillage-tools/orion/internal/logging"
)

// fence closes all connections and, with stopListening, stops listening (for example to avoid load balancers forward
// connections to us since we aren't ready or in a bad state): without a successful check the proxy can't know whether
// its master is still the master
func (c *ClusterChecker) fence(ctx context.Context) {
	_, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	logger.Warn().Msg("check timeout timer fired: closing all connections")
	c.setDestination(nil)
	if c.stopListening {
		c.stopServing(ctx)
	}
}

// timeout returns the proxy timeout
func (c *ClusterChecker) timeout() time.Duration {
	c.configMutex.Lock()
	defer c.configMutex.Unlock()
	return c.proxyTimeout
}

// Start runs the checks of the proxy until ctx is done or the proxy fails. When no check succeeded for the proxy
// timeout (a check failed, or still runs), the proxy fences itself; a check that runs longer than the proxy timeout is
// canceled.
func (c *ClusterChecker) Start(ctx context.Context) error {
	_, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	// buffered, so a check that ends after Start returned doesn't block
	checkCh := make(chan error, 1)
	timerCh := time.NewTimer(0).C
	fenceTimer := time.NewTimer(c.timeout())
	defer fenceTimer.Stop()

	for {
		select {
		case <-timerCh:
			go func() {
				checkCtx, cancel := context.WithTimeout(ctx, c.timeout())
				defer cancel()
				checkCh <- c.Check(checkCtx)
			}()
		case err := <-checkCh:
			if err != nil {
				// don't report check ok since it returned an error
				logger.Warn().AnErr(logging.KeyErr, err).Msg("check function error")
			} else {
				logger.Debug().Msg("check ok")
				// the check was ok: the proxy timeout starts again (from the timeout of the cluster spec)
				fenceTimer.Reset(c.timeout())
			}
			c.configMutex.Lock()
			timerCh = time.NewTimer(c.proxyCheckInterval).C
			c.configMutex.Unlock()

		case <-fenceTimer.C:
			c.fence(ctx)

		case err := <-c.proxyErrCh:
			return fmt.Errorf("proxy error: %v", err)
		case <-ctx.Done():
			c.stopServing(ctx)
			return nil
		}
	}
}
