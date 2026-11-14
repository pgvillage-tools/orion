// Package k8s_test runs the end-to-end tests of orion on Kubernetes, with the kubernetes store backend: it deploys the
// manifests of docs/deploy/k8s.md (tests/k8s/manifests) in a kind cluster (make kind-e2e-setup), and runs a cluster
// with them, front to back.
package k8s_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestK8s(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "K8s Suite")
}
