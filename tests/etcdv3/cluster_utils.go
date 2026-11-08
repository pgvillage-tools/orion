package etcdv3_test

import (
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/pki"
	"github.com/pgvillage-tools/orion/internal/util"
	client "github.com/pgvillage-tools/orion/pkg/api_client"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/etcd"
	"github.com/testcontainers/testcontainers-go/network"
)

const (
	localHost  = "127.0.0.1"
	pgPassword = "test123"
	// defaultKeepers is the number of keepers of a test cluster
	defaultKeepers = 3
	// apiTimeout is the timeout of the api client of the tests
	apiTimeout = 5 * time.Second
	// primaryTimeout is the time a cluster gets to elect a primary
	primaryTimeout = 3 * time.Minute
	// pollInterval is the interval for polling the cluster with Eventually
	pollInterval = 2 * time.Second
)

// failed is set when a test fails, so the containers are kept (and their logs are shown)
var failed bool

// testCluster is an orion cluster in containers: etcd, the api, a sentinel, keepers and a proxy
type testCluster struct {
	nw            *testcontainers.DockerNetwork
	etcd          *etcd.EtcdContainer
	etcdEndpoints string
	api           testcontainers.Container
	sentinel      testcontainers.Container
	proxy         testcontainers.Container
	keepers       []testcontainers.Container
	all           []testcontainers.Container

	// pki holds the directories with the certificates of the api and of the client of the tests
	pki pki.InitOptions
	// apiClient connects to the api over mTLS, with the certificate of the cli
	apiClient client.Connection
	// probeURL is the base url of the probe port of the api, e.g. http://127.0.0.1:32768
	probeURL string
	// probeClient connects to the probe port of the api (plain http)
	probeClient client.Connection
	// pgConn holds the connection parameters for postgres, without the port
	pgConn pgConnParams
}

// start starts a cluster with numKeepers keepers and initializes it (new) through the api. It fills tc while it
// starts the containers, so terminate can clean up (and show the logs) after a failure halfway. Tests that change
// the cluster (e.g. fail or remove a keeper) should start their own cluster; tests that only read can use
// sharedCluster.
func (tc *testCluster) start(ctx context.Context, numKeepers int) {
	tc.startAPI(ctx)

	httpCode, err := tc.apiClient.PostCluster(&apiv1.Spec{
		DefaultSUReplAccessMode: util.ToPtr(apiv1.SUReplAccessAll),
		PGParameters:            apiv1.PGParameters{},
		PGHBA:                   []string{},
		InitMode:                util.ToPtr(apiv1.New),
	})
	Ω(err).NotTo(HaveOccurred())
	Ω(httpCode).To(Equal(http.StatusCreated))

	tc.sentinel, err = runSentinel(ctx, tc.etcdEndpoints, tc.nw)
	tc.add(tc.sentinel)
	Ω(err).NotTo(HaveOccurred())

	for i := range numKeepers {
		alias := fmt.Sprintf("keeper_%d", i)
		settings := map[string]string{
			"pg-repl-password":  pgPassword,
			"pg-su-password":    pgPassword,
			"pg-listen-address": alias,
		}
		cnt, keeperErr := runKeeper(ctx, tc.etcdEndpoints, tc.nw, map[string][]string{tc.nw.Name: {alias}},
			maps.Clone(settings))
		tc.add(cnt)
		Ω(keeperErr).NotTo(HaveOccurred())
		tc.keepers = append(tc.keepers, cnt)
	}

	tc.proxy, err = runProxy(ctx, tc.etcdEndpoints, tc.nw, map[string][]string{tc.nw.Name: {"proxy"}})
	tc.add(tc.proxy)
	Ω(err).NotTo(HaveOccurred())
}

// startAPI starts the network, etcd and the api (with new certificates), and connects the clients of the tests to the
// api. It is the first part of start; on its own, it is for tests of the api that need no keepers.
func (tc *testCluster) startAPI(ctx context.Context) {
	// RYUK requires permissions we don't need and don't want to implement
	Ω(os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")).To(Succeed())

	tc.pgConn = pgConnParams{
		"host":     "localhost",
		"user":     pgUser,
		"password": pgPassword,
		"dbname":   pgDatabase,
	}
	var err error
	tc.nw, err = network.New(ctx)
	Ω(err).NotTo(HaveOccurred())

	tc.etcd, tc.etcdEndpoints, err = runEtcd(ctx, etcdImage, tc.nw)
	if tc.etcd != nil {
		tc.all = []testcontainers.Container{tc.etcd}
	}
	Ω(err).NotTo(HaveOccurred())

	// create the certificates for the api and the client of this test framework
	tmpDir := GinkgoT().TempDir()
	tc.pki = pki.InitOptions{CLIDir: filepath.Join(tmpDir, "cli"), APIDir: filepath.Join(tmpDir, "api")}
	_, err = pki.Init(tc.pki)
	Ω(err).NotTo(HaveOccurred())
	clientTLS, err := pki.CLIFiles(tc.pki.CLIDir).ClientConfig()
	Ω(err).NotTo(HaveOccurred())

	// run the api to control orion from this test framework
	tc.api, err = runAPI(ctx, tc.etcdEndpoints, tc.nw, map[string][]string{}, tc.pki.APIDir)
	tc.add(tc.api)
	Ω(err).NotTo(HaveOccurred())
	apiPort, err := tc.api.MappedPort(ctx, fmt.Sprintf("%d/tcp", apiInternalPort))
	Ω(err).NotTo(HaveOccurred())
	tc.apiClient = client.NewTLSConnection(localHost, apiPort.Num(), apiTimeout, clientTLS)
	probePort, err := tc.api.MappedPort(ctx, fmt.Sprintf("%d/tcp", apiProbeInternalPort))
	Ω(err).NotTo(HaveOccurred())
	tc.probeURL = fmt.Sprintf("http://%s:%s", localHost, probePort.Port())
	tc.probeClient = client.NewConnection(endpoints.HTTP, localHost, probePort.Num(), apiTimeout)
}

// withOwnCluster starts a cluster for the specs of the (Ordered) container it is called in, and removes it after them.
// Use it for tests that change the cluster, e.g. fail or remove a keeper.
func withOwnCluster(numKeepers int) *testCluster {
	return withOwn(func(tc *testCluster, ctx context.Context) { tc.start(ctx, numKeepers) })
}

// withOwnAPI starts etcd and the api (no sentinel, keepers or proxy) for the specs of the (Ordered) container it is
// called in, and removes them after them. Use it for tests that break the api or its store, e.g. pause etcd.
func withOwnAPI() *testCluster {
	return withOwn((*testCluster).startAPI)
}

// withOwn registers the BeforeAll that starts a test cluster with start, and the AfterAll that removes it (and shows
// the logs of its containers when one of the specs failed)
func withOwn(start func(tc *testCluster, ctx context.Context)) *testCluster {
	tc := &testCluster{}
	// whether starting the cluster or one of the specs of this container failed
	ownFailed := false
	BeforeAll(func(ctx context.Context) {
		ownFailed = true
		start(tc, ctx)
		ownFailed = false
	}, NodeTimeout(clusterTimeout))
	// For the last spec of an Ordered container, Ginkgo runs AfterAll before the AfterEach of the suite (which sets
	// failed), but after the AfterEach of the container itself: this one.
	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			ownFailed = true
		}
	})
	AfterAll(func(ctx context.Context) {
		if ownFailed {
			failed = true
		}
		tc.terminate(ctx, ownFailed)
	})
	return tc
}

// waitForPrimary waits until the status of the cluster (from the api) has a primary keeper, and returns its uid
func (tc *testCluster) waitForPrimary() string {
	var primary string
	Eventually(func(g Gomega) {
		status, httpCode, err := tc.apiClient.GetStatus()
		g.Ω(err).NotTo(HaveOccurred())
		g.Ω(httpCode).To(Equal(http.StatusOK))
		var ok bool
		primary, ok = status.Status["primaryKeeper"].(string)
		g.Ω(ok).To(BeTrue(), "no primary keeper in %v", status.Status)
		g.Ω(primary).NotTo(BeEmpty())
	}).WithTimeout(primaryTimeout).WithPolling(pollInterval).Should(Succeed())
	return primary
}

// probeGet gets ep from the probe port and returns the status code and the body
func (tc *testCluster) probeGet(ctx context.Context, ep endpoints.ProbeEndPoint) (int, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tc.probeURL+"/"+string(ep), nil)
	Ω(err).NotTo(HaveOccurred())
	resp, err := http.DefaultClient.Do(req)
	Ω(err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	Ω(err).NotTo(HaveOccurred())
	return resp.StatusCode, string(body)
}

// addSentinel starts another sentinel in the cluster, and returns its container
func (tc *testCluster) addSentinel(ctx context.Context) testcontainers.Container {
	cnt, err := runSentinel(ctx, tc.etcdEndpoints, tc.nw)
	tc.add(cnt)
	Ω(err).NotTo(HaveOccurred())
	return cnt
}

// add adds a container to the containers of the cluster (also when it failed to start, so it gets cleaned up)
func (tc *testCluster) add(cnt testcontainers.Container) {
	if cnt != nil {
		tc.all = append(tc.all, cnt)
	}
}

// keeperConn returns the connection parameters for the postgres of keeper i
func (tc *testCluster) keeperConn(ctx context.Context, i int) pgConnParams {
	port, err := tc.keepers[i].MappedPort(ctx, fmt.Sprintf("%d/tcp", keeperInternalPort))
	Ω(err).NotTo(HaveOccurred())
	return tc.pgConn.setParam("port", port.Port())
}

// proxyConn returns the connection parameters for postgres through the proxy
func (tc *testCluster) proxyConn(ctx context.Context) pgConnParams {
	port, err := tc.proxy.MappedPort(ctx, fmt.Sprintf("%d/tcp", proxyInternalPort))
	Ω(err).NotTo(HaveOccurred())
	return tc.pgConn.setParam("port", port.Port())
}

// terminate removes the containers and the network of the cluster. When a test failed, it writes the logs of all
// containers to the GinkgoWriter, and keeps them. ORION_TEST_KEEP=true keeps them always.
func (tc *testCluster) terminate(ctx context.Context, failed bool) {
	if tc == nil {
		return
	}
	if failed {
		tc.dumpLogs(ctx)
	}
	if failed || os.Getenv("ORION_TEST_KEEP") == "true" {
		GinkgoWriter.Printf("keeping the containers of the cluster\n")
		return
	}
	for _, cnt := range tc.all {
		Ω(cnt.Terminate(ctx)).To(Succeed())
	}
	if tc.nw != nil {
		Ω(tc.nw.Remove(ctx)).To(Succeed())
	}
}

// logTailLines is the number of lines of every container's log that dumpLogs shows
const logTailLines = 300

// dumpLogs writes the last logTailLines lines of the logs of all containers of the cluster to stdout, in a collapsible
// group in GitHub Actions. It writes to stdout and not to the GinkgoWriter: it runs in AfterAll, which succeeds, and
// Ginkgo only shows the GinkgoWriter output of nodes that fail (unless it runs verbose).
func (tc *testCluster) dumpLogs(ctx context.Context) {
	group, endGroup := "===== logs of %s (last %d lines) =====\n", ""
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		group, endGroup = "::group::logs of %s (last %d lines)\n", "::endgroup::\n"
	}
	for _, cnt := range tc.all {
		name, _ := cnt.Name(ctx)
		logs, err := cnt.Logs(ctx)
		if err != nil {
			fmt.Printf("no logs for %s: %v\n", name, err)
			continue
		}
		data, _ := io.ReadAll(logs)
		_ = logs.Close()
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		if len(lines) > logTailLines {
			lines = lines[len(lines)-logTailLines:]
		}
		fmt.Printf(group, name, logTailLines)
		fmt.Println(strings.Join(lines, "\n"))
		fmt.Print(endGroup)
	}
}
