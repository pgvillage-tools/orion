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

import "fmt"

// TimelinePosition is where a PostgreSQL instance is in its WAL: its timeline, its xlog position, and the history of
// its timelines (as in PostgresState and DBStatus)
type TimelinePosition struct {
	TimelineID       uint64
	XLogPos          uint64
	TimelinesHistory PostgresTimelinesHistory
}

// OnOtherBranch tells if an instance at p is on another timeline branch than the instance it follows (at followed),
// so it can't replicate from it without a resync (pg_rewind or a new base backup). The reason says why, for the log.
func (p TimelinePosition) OnOtherBranch(followed TimelinePosition) (other bool, reason string) {
	if followed.TimelineID < p.TimelineID {
		return true, fmt.Sprintf("followed instance timeline %d < our timeline %d", followed.TimelineID, p.TimelineID)
	}

	// if the timelines are the same check that also the switchpoints are the same.
	if followed.TimelineID == p.TimelineID {
		if p.TimelineID <= 1 {
			// if timeline <= 1 then no timeline history file exists.
			return false, ""
		}
		ftlh := followed.TimelinesHistory.GetTimelineHistory(p.TimelineID - 1)
		tlh := p.TimelinesHistory.GetTimelineHistory(p.TimelineID - 1)
		if ftlh == nil || tlh == nil {
			// No timeline history to check
			return false, ""
		}
		if ftlh.SwitchPoint == tlh.SwitchPoint {
			return false, ""
		}
		return true, fmt.Sprintf("followed instance timeline %d forked at xlog pos %d, ours at %d", p.TimelineID,
			ftlh.SwitchPoint, tlh.SwitchPoint)
	}

	// followed.TimelineID > p.TimelineID
	ftlh := followed.TimelinesHistory.GetTimelineHistory(p.TimelineID)
	if ftlh != nil && ftlh.SwitchPoint < p.XLogPos {
		return true, fmt.Sprintf("followed instance (timeline %d) forked from our timeline %d at xlog pos %d, "+
			"before our current xlog pos %d", followed.TimelineID, p.TimelineID, ftlh.SwitchPoint, p.XLogPos)
	}
	return false, ""
}
