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

// KeeperInfoHistory tracks the states of a keeper
type KeeperInfoHistory struct {
	KeeperInfo *cluster.KeeperInfo
	Seen       bool
	Timer      int64
}

// KeeperInfoHistories tracks info of all keepers of this cluster
type KeeperInfoHistories map[string]*KeeperInfoHistory

// DeepCopy returns a copy of all KeeperInfoHistories, where every KeeperInfoHistory is also copied
func (k KeeperInfoHistories) DeepCopy() (nk KeeperInfoHistories) {
	if k == nil {
		return nil
	}
	var ok bool
	if kihCopy, err := copystructure.Copy(k); err != nil {
		panic(err)
	} else if !reflect.DeepEqual(k, kihCopy) {
		panic("not equal")
	} else if nk, ok = kihCopy.(KeeperInfoHistories); !ok {
		panic("unexpectdly, copy is not same type")
	}
	return nk
}
