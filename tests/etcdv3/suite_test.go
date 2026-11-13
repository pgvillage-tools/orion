package etcdv3_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

// sharedCluster is started once for all tests that only read from the cluster
var sharedCluster = helpers.SharedCluster(helpers.DefaultKeepers)

func TestEtcdv3(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Etcdv3 Suite")
}
