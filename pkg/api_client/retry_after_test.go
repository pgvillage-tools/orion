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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// retryAfter is unexported, so these specs are in package api_client; they run in the suite of api_client_test
var _ = DescribeTable("retryAfter",
	func(value string, want time.Duration) {
		h := http.Header{}
		if value != "" {
			h.Set("Retry-After", value)
		}
		Ω(retryAfter(h)).To(Equal(want))
	},
	Entry("no header", "", defaultRetryAfter),
	Entry("not a number", "abc", defaultRetryAfter),
	Entry("negative", "-1", defaultRetryAfter),
	Entry("zero", "0", time.Duration(0)),
	Entry("seconds", "2", 2*time.Second),
	Entry("capped at the maximum", "600", maxRetryAfter),
	// an HTTP date is valid, but the api sends seconds
	Entry("an HTTP date", "Wed, 21 Oct 2026 07:28:00 GMT", defaultRetryAfter),
)
