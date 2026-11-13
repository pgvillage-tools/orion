// Package cli_test runs the end-to-end tests of orion-cli: a user who only has the cli image sets up and runs a
// cluster with it, front to back. The shared helpers (tests/helpers) run the other containers.
package cli_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCLI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CLI Suite")
}
