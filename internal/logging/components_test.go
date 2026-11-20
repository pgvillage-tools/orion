package logging

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog"
)

var _ = Describe("components", func() {
	It("names an unknown component like UnknownComponent", func() {
		var nonexistentComponent Component = -1
		Ω(componentToString(nonexistentComponent)).To(Equal(componentToString(UnknownComponent)))
	})

	DescribeTable("ParseComponents parses name=level lists, a bare name meaning debug",
		func(in string, want Components) {
			Ω(ParseComponents(in)).To(Equal(want))
		},
		Entry("nothing", "", Components{}),
		Entry("a bare name", "keeper", Components{KeeperComponent: zerolog.DebugLevel}),
		Entry("a list with spaces", "keeper=debug, kv-store=warn,postgres,web-api=error", Components{
			KeeperComponent: zerolog.DebugLevel,
			StoreComponent:  zerolog.WarnLevel,
			PgComponent:     zerolog.DebugLevel,
			WebApiComponent: zerolog.ErrorLevel,
		}),
		Entry("a trailing comma", "sentinel=info,", Components{SentinelComponent: zerolog.InfoLevel}),
	)

	DescribeTable("ParseComponents refuses",
		func(in, msg string) {
			_, err := ParseComponents(in)
			Ω(err).To(MatchError(ContainSubstring(msg)))
		},
		Entry("an unknown component, listing the known ones", "zookeeper",
			`unknown log component "zookeeper" (components: keeper, kv-store, orion-cli, postgres, `+
				`postgres-utils, proxy, sentinel, web-api)`),
		Entry("the test component", "unittest_component", `unknown log component "unittest_component"`),
		Entry("an unknown level", "keeper=loud", `unknown log level "loud" for component keeper`),
	)

	It("makes components from a map of names to levels", func() {
		result := NewComponentsFromStringMap(map[string]string{
			"keeper": "debug", "proxy": "debug", "sentinel": "warn", "orion-cli": "warn",
		})
		Ω(result).To(Equal(Components{
			componentConverter["keeper"]:    zerolog.DebugLevel,
			componentConverter["proxy"]:     zerolog.DebugLevel,
			componentConverter["sentinel"]:  zerolog.WarnLevel,
			componentConverter["orion-cli"]: zerolog.WarnLevel,
		}))
	})
})
