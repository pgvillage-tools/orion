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

package sentinel

import (
	"context"
	"io"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/rs/zerolog"
)

// TestSentinel runs the Ginkgo specs of the package
func TestSentinel(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Sentinel Suite")
}

// quietCtx returns a context whose sentinel logger discards everything (at the sentinel log level, or
// logging.GetLogComponent replaces it)
func quietCtx() context.Context {
	_, def := logging.GetLogComponent(context.Background(), logging.SentinelComponent)
	return zerolog.New(io.Discard).Level(def.GetLevel()).WithContext(context.Background())
}
