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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("replication slot names", func() {
	DescribeTable("IsValidReplSlotName",
		func(name string, valid bool) {
			Ω(IsValidReplSlotName(name)).To(Equal(valid))
		},
		Entry(nil, "aaaaaaaa", true),
		Entry(nil, "a12345aa", true),
		Entry(nil, "_a1_2345aa_", true),
		Entry(nil, "", false),
		Entry(nil, "a-aaaaaaa", false),
		Entry(nil, "_a1_-2345aa_", false),
		Entry(nil, "ABC123", false),
		Entry(nil, "$123", false),
	)
})
