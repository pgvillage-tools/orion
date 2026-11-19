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

package v1

import (
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/util"
)

// PostgresState defines the state of a PostgreSQL instance
type PostgresState struct {
	UID        string `json:"uid,omitempty"`
	Generation int64  `json:"generation,omitempty"`

	ListenAddress string `json:"listenAddress,omitempty"`
	Port          string `json:"port,omitempty"`

	Healthy bool `json:"healthy,omitempty"`

	SystemID         string                   `json:"systemID,omitempty"`
	TimelineID       uint64                   `json:"timelineID,omitempty"`
	XLogPos          uint64                   `json:"xLogPos,omitempty"`
	TimelinesHistory PostgresTimelinesHistory `json:"timelinesHistory,omitempty"`

	PGParameters        common.Parameters `json:"pgParameters,omitempty"`
	SynchronousStandbys []string          `json:"synchronousStandbys"`
	OlderWalFile        string            `json:"olderWalFile,omitempty"`
}

// DeepCopy returns a copy of the PostgresState resource
func (p *PostgresState) DeepCopy() (dc *PostgresState) {
	return util.DeepCopy(p)
}
