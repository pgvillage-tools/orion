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

// PostgresTimelinesHistory stores all PostgreSQL timelines belonging to this cluster
type PostgresTimelinesHistory []*PostgresTimelineHistory

// PostgresTimelineHistory defines a PostgreSQL timeline
type PostgresTimelineHistory struct {
	TimelineID  uint64 `json:"timelineID,omitempty"`
	SwitchPoint uint64 `json:"switchPoint,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// GetTimelineHistory returns a PostgresTimelineHistory for a PostgresTimelinesHistory
func (tlsh PostgresTimelinesHistory) GetTimelineHistory(id uint64) *PostgresTimelineHistory {
	for _, tlh := range tlsh {
		if tlh.TimelineID == id {
			return tlh
		}
	}
	return nil
}
