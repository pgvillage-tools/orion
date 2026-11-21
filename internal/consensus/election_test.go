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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const testTimeout = 5 * time.Second

// expectElected waits for the next election result, which must be the expected one: an unexpected result fails
// right away instead of waiting for the next one.
func expectElected(electedCh <-chan bool, expected bool) {
	GinkgoHelper()
	var elected bool
	Eventually(electedCh).WithTimeout(testTimeout).Should(Receive(&elected), "waiting for elected %t", expected)
	Ω(elected).Should(Equal(expected))
}

// receiveOK reads one value of ch in the background, and reports whether that was a value (true) or the close of
// the channel (false).
func receiveOK[T any](ch <-chan T) <-chan bool {
	done := make(chan bool, 1)
	go func() {
		_, ok := <-ch
		done <- ok
	}()
	return done
}

// expectClosed expects both channels to be closed: the next read of each must report the close, not a value.
func expectClosed(electedCh <-chan bool, errCh <-chan error) {
	GinkgoHelper()
	for name, done := range map[string]<-chan bool{"elected": receiveOK(electedCh), "error": receiveOK(errCh)} {
		var ok bool
		Eventually(done).WithTimeout(testTimeout).Should(Receive(&ok), "waiting for the %s channel to close", name)
		Ω(ok).Should(BeFalse(), "expected the %s channel to be closed", name)
	}
}
