package tls

import (
	"crypto/x509/pkix"
)

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

// SetCommonName will return a new Subject, but with another CommonName
func (s Subject) SetCommonName(commonName string) Subject {
	return Subject{
		Country:            s.Country,
		CommonName:         commonName,
		Locality:           s.Locality,
		Organisation:       s.Organisation,
		OrganisationalUnit: s.OrganisationalUnit,
		PostalCode:         s.PostalCode,
		SerialNumber:       s.SerialNumber,
		State:              s.State,
		StreetAddress:      s.StreetAddress,
		UserID:             s.UserID,
	}
}

// AsPkixName will convert the Subject to a Pkix.Name
func (s Subject) AsPkixName() pkix.Name {
	return pkix.Name{
		Country:            []string{s.Country},
		CommonName:         s.CommonName,
		Locality:           []string{s.Locality},
		Organization:       []string{s.Organisation},
		OrganizationalUnit: []string{s.OrganisationalUnit},
		Province:           []string{s.State},
		PostalCode:         []string{s.PostalCode},
		SerialNumber:       s.SerialNumber,
		StreetAddress:      []string{s.StreetAddress},
	}
}
