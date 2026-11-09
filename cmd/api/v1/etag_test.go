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

package v1

import (
	"net/http/httptest"
	"testing"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/consensus"
)

func TestIfMatches(t *testing.T) {
	pair := &consensus.KVPair{LastIndex: 42}
	for name, tc := range map[string]struct {
		header   string
		pair     *consensus.KVPair
		expected bool
	}{
		"no If-Match":                  {"", pair, true},
		"the current ETag":             {`"42"`, pair, true},
		"an old ETag":                  {`"41"`, pair, false},
		"one of several":               {`"40", "42"`, pair, true},
		"a weak ETag":                  {`W/"42"`, pair, true},
		"* with cluster data":          {"*", pair, true},
		"* without cluster data":       {"*", nil, false},
		"an ETag without cluster data": {`"42"`, nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("PUT", "/v1/cluster/spec", nil)
			if tc.header != "" {
				r.Header.Set("If-Match", tc.header)
			}
			if got := ifMatches(r, etagOf(tc.pair)); got != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, got)
			}
		})
	}
	if got := etagOf(pair); got != `"42"` {
		t.Errorf(`expected "42", got %s`, got)
	}
}

func TestETagOfSpec(t *testing.T) {
	a := &apiv1.Spec{PGParameters: apiv1.PGParameters{"work_mem": "8MB"}}
	b := &apiv1.Spec{PGParameters: apiv1.PGParameters{"work_mem": "8MB"}}
	c := &apiv1.Spec{PGParameters: apiv1.PGParameters{"work_mem": "16MB"}}
	if etagOfSpec(a) != etagOfSpec(b) {
		t.Error("the same spec must have the same ETag")
	}
	if etagOfSpec(a) == etagOfSpec(c) {
		t.Error("another spec must have another ETag")
	}
	if etagOfSpec(nil) != "" {
		t.Error("no spec has no ETag")
	}
}
