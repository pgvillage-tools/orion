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

package tcpproxy

import (
	"context"
	"fmt"

	"github.com/pgvillage-tools/orion/internal/logging"
)

// Serve accepts connections until ctx is done (it returns nil then) or accepting fails. It closes the listener, and
// all connections before it returns.
func (p *Proxy) Serve(ctx context.Context) error {
	_, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	stopListening := context.AfterFunc(ctx, func() { _ = p.listener.Close() })
	defer func() {
		stopListening()
		_ = p.listener.Close()
		p.mutex.Lock()
		p.connCancel()
		p.mutex.Unlock()
		p.conns.Wait()
	}()

	for {
		conn, err := p.listener.AcceptTCP()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept error: %w", err)
		}
		connLogger := logger.With().Stringer("client", conn.RemoteAddr()).Logger()
		if err = conn.SetKeepAliveConfig(p.keepAlive); err != nil {
			// a client that is gone already, most likely: it shouldn't stop the proxy for the other clients
			connLogger.Warn().AnErr(logging.KeyErr, err).Msg("cannot set the tcp keepalive, closing the connection")
			_ = conn.Close()
			continue
		}
		p.mutex.Lock()
		dest, connCtx := p.dest, p.connCtx
		p.conns.Add(1)
		p.mutex.Unlock()
		go func() {
			defer p.conns.Done()
			proxyConn(connCtx, &connLogger, conn, dest)
		}()
	}
}
