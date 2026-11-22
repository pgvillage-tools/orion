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

package keeper

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("condition", func() {
	It("reports when it starts and when it ends, not while it lasts", func() {
		var c condition
		Expect(c.changed(false)).To(BeFalse(), "inactive from the start")
		Expect(c.changed(true)).To(BeTrue(), "starts")
		Expect(c.changed(true)).To(BeFalse(), "lasts")
		Expect(c.changed(false)).To(BeTrue(), "ends")
		Expect(c.changed(false)).To(BeFalse(), "stays inactive")
	})
})
