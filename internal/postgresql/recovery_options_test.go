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

package postgresql

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RecoveryOptions", func() {
	It("starts with no recovery mode and empty, but usable, parameters", func() {
		ro := NewRecoveryOptions()
		Ω(ro.RecoveryMode).To(Equal(RecoveryModeNone))
		Ω(ro.RecoveryParameters).NotTo(BeNil())
		Ω(ro.RecoveryParameters).To(BeEmpty())
		ro.RecoveryParameters["primary_conninfo"] = "host=db1"
		Ω(ro.RecoveryParameters).To(HaveLen(1))
	})
	It("returns a deep copy that is equal but independent", func() {
		ro := NewRecoveryOptions()
		ro.RecoveryMode = RecoveryModeStandby
		ro.RecoveryParameters["primary_slot_name"] = "slot1"

		cp := ro.DeepCopy()
		Ω(cp).To(Equal(ro))
		Ω(cp).NotTo(BeIdenticalTo(ro))

		cp.RecoveryMode = RecoveryModeRecovery
		cp.RecoveryParameters["primary_slot_name"] = "slot2"
		Ω(ro.RecoveryMode).To(Equal(RecoveryModeStandby))
		Ω(ro.RecoveryParameters).To(HaveKeyWithValue("primary_slot_name", "slot1"))
	})
})
