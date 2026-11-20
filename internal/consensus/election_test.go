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

package consensus

import (
	"testing"
	"time"
)

const testTimeout = 5 * time.Second

func expectElected(t *testing.T, electedCh <-chan bool, expected bool) {
	t.Helper()
	select {
	case elected, ok := <-electedCh:
		if !ok {
			t.Fatal("elected channel closed")
		}
		if elected != expected {
			t.Fatalf("expected elected %t, got %t", expected, elected)
		}
	case <-time.After(testTimeout):
		t.Fatalf("timeout waiting for elected %t", expected)
	}
}

func expectClosed(t *testing.T, electedCh <-chan bool, errCh <-chan error) {
	t.Helper()
	for _, closed := range []func() bool{
		func() bool { _, ok := <-electedCh; return !ok },
		func() bool { _, ok := <-errCh; return !ok },
	} {
		done := make(chan bool)
		go func() { done <- closed() }()
		select {
		case ok := <-done:
			if !ok {
				t.Fatal("expected the channel to be closed")
			}
		case <-time.After(testTimeout):
			t.Fatal("timeout waiting for the channel to be closed")
		}
	}
}
