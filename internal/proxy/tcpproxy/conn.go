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
	"io"
	"net"
	"sync"

	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/rs/zerolog"
)

// proxyConn forwards conn to dest until either side closes, or connCtx is done; it closes conn
func proxyConn(connCtx context.Context, logger *zerolog.Logger, conn net.Conn, dest *net.TCPAddr) {
	defer func() { _ = conn.Close() }()
	if dest == nil {
		logger.Debug().Msg("no destination, closing the connection")
		return
	}

	var dialer net.Dialer
	destConn, err := dialer.DialContext(connCtx, "tcp", dest.String())
	if err != nil {
		event := logger.Warn()
		if connCtx.Err() != nil {
			// the destination changed while connecting: not a failure of the destination
			event = logger.Debug()
		}
		event.AnErr(logging.KeyErr, err).Stringer("destination", dest).
			Msg("cannot connect to the destination, closing the connection")
		return
	}
	logger.Debug().Stringer("destination", dest).Msg("proxying connection")

	var toDest, toClient int64
	var wg sync.WaitGroup
	// copied has room for both copies, so the second one doesn't block
	copied := make(chan struct{}, 2)
	pipe := func(dst, src net.Conn, n *int64) {
		defer wg.Done()
		*n, _ = io.Copy(dst, src)
		copied <- struct{}{}
	}
	wg.Add(2)
	go pipe(destConn, conn, &toDest)
	go pipe(conn, destConn, &toClient)

	reason := "connection ended"
	select {
	case <-copied:
	case <-connCtx.Done():
		reason = "destination changed or proxy stopped"
	}
	// closing both sides ends the other copy too
	_ = conn.Close()
	_ = destConn.Close()
	wg.Wait()
	logger.Debug().Stringer("destination", dest).Str("reason", reason).
		Int64("bytes to destination", toDest).Int64("bytes to client", toClient).
		Msg("closed the connection")
}
