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

package api_endpoints_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

var _ = Describe("Problem", func() {
	It("is written as application/problem+json (RFC 9457)", func() {
		w := httptest.NewRecorder()
		endpoints.WriteProblem(w, http.StatusNotFound, "keeper keeper1 doesn't exist")
		Ω(w.Code).To(Equal(http.StatusNotFound))
		Ω(w.Header().Get("Content-Type")).To(Equal(endpoints.ContentTypeProblem))
		var p map[string]any
		Ω(json.Unmarshal(w.Body.Bytes(), &p)).To(Succeed())
		Ω(p).To(Equal(map[string]any{
			"type": "about:blank", "title": "Not Found", "status": float64(404),
			"detail": "keeper keeper1 doesn't exist",
		}))
	})
	It("is an error", func() {
		Ω(endpoints.NewProblem(http.StatusConflict, "the cluster already exists").Error()).
			To(Equal("409 Conflict: the cluster already exists"))
		Ω(endpoints.NewProblem(http.StatusInternalServerError, "").Error()).To(Equal("500 Internal Server Error"))
	})
})
