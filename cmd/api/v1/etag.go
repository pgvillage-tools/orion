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

package v1

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/consensus"
)

// errPreconditionFailed is the error when the If-Match header of a request doesn't match the current ETag
var errPreconditionFailed = statusError{http.StatusPreconditionFailed,
	errors.New("it changed since you read it (If-Match doesn't match its ETag): read it again")}

// etagOf returns the ETag of all cluster data: its revision in the store (pair), or "" without a pair. The sentinel
// writes the cluster data often (e.g. the health of the keepers), so this ETag changes often too.
func etagOf(pair *consensus.KVPair) string {
	if pair == nil {
		return ""
	}
	return `"` + strconv.FormatUint(pair.LastIndex, decimal) + `"`
}

// etagOfSpec returns the ETag of a cluster spec: a hash of its JSON, so it only changes when the spec changes (and
// not when the sentinel writes other parts of the cluster data). It is "" without a spec.
func etagOfSpec(spec *apiv1.Spec) string {
	if spec == nil {
		return ""
	}
	data, err := json.Marshal(spec)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:specETagBytes]) + `"`
}

// specETagBytes is the number of bytes of the hash in a spec ETag
const specETagBytes = 16

// setETag sets the ETag header (unless it is empty)
func setETag(w http.ResponseWriter, etag string) {
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
}

// ifMatches returns whether the If-Match header of r matches etag (the current ETag, "" when the resource doesn't
// exist). Without the header it always matches (If-Match is optional); "*" matches any existing resource.
func ifMatches(r *http.Request, etag string) bool {
	header := r.Header.Get("If-Match")
	if header == "" {
		return true
	}
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if (candidate == "*" && etag != "") || (candidate != "" && candidate == etag) {
			return true
		}
	}
	return false
}
