package logging

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponentToString(t *testing.T) {
	var nonexistentComponent Component = -1
	assert.Equal(t, componentToString(nonexistentComponent), componentToString(UnknownComponent))
}

func TestParseComponents(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Components
		err  string
	}{
		{in: "", want: Components{}},
		{in: "keeper", want: Components{KeeperComponent: zerolog.DebugLevel}},
		{
			in: "keeper=debug, kv-store=warn,postgres,web-api=error",
			want: Components{
				KeeperComponent: zerolog.DebugLevel,
				StoreComponent:  zerolog.WarnLevel,
				PgComponent:     zerolog.DebugLevel,
				WebApiComponent: zerolog.ErrorLevel,
			},
		},
		{in: "sentinel=info,", want: Components{SentinelComponent: zerolog.InfoLevel}},
		{in: "zookeeper", err: `unknown log component "zookeeper" (components: keeper, kv-store, orion-cli, postgres, ` +
			`postgres-utils, proxy, sentinel, web-api)`},
		{in: "unittest_component", err: `unknown log component "unittest_component"`},
		{in: "keeper=loud", err: `unknown log level "loud" for component keeper`},
	} {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseComponents(tc.in)
			if tc.err != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestNewComponentsFromStringMap(t *testing.T) {
	var (
		debugComponents = []string{
			"keeper",
			"proxy",
		}
		warnComponents = []string{
			"sentinel",
			"orion-cli",
		}
		strMap = map[string]string{}
	)
	for _, compName := range debugComponents {
		strMap[compName] = "debug"
	}
	for _, compName := range warnComponents {
		strMap[compName] = "warn"
	}
	result := NewComponentsFromStringMap(strMap)

	for _, compName := range debugComponents {
		comp, exists := componentConverter[compName]
		require.True(t, exists)
		level, exists := result[comp]
		require.True(t, exists)
		require.Equal(t, level, zerolog.DebugLevel)
	}
	for _, compName := range warnComponents {
		comp, exists := componentConverter[compName]
		require.True(t, exists)
		level, exists := result[comp]
		require.True(t, exists)
		require.Equal(t, level, zerolog.WarnLevel)
	}
}
