package etcdv3_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	// sharedCluster is started once for all tests that only read from the cluster
	sharedCluster *testCluster
	// failed is set when a test fails, so the containers are kept (and their logs are shown)
	failed bool
)

func TestEtcdv3(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Etcdv3 Suite")
}

var _ = BeforeSuite(func(ctx context.Context) {
	sharedCluster = &testCluster{}
	// until the cluster runs: when it fails to start, keep the containers and show their logs
	failed = true
	sharedCluster.start(ctx, defaultKeepers)
	failed = false
}, NodeTimeout(clusterTimeout))

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		failed = true
	}
})

var _ = AfterSuite(func(ctx context.Context) {
	sharedCluster.terminate(ctx, failed)
})
