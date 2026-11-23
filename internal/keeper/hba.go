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
	"context"
	"fmt"
	"sort"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/logging"
)

// IsMaster return if the db is the cluster master db.
// A master is a db that:
// * Has a master db role
// or
// * Has a standby db role with followtype external
func IsMaster(db *cluster.DB) bool {
	switch db.Spec.Role {
	case common.RolePrimary:
		return true
	case common.RoleReplica:
		if db.Spec.FollowConfig.Type == cluster.FollowTypeExternal {
			return true
		}
		return false
	default:
		panic("invalid db role in db Spec")
	}
}

// localAuthMethod returns the authentication method that works for local connections
// cert does not work for local connections, in which case we should fall back to peer authentication
func localAuthMethod(ctx context.Context, authMethod string) string {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	if authMethod == authCert {
		logger.Debug().Msg("using peer instead of cert for local connection authentication method")
		return authPeer
	}
	return authMethod
}

// generateHBA generates the instance hba entries depending on the value of
// DefaultSUReplAccessMode.
// When onlyInternal is true only rules needed for replication will be setup
// and the traffic should be permitted only for pgSUUsername standard
// connections and pgReplUsername replication connections.
func (p *PostgresKeeper) generateHBA(ctx context.Context, cd *cluster.Data, db *cluster.DB,
	onlyInternal bool) []string {
	// Minimal entries for local normal and replication connections needed by the orion keeper
	// Matched local connections are for postgres database and suUsername user with authMd5 auth
	// Matched local replication connections are for replUsername user with authMd5 auth
	computedHBA := []string{
		fmt.Sprintf("local postgres %s %s", p.pgSUUsername, localAuthMethod(ctx, p.pgSULocalAuthMethod)),
		fmt.Sprintf("local replication %s %s", p.pgReplUsername, localAuthMethod(ctx, p.pgReplLocalAuthMethod)),
	}

	switch *cd.Cluster.DefSpec().DefaultSUReplAccessMode {
	case cluster.SUReplAccessAll:
		// all the keepers will accept connections from every host
		computedHBA = append(
			computedHBA,
			fmt.Sprintf("%s all %s %s %s", p.pgSUConnType, p.pgSUUsername, "0.0.0.0/0", p.pgSUAuthMethod),
			fmt.Sprintf("%s all %s %s %s", p.pgSUConnType, p.pgSUUsername, "::0/0", p.pgSUAuthMethod),
			fmt.Sprintf("%s replication %s %s %s", p.pgReplConnType, p.pgReplUsername, "0.0.0.0/0", p.pgReplAuthMethod),
			fmt.Sprintf("%s replication %s %s %s", p.pgReplConnType, p.pgReplUsername, "::0/0", p.pgReplAuthMethod),
		)
	case cluster.SUReplAccessStrict:
		// only the master keeper (primary instance or standby of a remote primary when in standby cluster mode) will
		// accept connections only from the other standby keepers IPs
		if IsMaster(db) {
			addresses := []string{}
			for _, dbElt := range cd.DBs {
				if dbElt.UID != db.UID {
					addresses = append(addresses, dbElt.Status.ListenAddress)
				}
			}
			sort.Strings(addresses)
			for _, address := range addresses {
				computedHBA = append(
					computedHBA,
					fmt.Sprintf(
						"%s all %s %s/32 %s",
						p.pgSUConnType,
						p.pgSUUsername,
						address,
						p.pgReplAuthMethod),
					fmt.Sprintf(
						"%s replication %s %s/32 %s",
						p.pgReplConnType,
						p.pgReplUsername,
						address,
						p.pgReplAuthMethod),
				)
			}
		}
	}

	if !onlyInternal {
		// By default, if no custom pg_hba entries are provided, accept
		// connections for all databases and users with authMd5 auth
		if db.Spec.PGHBA != nil {
			computedHBA = append(computedHBA, db.Spec.PGHBA...)
		} else {
			computedHBA = append(
				computedHBA,
				fmt.Sprintf("%s all all 0.0.0.0/0 %s", p.pgSUConnType, p.pgSUAuthMethod),
				fmt.Sprintf("%s all all ::0/0 %s", p.pgSUConnType, p.pgSUAuthMethod),
			)
		}
	}

	// return generated Hba merged with user Hba
	return computedHBA
}
