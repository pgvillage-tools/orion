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
	"reflect"

	"github.com/mitchellh/copystructure"
	"github.com/pgvillage-tools/orion/internal/common"
)

// DBLocalState can be used to store the local state for DB settings
type DBLocalState struct {
	UID        string
	Generation int64
	// Initializing registers when the db is initializing. Needed to detect
	// when the initialization has failed.
	Initializing bool
	// InitPGParameters contains the postgres parameter after the
	// initialization
	InitPGParameters common.Parameters
}

// DeepCopy is a function that copies the state of a local database
func (s *DBLocalState) DeepCopy() (dc *DBLocalState) {
	var ok bool
	if s == nil {
		return nil
	}
	if ns, err := copystructure.Copy(s); err != nil {
		panic(err)
	} else if !reflect.DeepEqual(s, ns) {
		panic("not equal")
	} else if dc, ok = ns.(*DBLocalState); !ok {
		panic("different type after copy")
	}
	return dc
}
