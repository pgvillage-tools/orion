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

package api

import (
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/consensus"
)

var _ = Describe("ETags", func() {
	pair := &consensus.KVPair{LastIndex: 42}

	DescribeTable("ifMatches",
		func(header string, pair *consensus.KVPair, expected bool) {
			r := httptest.NewRequest("PUT", "/v1/cluster/spec", nil)
			if header != "" {
				r.Header.Set("If-Match", header)
			}
			Expect(ifMatches(r, etagOf(pair))).To(Equal(expected))
		},
		Entry("no If-Match", "", pair, true),
		Entry("the current ETag", `"42"`, pair, true),
		Entry("an old ETag", `"41"`, pair, false),
		Entry("one of several", `"40", "42"`, pair, true),
		Entry("a weak ETag", `W/"42"`, pair, true),
		Entry("* with cluster data", "*", pair, true),
		Entry("* without cluster data", "*", nil, false),
		Entry("an ETag without cluster data", `"42"`, nil, false),
	)

	It("is the revision of the cluster data, quoted", func() {
		Expect(etagOf(pair)).To(Equal(`"42"`))
	})

	It("of a spec depends on the spec", func() {
		a := &apiv1.Spec{PGParameters: apiv1.PGParameters{"work_mem": "8MB"}}
		b := &apiv1.Spec{PGParameters: apiv1.PGParameters{"work_mem": "8MB"}}
		c := &apiv1.Spec{PGParameters: apiv1.PGParameters{"work_mem": "16MB"}}
		Expect(etagOfSpec(a)).To(Equal(etagOfSpec(b)), "the same spec must have the same ETag")
		Expect(etagOfSpec(a)).NotTo(Equal(etagOfSpec(c)), "another spec must have another ETag")
		Expect(etagOfSpec(nil)).To(BeEmpty(), "no spec has no ETag")
	})
})
