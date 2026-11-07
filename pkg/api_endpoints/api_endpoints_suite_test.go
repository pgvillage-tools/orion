package api_endpoints_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEndpoints(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Endpoints Suite")
}
