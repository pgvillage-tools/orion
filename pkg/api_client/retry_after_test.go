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

package api_client

import (
	"net/http"
	"testing"
	"time"
)

func TestRetryAfter(t *testing.T) {
	for value, want := range map[string]time.Duration{
		"":    defaultRetryAfter,
		"abc": defaultRetryAfter,
		"-1":  defaultRetryAfter,
		"0":   0,
		"2":   2 * time.Second,
		"600": maxRetryAfter,
		// an HTTP date is valid, but the api sends seconds
		"Wed, 21 Oct 2026 07:28:00 GMT": defaultRetryAfter,
	} {
		h := http.Header{}
		if value != "" {
			h.Set("Retry-After", value)
		}
		if got := retryAfter(h); got != want {
			t.Errorf("retryAfter(%q) = %v, want %v", value, got, want)
		}
	}
}
