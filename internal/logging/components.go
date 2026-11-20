// Copyright 2026 PgVillage
// Copyright 2016 Sorint.lab
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

// Package logging brings a generic logging interface
package logging

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rs/zerolog"
)

// Components is a map that holds components and their Debug state (false is InfoLevel, True is DebugLevel)
type Components map[Component]zerolog.Level

// Component is a custom type, so that we can use it as an ENUM
type Component int

const (
	// CmdComponent is the main component specifically for orion-cli commands
	CmdComponent Component = iota
	// KeeperComponent is the main component specifically for the Keeper commands
	KeeperComponent Component = iota
	// ProxyComponent is the main component specifically for the Proxy commands
	ProxyComponent Component = iota
	// SentinelComponent is the main component specifically for the Sentinel commands
	SentinelComponent Component = iota
	// WebApiComponent is the main component specifically for the WebApi
	WebApiComponent Component = iota

	// PgComponent is the component for all PostgreSQL logging
	PgComponent Component = iota
	// PgUtilsComponent is the component for all PostgreSQL logging
	PgUtilsComponent Component = iota

	// StoreComponent is the logging component for all code dealing with KV stores
	StoreComponent Component = iota

	// UnknownComponent represents a logging component with unknown origin
	UnknownComponent Component = iota
	// TestComponent represents a logging component only used in unittests
	TestComponent Component = iota
)

var (
	componentConverter = map[string]Component{
		"keeper":              KeeperComponent,
		"kv-store":            StoreComponent,
		"proxy":               ProxyComponent,
		"sentinel":            SentinelComponent,
		"orion-cli":           CmdComponent,
		"postgres":            PgComponent,
		"postgres-utils":      PgUtilsComponent,
		"undefined_component": UnknownComponent,
		"unittest_component":  TestComponent,
		"web-api":             WebApiComponent,
	}
	reverseComponentMap map[Component]string
)

func componentToString(component Component) string {
	if reverseComponentMap == nil {
		reverseComponentMap = map[Component]string{}
		for s, comp := range componentConverter {
			reverseComponentMap[comp] = s
		}
	}
	if s, exists := reverseComponentMap[component]; exists {
		return s
	}
	return "undefined_component"
}

// ComponentNames returns the names of the components that ParseComponents accepts, sorted
func ComponentNames() []string {
	var names []string
	for name, comp := range componentConverter {
		if comp != UnknownComponent && comp != TestComponent {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// ParseComponents parses a comma separated list of components with their levels (as used in a command argument,
// e.g. "keeper=debug,kv-store=warn"); a component without a level is at debug. An empty string gives no components.
func ParseComponents(commaSeparated string) (Components, error) {
	components := Components{}
	for _, item := range strings.Split(commaSeparated, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		name, levelName, hasLevel := strings.Cut(item, "=")
		component, ok := componentConverter[name]
		if !ok || component == UnknownComponent || component == TestComponent {
			return nil, fmt.Errorf("unknown log component %q (components: %s)", name,
				strings.Join(ComponentNames(), ", "))
		}
		level := zerolog.DebugLevel
		if hasLevel {
			if level, ok = sToLevel[levelName]; !ok {
				return nil, fmt.Errorf("unknown log level %q for component %s (levels: debug, info, warn, error)",
					levelName, name)
			}
		}
		components[component] = level
	}
	return components, nil
}

// NewComponentsFromStringMap takes a string map and converts it into a Components object
// where every component is set to the level represented by the string
func NewComponentsFromStringMap(enabledComponents map[string]string) Components {
	components := Components{}
	for compName, sLevel := range enabledComponents {
		level, ok := sToLevel[sLevel]
		if !ok {
			level = zerolog.DebugLevel
		}
		if component, exists := componentConverter[compName]; exists {
			components[component] = level
		} else {
			components[UnknownComponent] = level
		}
	}
	return components
}
