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
	"crypto/x509"
	"time"
)

const (
	// Day is a duration of 24 hours
	Day = 24 * time.Hour
	// Year is a duration of 365 days
	Year = 365 * Day

	// ClockSkew is subtracted from the start of the validity (NotBefore) of new certificates, so hosts with a clock
	// that is slightly behind accept them too
	ClockSkew = 5 * time.Minute

	// DefaultRootExpiry is the default validity of a root CA
	DefaultRootExpiry = 5 * Year
	// DefaultIntermediateExpiry is the default validity of an intermediate CA
	DefaultIntermediateExpiry = 3 * Year
	// DefaultExpiry is the default validity of a server or client certificate
	DefaultExpiry = Year

	// rootMaxPathLen allows one level of intermediates below the root
	rootMaxPathLen = 1
)

var (
	// DefaultCAKeyUsage is the key usage of a root or intermediate CA
	DefaultCAKeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature

	// DefaultKeyUsage is the key usage of a server or client certificate. For RSA keys, KeyEncipherment is added.
	DefaultKeyUsage = x509.KeyUsageDigitalSignature

	// DefaultServerExtKeyUsages are the extended key usages of a server certificate
	DefaultServerExtKeyUsages = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}

	// DefaultClientExtKeyUsages are the extended key usages of a client certificate
	DefaultClientExtKeyUsages = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}

	// DefaultSubject is used when no subject is set
	DefaultSubject = Subject{
		Country:            "NL",
		CommonName:         "orion",
		Locality:           "Blaricum",
		Organisation:       "PGVillage",
		OrganisationalUnit: "postgres",
		PostalCode:         "1261 WZ",
		State:              "Utrecht",
		StreetAddress:      "Binnendelta 1-U2",
	}
)
