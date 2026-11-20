package logging

import (
	"bytes"
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog"
)

const (
	defaultLevelString = "info"
	enabledLevelString = "debug"
)

var _ = Describe("log levels", func() {
	const comp = TestComponent
	ctx := context.TODO()

	// level returns the level of the logger of comp
	level := func() zerolog.Level {
		_, logger := GetLogComponent(ctx, comp)
		return logger.GetLevel()
	}

	BeforeEach(func() {
		SetStaticLevel(defaultLevelString)
		SetStaticComponents(nil)
		SetDynamicLoggingConfig(defaultLevelString, nil)
	})
	AfterEach(func() {
		SetStaticLevel(defaultLevelString)
		SetStaticComponents(nil)
		SetDynamicLoggingConfig(defaultLevelString, nil)
	})

	It("takes the static level, or the static level of the component", func() {
		Ω(level()).To(Equal(zerolog.InfoLevel))
		SetStaticLevel(enabledLevelString)
		Ω(level()).To(Equal(zerolog.DebugLevel))
		SetStaticLevel(defaultLevelString)
		SetStaticComponents(Components{comp: zerolog.DebugLevel})
		Ω(level()).To(Equal(zerolog.DebugLevel))
	})

	It("takes the dynamic level, or the dynamic level of the component", func() {
		SetDynamicLoggingConfig(enabledLevelString, nil)
		Ω(level()).To(Equal(zerolog.DebugLevel))
		SetDynamicLoggingConfig(defaultLevelString, Components{comp: zerolog.DebugLevel})
		Ω(level()).To(Equal(zerolog.DebugLevel))
		SetDynamicLoggingConfig(defaultLevelString, Components{})
		Ω(level()).To(Equal(zerolog.InfoLevel))
	})

	It("falls back to info for invalid levels", func() {
		SetStaticLevel("invalidlevel")
		SetDynamicLoggingConfig("anotherinvalidlevel", nil)
		Ω(level()).To(Equal(zerolog.InfoLevel))
	})

	DescribeTable("takes the static level when there is no dynamic level, as in the components",
		func(lvl string) {
			// SetDynamicLoggingConfig is never called in the components
			dynamicLevelSet = false
			SetStaticLevel(lvl)
			Ω(level()).To(Equal(sToLevel[lvl]))
		},
		Entry("debug", "debug"),
		Entry("info", "info"),
		Entry("warn", "warn"),
		Entry("error", "error"),
	)
})

var _ = Describe("output", func() {
	var sink *bytes.Buffer

	BeforeEach(func() {
		originalOut := output.Out
		sink = &bytes.Buffer{}
		output.Out = sink
		SetStaticComponents(nil)
		DeferCleanup(func() {
			output.Out = originalOut
			SetStaticLevel(defaultLevelString)
			SetDynamicLoggingConfig(defaultLevelString, nil)
		})
	})

	// logs logs msg at lvl with the logger of the test component, and returns whether it was written
	logs := func(lvl zerolog.Level, msg string) bool {
		sink.Reset()
		_, logger := GetLogComponent(context.TODO(), TestComponent)
		logger.WithLevel(lvl).Msg(msg)
		return bytes.Contains(sink.Bytes(), []byte(msg))
	}

	It("writes from info up at the info level", func() {
		SetStaticLevel("info")
		SetDynamicLoggingConfig("info", nil)
		Ω(logs(zerolog.DebugLevel, "a debug message")).To(BeFalse())
		Ω(logs(zerolog.InfoLevel, "an info message")).To(BeTrue())
		Ω(logs(zerolog.WarnLevel, "a warning message")).To(BeTrue())
	})

	It("writes only errors at the error level", func() {
		SetStaticLevel("error")
		SetDynamicLoggingConfig("error", nil)
		Ω(logs(zerolog.WarnLevel, "a warning message")).To(BeFalse())
		Ω(logs(zerolog.ErrorLevel, "an error message")).To(BeTrue())
	})
})

var _ = Describe("color", func() {
	It("is enabled and disabled", func() {
		originalNoColor := output.NoColor
		DeferCleanup(func() { output.NoColor = originalNoColor })
		EnableColor()
		Ω(output.NoColor).To(BeFalse())
		DisableColor()
		Ω(output.NoColor).To(BeTrue())
	})
})
