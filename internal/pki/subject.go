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

package pki

import (
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"

	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
)

// maxSubjectFieldLength is the maximum length of the organisation, the state and the locality (ub-organization-name,
// ub-state-name and ub-locality-name of RFC 5280)
const maxSubjectFieldLength = 64

// SubjectOptions are the fields of the subject of the root, which the intermediates and the server and client
// certificates inherit. The CommonName is set per certificate, and there is no OrganisationalUnit on purpose: the api
// uses the OrganisationalUnits of a client certificate as the groups of the client.
type SubjectOptions struct {
	// Organisation (O); oriontls.DefaultSubject.Organisation when not set
	Organisation string
	// Country (C): two letters (ISO 3166-1 alpha-2), e.g. NL
	Country string
	// State (ST): the state or province
	State string
	// Locality (L): the city
	Locality string
}

// Check returns an error when a field of the subject is invalid
func (s SubjectOptions) Check() error {
	if s.Country != "" && !isCountryCode(s.Country) {
		return fmt.Errorf("invalid country %q: use the two letter code (ISO 3166-1), e.g. NL", s.Country)
	}
	for _, field := range []struct{ name, value string }{
		{"organisation", s.Organisation},
		{"state", s.State},
		{"locality", s.Locality},
	} {
		if err := checkSubjectField(field.value); err != nil {
			return fmt.Errorf("invalid %s %q: %w", field.name, field.value, err)
		}
	}
	return nil
}

// Subject returns the subject of the root: oriontls.DefaultSubject with the fields of s that are set
func (s SubjectOptions) Subject() oriontls.Subject {
	subject := oriontls.DefaultSubject
	if s.Organisation != "" {
		subject.Organisation = s.Organisation
	}
	subject.Country = s.Country
	subject.State = s.State
	subject.Locality = s.Locality
	return subject
}

func isCountryCode(country string) bool {
	if len(country) != 2 {
		return false
	}
	for _, r := range country {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func checkSubjectField(value string) error {
	if !utf8.ValidString(value) {
		return errors.New("not valid utf-8")
	}
	if utf8.RuneCountInString(value) > maxSubjectFieldLength {
		return fmt.Errorf("longer than %d characters", maxSubjectFieldLength)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return errors.New("contains a control character")
		}
	}
	return nil
}
