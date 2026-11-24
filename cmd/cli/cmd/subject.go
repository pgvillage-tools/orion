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

package cmd

import (
	"strings"

	"github.com/pgvillage-tools/orion/internal/pki"
	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
	"github.com/spf13/pflag"
)

// subjectFlags are the flags for the subject of the certificates of a new PKI. There is no flag for the
// OrganisationalUnit on purpose: the api uses it as the groups of a client.
type subjectFlags struct {
	organisation string
	country      string
	state        string
	locality     string
}

// addSubjectFlags adds the subject flags to flags, and returns where their values are stored
func addSubjectFlags(flags *pflag.FlagSet) *subjectFlags {
	s := &subjectFlags{}
	flags.StringVar(&s.organisation, "org", oriontls.DefaultSubject.Organisation,
		"organisation (O) in the subject of the certificates")
	flags.StringVar(&s.country, "country", "",
		"two letter country code (C) in the subject of the certificates, e.g. NL")
	flags.StringVar(&s.state, "state", "", "state or province (ST) in the subject of the certificates")
	flags.StringVar(&s.locality, "locality", "", "locality or city (L) in the subject of the certificates")
	return s
}

// options returns the subject options of the flags; the country is upper case
func (s *subjectFlags) options() pki.SubjectOptions {
	return pki.SubjectOptions{
		Organisation: s.organisation,
		Country:      strings.ToUpper(s.country),
		State:        s.state,
		Locality:     s.locality,
	}
}
