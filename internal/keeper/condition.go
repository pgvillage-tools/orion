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

// condition is a state the keeper finds on every check (e.g. no db assigned), so it can log it once when it starts
// and once when it ends, instead of on every check
type condition struct {
	active bool
}

// changed records whether the condition is active now, and returns true when that differs from the previous call
func (c *condition) changed(active bool) bool {
	if c.active == active {
		return false
	}
	c.active = active
	return true
}
