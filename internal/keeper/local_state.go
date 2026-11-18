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
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/pgvillage-tools/orion/internal/common"
)

// LocalState can be used to define the local state for a keep process
type LocalState struct {
	UID        string
	ClusterUID string
}

func (p *PostgresKeeper) dbLocalStateCopy() *DBLocalState {
	p.localStateMutex.Lock()
	defer p.localStateMutex.Unlock()
	return p.dbLocalState.DeepCopy()
}

func (p *PostgresKeeper) keeperLocalStateFilePath() string {
	return filepath.Join(p.cfg.DataDir, "keeperstate")
}

func (p *PostgresKeeper) loadKeeperLocalState() error {
	sj, err := os.ReadFile(p.keeperLocalStateFilePath())
	if err != nil {
		return err
	}
	var s *LocalState
	if err := json.Unmarshal(sj, &s); err != nil {
		return err
	}
	p.keeperLocalState = s
	return nil
}

func (p *PostgresKeeper) saveKeeperLocalState() error {
	sj, err := json.Marshal(p.keeperLocalState)
	if err != nil {
		return err
	}
	return common.WriteFileAtomic(p.keeperLocalStateFilePath(), ownerRWPermisions, sj)
}

func (p *PostgresKeeper) dbLocalStateFilePath() string {
	return filepath.Join(p.cfg.DataDir, "dbstate")
}

func (p *PostgresKeeper) loadDBLocalState() error {
	sj, err := os.ReadFile(p.dbLocalStateFilePath())
	if err != nil {
		return err
	}
	var s *DBLocalState
	if err := json.Unmarshal(sj, &s); err != nil {
		return err
	}
	p.dbLocalState = s
	return nil
}

// saveDBLocalState saves on disk the dbLocalState and only if successful
// updates the current in memory state
func (p *PostgresKeeper) saveDBLocalState(dbls *DBLocalState) error {
	sj, err := json.Marshal(dbls)
	if err != nil {
		return err
	}
	if err = common.WriteFileAtomic(p.dbLocalStateFilePath(), ownerRWPermisions, sj); err != nil {
		return err
	}

	p.localStateMutex.Lock()
	p.dbLocalState = dbls.DeepCopy()
	p.localStateMutex.Unlock()

	return nil
}
