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

package api_endpoints

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	// ContentTypeProblem is the content type of a Problem (RFC 9457)
	ContentTypeProblem = "application/problem+json"
	// ContentTypeMergePatch is the content type of a JSON merge patch (RFC 7386), e.g. for PATCH /v1/cluster/spec
	ContentTypeMergePatch = "application/merge-patch+json"
)

// Problem is the body of every error response of the api: a problem details object (RFC 9457). The api uses the
// default type (about:blank), so title is the text of the status code and detail explains this occurrence.
type Problem struct {
	Type   string `json:"type,omitempty"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// NewProblem returns the problem for status, with detail
func NewProblem(status int, detail string) Problem {
	return Problem{Type: "about:blank", Title: http.StatusText(status), Status: status, Detail: detail}
}

// Error makes a Problem an error, e.g. "404 Not Found: keeper keeper1 doesn't exist"
func (p Problem) Error() string {
	if p.Detail == "" {
		return fmt.Sprintf("%d %s", p.Status, p.Title)
	}
	return fmt.Sprintf("%d %s: %s", p.Status, p.Title, p.Detail)
}

// WriteProblem writes the problem for status, with detail, as the response
func WriteProblem(w http.ResponseWriter, status int, detail string) {
	body, err := json.Marshal(NewProblem(status, detail))
	if err != nil {
		// a Problem only has strings and an int, so this doesn't happen
		http.Error(w, detail, status)
		return
	}
	w.Header().Set("Content-Type", ContentTypeProblem)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
}
