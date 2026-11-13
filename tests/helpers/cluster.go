// Package helpers runs orion clusters in containers for the end-to-end test suites in tests: etcd, the api, sentinels,
// keepers and a proxy, with Ginkgo and Gomega assertions.
package helpers

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
	// LocalHost is the address of the mapped ports of the containers
	LocalHost  = "127.0.0.1"
	pgPassword = "test123"
	// DefaultKeepers is the number of keepers of a test cluster
	DefaultKeepers = 3
	// APITimeout is the timeout of the api client of the tests
	APITimeout = 5 * time.Second
	// PrimaryTimeout is the time a cluster gets to elect a primary
	PrimaryTimeout = 3 * time.Minute
	// PollInterval is the interval for polling the cluster with Eventually
	PollInterval = 2 * time.Second
)

// failed is set when a test fails, so the containers are kept (and their logs are shown)
var failed bool

// Cluster is an orion cluster in containers: etcd, the api, a sentinel, keepers and a proxy
type Cluster struct {
	Network       *testcontainers.DockerNetwork
	Etcd          *etcd.EtcdContainer
	EtcdEndpoints string
	API           testcontainers.Container
	Sentinel      testcontainers.Container
	Proxy         testcontainers.Container
	Keepers       []testcontainers.Container
	all           []testcontainers.Container

	// pki holds the directories with the certificates of the api and of the client of the tests
	PKI pki.InitOptions
	// apiClient connects to the api over mTLS, with the certificate of the cli
	APIClient client.Connection
	// probeURL is the base url of the probe port of the api, e.g. http://127.0.0.1:32768
	ProbeURL string
	// probeClient connects to the probe port of the api (plain http)
	ProbeClient client.Connection
	// pgConn holds the connection parameters for postgres, without the port
	pgConn PgConnParams
}

// Start starts a cluster with numKeepers keepers and initializes it (new) through the api. It fills tc while it
// starts the containers, so terminate can clean up (and show the logs) after a failure halfway. Tests that change
// the cluster (e.g. fail or remove a keeper) should start their own cluster; tests that only read can use
// sharedCluster.
func (tc *Cluster) Start(ctx context.Context, numKeepers int) {
	tc.StartAPI(ctx)

	httpCode, err := tc.APIClient.PostCluster(&apiv1.Spec{
		DefaultSUReplAccessMode: util.ToPtr(apiv1.SUReplAccessAll),
		PGParameters:            apiv1.PGParameters{},
		PGHBA:                   []string{},
		InitMode:                util.ToPtr(apiv1.New),
	})
	Ω(err).NotTo(HaveOccurred())
	Ω(httpCode).To(Equal(http.StatusCreated))

	tc.Sentinel, err = runSentinel(ctx, tc.EtcdEndpoints, tc.Network)
	tc.add(tc.Sentinel)
	Ω(err).NotTo(HaveOccurred())

	for i := range numKeepers {
		alias := fmt.Sprintf("keeper_%d", i)
		settings := map[string]string{
			"pg-repl-password":  pgPassword,
			"pg-su-password":    pgPassword,
			"pg-listen-address": alias,
		}
		cnt, keeperErr := runKeeper(ctx, tc.EtcdEndpoints, tc.Network, map[string][]string{tc.Network.Name: {alias}},
			maps.Clone(settings))
		tc.add(cnt)
		Ω(keeperErr).NotTo(HaveOccurred())
		tc.Keepers = append(tc.Keepers, cnt)
	}

	tc.Proxy, err = runProxy(ctx, tc.EtcdEndpoints, tc.Network, map[string][]string{tc.Network.Name: {"proxy"}})
	tc.add(tc.Proxy)
	Ω(err).NotTo(HaveOccurred())
}

// StartAPI starts the network, etcd and the api (with new certificates), and connects the clients of the tests to the
// api. It is the first part of start; on its own, it is for tests of the api that need no keepers.
func (tc *Cluster) StartAPI(ctx context.Context) {
	// RYUK requires permissions we don't need and don't want to implement
	Ω(os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")).To(Succeed())

	tc.pgConn = PgConnParams{
		"host":     "localhost",
		"user":     pgUser,
		"password": pgPassword,
		"dbname":   pgDatabase,
	}
	var err error
	tc.Network, err = network.New(ctx)
	Ω(err).NotTo(HaveOccurred())

	tc.Etcd, tc.EtcdEndpoints, err = runEtcd(ctx, etcdImage, tc.Network)
	if tc.Etcd != nil {
		tc.all = []testcontainers.Container{tc.Etcd}
	}
	Ω(err).NotTo(HaveOccurred())

	// create the certificates for the api and the client of this test framework
	tmpDir := GinkgoT().TempDir()
	tc.PKI = pki.InitOptions{CLIDir: filepath.Join(tmpDir, "cli"), APIDir: filepath.Join(tmpDir, "api")}
	_, err = pki.Init(tc.PKI)
	Ω(err).NotTo(HaveOccurred())
	clientTLS, err := pki.CLIFiles(tc.PKI.CLIDir).ClientConfig()
	Ω(err).NotTo(HaveOccurred())

	// run the api to control orion from this test framework
	tc.API, err = runAPI(ctx, tc.EtcdEndpoints, tc.Network, map[string][]string{}, tc.PKI.APIDir)
	tc.add(tc.API)
	Ω(err).NotTo(HaveOccurred())
	apiPort, err := tc.API.MappedPort(ctx, fmt.Sprintf("%d/tcp", APIInternalPort))
	Ω(err).NotTo(HaveOccurred())
	tc.APIClient = client.NewTLSConnection(LocalHost, apiPort.Num(), APITimeout, clientTLS)
	probePort, err := tc.API.MappedPort(ctx, fmt.Sprintf("%d/tcp", apiProbeInternalPort))
	Ω(err).NotTo(HaveOccurred())
	tc.ProbeURL = fmt.Sprintf("http://%s:%s", LocalHost, probePort.Port())
	tc.ProbeClient = client.NewConnection(endpoints.HTTP, LocalHost, probePort.Num(), APITimeout)
}

// SharedCluster registers the BeforeSuite that starts a cluster with numKeepers keepers once, for all specs that only
// read from it, and the AfterSuite that removes it (and shows the logs of its containers when a spec failed). Call it
// once, at the top level of a suite.
func SharedCluster(numKeepers int) *Cluster {
	tc := &Cluster{}
	BeforeSuite(func(ctx context.Context) {
		// until the cluster runs: when it fails to start, keep the containers and show their logs
		failed = true
		tc.Start(ctx, numKeepers)
		failed = false
	}, NodeTimeout(ClusterTimeout))
	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			failed = true
		}
	})
	AfterSuite(func(ctx context.Context) {
		tc.Terminate(ctx, failed)
	})
	return tc
}

// WithOwnCluster starts a cluster for the specs of the (Ordered) container it is called in, and removes it after them.
// Use it for tests that change the cluster, e.g. fail or remove a keeper.
func WithOwnCluster(numKeepers int) *Cluster {
	return withOwn(func(tc *Cluster, ctx context.Context) { tc.Start(ctx, numKeepers) })
}

// WithOwnAPI starts etcd and the api (no sentinel, keepers or proxy) for the specs of the (Ordered) container it is
// called in, and removes them after them. Use it for tests that break the api or its store, e.g. pause etcd.
func WithOwnAPI() *Cluster {
	return withOwn((*Cluster).StartAPI)
}

// withOwn registers the BeforeAll that starts a test cluster with start, and the AfterAll that removes it (and shows
// the logs of its containers when one of the specs failed)
func withOwn(start func(tc *Cluster, ctx context.Context)) *Cluster {
	tc := &Cluster{}
	// whether starting the cluster or one of the specs of this container failed
	ownFailed := false
	BeforeAll(func(ctx context.Context) {
		ownFailed = true
		start(tc, ctx)
		ownFailed = false
	}, NodeTimeout(ClusterTimeout))
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
		tc.Terminate(ctx, ownFailed)
	})
	return tc
}

// WaitForPrimary waits until the status of the cluster (from the api) has a primary keeper, and returns its uid
func (tc *Cluster) WaitForPrimary() string {
	var primary string
	Eventually(func(g Gomega) {
		status, httpCode, err := tc.APIClient.GetStatus()
		g.Ω(err).NotTo(HaveOccurred())
		g.Ω(httpCode).To(Equal(http.StatusOK))
		var ok bool
		primary, ok = status.Status["primaryKeeper"].(string)
		g.Ω(ok).To(BeTrue(), "no primary keeper in %v", status.Status)
		g.Ω(primary).NotTo(BeEmpty())
	}).WithTimeout(PrimaryTimeout).WithPolling(PollInterval).Should(Succeed())
	return primary
}

// ProbeGet gets ep from the probe port and returns the status code and the body
func (tc *Cluster) ProbeGet(ctx context.Context, ep endpoints.ProbeEndPoint) (int, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tc.ProbeURL+"/"+string(ep), nil)
	Ω(err).NotTo(HaveOccurred())
	resp, err := http.DefaultClient.Do(req)
	Ω(err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	Ω(err).NotTo(HaveOccurred())
	return resp.StatusCode, string(body)
}

// AddSentinel starts another sentinel in the cluster, and returns its container
func (tc *Cluster) AddSentinel(ctx context.Context) testcontainers.Container {
	cnt, err := runSentinel(ctx, tc.EtcdEndpoints, tc.Network)
	tc.add(cnt)
	Ω(err).NotTo(HaveOccurred())
	return cnt
}

// add adds a container to the containers of the cluster (also when it failed to start, so it gets cleaned up)
func (tc *Cluster) add(cnt testcontainers.Container) {
	if cnt != nil {
		tc.all = append(tc.all, cnt)
	}
}

// KeeperConn returns the connection parameters for the postgres of keeper i
func (tc *Cluster) KeeperConn(ctx context.Context, i int) PgConnParams {
	port, err := tc.Keepers[i].MappedPort(ctx, fmt.Sprintf("%d/tcp", keeperInternalPort))
	Ω(err).NotTo(HaveOccurred())
	return tc.pgConn.SetParam("port", port.Port())
}

// ProxyConn returns the connection parameters for postgres through the proxy
func (tc *Cluster) ProxyConn(ctx context.Context) PgConnParams {
	port, err := tc.Proxy.MappedPort(ctx, fmt.Sprintf("%d/tcp", proxyInternalPort))
	Ω(err).NotTo(HaveOccurred())
	return tc.pgConn.SetParam("port", port.Port())
}

// Terminate removes the containers and the network of the cluster. When a test failed, it writes the logs of all
// containers to the GinkgoWriter, and keeps them. ORION_TEST_KEEP=true keeps them always.
func (tc *Cluster) Terminate(ctx context.Context, failed bool) {
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
		// A spec may leave a container paused: its DeferCleanup that unpauses it runs after the AfterAll that calls
		// terminate. Docker stops a paused container, podman refuses to ("container state improper").
		_ = unpause(ctx, cnt) // fails for a container that isn't paused
		Ω(cnt.Terminate(ctx)).To(Succeed())
	}
	if tc.Network != nil {
		Ω(tc.Network.Remove(ctx)).To(Succeed())
	}
}

// logTailLines is the number of lines of every container's log that dumpLogs shows
const logTailLines = 300

// dumpLogs writes the last logTailLines lines of the logs of all containers of the cluster to stdout, in a collapsible
// group in GitHub Actions. It writes to stdout and not to the GinkgoWriter: it runs in AfterAll, which succeeds, and
// Ginkgo only shows the GinkgoWriter output of nodes that fail (unless it runs verbose).
func (tc *Cluster) dumpLogs(ctx context.Context) {
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
