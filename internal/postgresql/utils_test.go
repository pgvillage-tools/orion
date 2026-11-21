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

package postgresql

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("utils", func() {
	DescribeTable("parseTimelinesHistory",
		func(contents string, want []*TimelineHistory) {
			Ω(parseTimelinesHistory(contents)).To(Equal(want))
		},
		Entry("empty", "", []*TimelineHistory{}),
		Entry("one timeline", "1\t0/5000090\tno recovery target specified", []*TimelineHistory{
			{TimelineID: 1, SwitchPoint: 83886224, Reason: "no recovery target specified"},
		}),
	)

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

	DescribeTable("expandRecoveryCommand replaces %d, %w and %%",
		func(in, out string) {
			Ω(expandRecoveryCommand(in, "/datadir", "/waldir")).To(Equal(out))
		},
		Entry(nil, "", ""),
		Entry(nil, "%a", "%a"),
		Entry(nil, "%d", "/datadir"),
		Entry(nil, "%w", "/waldir"),
		Entry(nil, "%%d", "%d"),
		Entry(nil, "%%%d", "%/datadir"),
		Entry(nil, "%%%danother/string/%%%%%%%f%dddddblabla", "%/datadiranother/string/%%%%f/datadirddddblabla"),
	)

	DescribeTable("IsWalFileName",
		func(name string, valid bool) {
			Ω(IsWalFileName(name)).To(Equal(valid))
		},
		Entry(nil, "000000000000000000000000", true),
		Entry(nil, "ABCDEF0123456789ABCDEF00", true),
		Entry(nil, "", false),
		Entry(nil, "ABCDEF0123456789ABCDEF0", false),
		Entry(nil, "$123", false),
	)

	DescribeTable("WalFileNameNoTimeLine",
		func(name, out string) {
			Ω(WalFileNameNoTimeLine(name)).To(Equal(out))
		},
		Entry(nil, "000000000000000000000000", "0000000000000000"),
		Entry(nil, "ABCDEF0123456789ABCDEF00", "23456789ABCDEF00"),
	)
})
