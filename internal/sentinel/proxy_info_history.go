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

package sentinel

import (
	"reflect"

	cluster "github.com/pgvillage-tools/orion/api/v1"

	"github.com/mitchellh/copystructure"
)

// ProxyInfoHistory is one record in ProxyInfoHistories, whichh holds a record of ProxyInfo's
type ProxyInfoHistory struct {
	ProxyInfo *cluster.ProxyInfo
	Timer     int64
}

// ProxyInfoHistories is a map of ProxyInfoHistory items
type ProxyInfoHistories map[string]*ProxyInfoHistory

// DeepCopy returns a deep copy of ProxyInfoHistories which is a new ProxyInfoHistories with a copy of all
// ProxyInfoHistory items
func (p ProxyInfoHistories) DeepCopy() (cp ProxyInfoHistories) {
	if p == nil {
		return nil
	}
	var ok bool
	if np, err := copystructure.Copy(p); err != nil {
		panic(err)
	} else if !reflect.DeepEqual(p, np) {
		panic("not equal")
	} else if cp, ok = np.(ProxyInfoHistories); !ok {
		panic("unexpectedly copy is not same type")
	}
	return cp
}
