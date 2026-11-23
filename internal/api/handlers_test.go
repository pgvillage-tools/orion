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

package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("writeAccepted", func() {
	It("returns 202 with the status as Location", func() {
		w := httptest.NewRecorder()
		writeAccepted(w)
		Expect(w.Code).To(Equal(http.StatusAccepted))
		Expect(w.Header().Get("Location")).To(Equal("/v1/cluster/status"))
	})
})

// failingWriter is a ResponseWriter whose Write fails, like one of a client that is gone
type failingWriter struct {
	*httptest.ResponseRecorder
	writeHeaders int
}

func (w *failingWriter) WriteHeader(status int) {
	w.writeHeaders++
	w.ResponseRecorder.WriteHeader(status)
}

func (*failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

var _ = Describe("writeJSONStatus", func() {
	It("responds with the status and the data as JSON", func() {
		w := httptest.NewRecorder()
		(&Handlers{}).writeJSONStatus(context.Background(), w, http.StatusCreated, map[string]int{"a": 1})
		Expect(w.Code).To(Equal(http.StatusCreated))
		Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))
		Expect(w.Body.String()).To(MatchJSON(`{"a": 1}`))
	})

	It("responds with a 500 problem when the data can't be encoded", func() {
		w := httptest.NewRecorder()
		(&Handlers{}).writeJSONStatus(context.Background(), w, http.StatusOK, func() {})
		Expect(w.Code).To(Equal(http.StatusInternalServerError))
	})

	It("doesn't respond a second time when writing the body fails", func() {
		w := &failingWriter{ResponseRecorder: httptest.NewRecorder()}
		(&Handlers{}).writeJSONStatus(context.Background(), w, http.StatusOK, map[string]int{"a": 1})
		Expect(w.writeHeaders).To(Equal(1), "the header is written once")
		Expect(w.Code).To(Equal(http.StatusOK))
		Expect(w.Header().Get("Content-Type")).To(Equal("application/json"), "no problem response after the header")
	})
})
