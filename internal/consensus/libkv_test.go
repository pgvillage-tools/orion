// Copyright 2026 PgVillage
// Copyright 2017 Sorint.lab
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

package consensus

import (
	"errors"
	"fmt"
	"testing"

	libkvstore "github.com/kvtools/valkeyrie/store"
)

func TestFromLibKVStoreErr(t *testing.T) {
	other := errors.New("other")
	for in, expected := range map[error]error{
		libkvstore.ErrKeyNotFound: ErrKeyNotFound,
		libkvstore.ErrKeyModified: ErrKeyModified,
		// create only (AtomicPut without a previous pair) on an existing key
		libkvstore.ErrKeyExists:                            ErrKeyModified,
		fmt.Errorf("wrapped: %w", libkvstore.ErrKeyExists): ErrKeyModified,
		other: other,
		nil:   nil,
	} {
		if got := fromLibKVStoreErr(in); !errors.Is(got, expected) && got != expected {
			t.Errorf("%v: expected %v, got %v", in, expected, got)
		}
	}
}
