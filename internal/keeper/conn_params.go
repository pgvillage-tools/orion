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

package keeper

import (
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	pg "github.com/pgvillage-tools/orion/internal/postgresql"
)

func (p *PostgresKeeper) getSUConnParams(db, followedDB *cluster.DB) pg.ConnParams {
	cp := pg.ConnParams{}.
		WithUser(p.pgSUUsername).
		WithHost(followedDB.Status.ListenAddress).
		WithSPort(followedDB.Status.Port).
		WithAppName(common.OrionName(db.UID)).
		WithDbName(defaultDatabase).
		// This is currently only used for pgRewind, which requires a SU (repluser might not be enough).
		// Pgrewind is the only feature using SU over remote connection
		// and with that the only type using SU with sslmode.
		// Therefore we have skipped extra config option for sslmode for SU,
		// and reuse config for sslmode for repl user instead.
		WithSSLMode(p.pgReplSslMode)

	if p.pgSUAuthMethod == authMd5 {
		cp = cp.WithPassword(p.pgSUPassword)
	}
	return cp
}

func (p *PostgresKeeper) getReplConnParams(db, followedDB *cluster.DB) pg.ConnParams {
	cp := pg.ConnParams{}.
		WithUser(p.pgReplUsername).
		WithHost(followedDB.Status.ListenAddress).
		WithSPort(followedDB.Status.Port).
		WithAppName(common.OrionName(db.UID)).
		WithSSLMode("prefer")
	if p.pgReplAuthMethod == authMd5 {
		cp = cp.WithPassword(p.pgReplPassword)
	}
	return cp
}

func (p *PostgresKeeper) getLocalConnParams() pg.ConnParams {
	cp := pg.ConnParams{}.
		WithUser(p.pgSUUsername).
		WithHost(common.PgUnixSocketDirectories).
		WithSPort(p.pgPort).
		WithDbName(defaultDatabase)
		// no sslmode defined since it's not needed and supported over unix sockets
	if p.pgSUAuthMethod == authMd5 {
		cp = cp.WithPassword(p.pgSUPassword)
	}
	return cp
}

func (p *PostgresKeeper) getLocalReplConnParams() pg.ConnParams {
	cp := pg.ConnParams{}.
		WithUser(p.pgReplUsername).
		WithHost(common.PgUnixSocketDirectories).
		WithSPort(p.pgPort)
		// no sslmode defined since it's not needed and supported over unix sockets

	if p.pgReplAuthMethod == authMd5 {
		cp = cp.WithPassword(p.pgReplPassword)
	}
	return cp
}
