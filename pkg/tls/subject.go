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

package tls

import (
	"crypto/x509/pkix"
	"encoding/asn1"
)

// oidUserID is the object identifier of the UID attribute (RFC 4519)
var oidUserID = asn1.ObjectIdentifier{0, 9, 2342, 19200300, 100, 1, 1}

// Subject can hold all fields that belong to the subject of a cert
type Subject struct {
	Country            string `json:"C"`
	CommonName         string `json:"CN"`
	Locality           string `json:"L"`
	Organisation       string `json:"O"`
	OrganisationalUnit string `json:"OU"`
	PostalCode         string `json:"PC"`
	SerialNumber       string `json:"SERIAL"`
	State              string `json:"ST"`
	StreetAddress      string `json:"STREET"`
	UserID             string `json:"UID"`
}

// SetCommonName returns a copy of the Subject, with another CommonName
func (s Subject) SetCommonName(commonName string) Subject {
	s.CommonName = commonName
	return s
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// SubjectFromPkixName converts a pkix.Name (e.g. of a loaded certificate) to a Subject. Only the first value of
// each field is used.
func SubjectFromPkixName(name pkix.Name) Subject {
	s := Subject{
		Country:            first(name.Country),
		CommonName:         name.CommonName,
		Locality:           first(name.Locality),
		Organisation:       first(name.Organization),
		OrganisationalUnit: first(name.OrganizationalUnit),
		PostalCode:         first(name.PostalCode),
		SerialNumber:       name.SerialNumber,
		State:              first(name.Province),
		StreetAddress:      first(name.StreetAddress),
	}
	for _, attr := range name.Names {
		if value, ok := attr.Value.(string); ok && attr.Type.Equal(oidUserID) {
			s.UserID = value
		}
	}
	return s
}

func nonEmpty(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}

// AsPkixName converts the Subject to a pkix.Name. Empty fields are left out.
func (s Subject) AsPkixName() pkix.Name {
	name := pkix.Name{
		Country:            nonEmpty(s.Country),
		CommonName:         s.CommonName,
		Locality:           nonEmpty(s.Locality),
		Organization:       nonEmpty(s.Organisation),
		OrganizationalUnit: nonEmpty(s.OrganisationalUnit),
		Province:           nonEmpty(s.State),
		PostalCode:         nonEmpty(s.PostalCode),
		SerialNumber:       s.SerialNumber,
		StreetAddress:      nonEmpty(s.StreetAddress),
	}
	if s.UserID != "" {
		name.ExtraNames = append(name.ExtraNames, pkix.AttributeTypeAndValue{Type: oidUserID, Value: s.UserID})
	}
	return name
}
