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

package keeper

import (
	"context"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/pgvillage-tools/orion/internal/common"
	pg "github.com/pgvillage-tools/orion/internal/postgresql"
)

// pgManager is what the keeper uses of a *pg.Manager to control its PostgreSQL instance; the unit tests use a fake.
type pgManager interface {
	pg.PGManager

	// configuration
	SetParameters(parameters common.Parameters)
	CurParameters() common.Parameters
	SetRecoveryOptions(recoveryOptions *pg.RecoveryOptions)
	CurRecoveryOptions() *pg.RecoveryOptions
	SetHba(hba []string)
	CurHba() []string
	GetConfigFilePGParameters(ctx context.Context) (common.Parameters, error)
	IsRestartRequired(ctx context.Context, changedParams []string) (bool, error)

	// the data directory
	IsInitialized() (bool, error)
	Init(ctx context.Context, initConfig *pg.InitConfig) error
	Restore(ctx context.Context, command string) error
	RemoveAllIfInitialized(ctx context.Context) error
	SyncFromFollowed(ctx context.Context, followedConnParams pg.ConnParams, replSlot string) error
	SyncFromFollowedPGRewind(ctx context.Context, followedConnParams pg.ConnParams, password string) error

	// the instance
	Start(ctx context.Context) error
	StartTmpMerged(ctx context.Context) error
	StopIfStarted(ctx context.Context, fast bool) error
	Restart(ctx context.Context, fast bool) error
	Reload(ctx context.Context) error
	IsStarted() (bool, error)
	WaitReady(ctx context.Context, timeout time.Duration) error
	WaitRecoveryDone(timeout time.Duration) error
	Promote(ctx context.Context) error
	SetupRoles(ctx context.Context) error

	// state
	BinaryVersion() (*semver.Version, error)
	GetRole() (common.Role, error)
	GetSystemID() (string, error)
	GetSystemData(ctx context.Context) (*pg.SystemData, error)
	GetSyncStandbys(ctx context.Context) ([]string, error)
	OlderWalFile() (string, error)

	// replication slots
	GetReplicationSlots(ctx context.Context) ([]string, error)
	CreateReplicationSlot(ctx context.Context, name string) error
	DropReplicationSlot(ctx context.Context, name string) error
}

var _ pgManager = (*pg.Manager)(nil)
