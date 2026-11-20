package v1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TimelinePosition", func() {
	history := func(switchPoints ...uint64) PostgresTimelinesHistory {
		var h PostgresTimelinesHistory
		for i, sp := range switchPoints {
			h = append(h, &PostgresTimelineHistory{TimelineID: uint64(i + 1), SwitchPoint: sp})
		}
		return h
	}
	DescribeTable("OnOtherBranch",
		func(pos, followed TimelinePosition, other bool, reason string) {
			gotOther, gotReason := pos.OnOtherBranch(followed)
			Ω(gotOther).To(Equal(other))
			Ω(gotReason).To(Equal(reason))
		},
		Entry("the followed instance is on an older timeline",
			TimelinePosition{TimelineID: 3}, TimelinePosition{TimelineID: 2},
			true, "followed instance timeline 2 < our timeline 3"),
		Entry("both on timeline 1: no history to compare",
			TimelinePosition{TimelineID: 1, XLogPos: 50}, TimelinePosition{TimelineID: 1, XLogPos: 10},
			false, ""),
		Entry("same timeline, same switch point",
			TimelinePosition{TimelineID: 2, TimelinesHistory: history(100)},
			TimelinePosition{TimelineID: 2, TimelinesHistory: history(100)},
			false, ""),
		Entry("same timeline, forked at another switch point",
			TimelinePosition{TimelineID: 2, TimelinesHistory: history(90)},
			TimelinePosition{TimelineID: 2, TimelinesHistory: history(100)},
			true, "followed instance timeline 2 forked at xlog pos 100, ours at 90"),
		Entry("same timeline, a history is missing",
			TimelinePosition{TimelineID: 2},
			TimelinePosition{TimelineID: 2, TimelinesHistory: history(100)},
			false, ""),
		Entry("the followed instance forked from our timeline after our position",
			TimelinePosition{TimelineID: 1, XLogPos: 80},
			TimelinePosition{TimelineID: 2, TimelinesHistory: history(100)},
			false, ""),
		Entry("the followed instance forked from our timeline before our position",
			TimelinePosition{TimelineID: 1, XLogPos: 120},
			TimelinePosition{TimelineID: 2, TimelinesHistory: history(100)},
			true, "followed instance (timeline 2) forked from our timeline 1 at xlog pos 100, before our current xlog pos 120"),
		Entry("the followed instance is ahead without the history of our timeline",
			TimelinePosition{TimelineID: 1, XLogPos: 120},
			TimelinePosition{TimelineID: 3},
			false, ""),
	)
})
