// Package helpers runs orion clusters in containers for the end-to-end test suites (tests/system, tests/cli, ...):
// etcd, the api, sentinels, keepers, a proxy, and orion-cli commands, with Ginkgo and Gomega assertions.
package helpers

import (
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
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
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/modules/etcd"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
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
	Network *testcontainers.DockerNetwork
	Etcd    *etcd.EtcdContainer
	// StoreTLS makes etcd require TLS with client certificates; set it before the cluster starts
	StoreTLS bool
	store    store
	API      testcontainers.Container
	Sentinel testcontainers.Container
	Proxy    testcontainers.Container
	Keepers  []testcontainers.Container
	all      []testcontainers.Container
	// tmpDir holds the files of the cluster (e.g. the certificates); it is removed with the cluster, when the node
	// that started it (StartStore) ends
	tmpDir string

	// PKI holds the directories with the certificates of the api and of the client of the tests
	PKI pki.InitOptions
	// APIClient connects to the api over mTLS, with the certificate of the cli
	APIClient client.Connection
	// ProbeURL is the base url of the probe port of the api, e.g. http://127.0.0.1:32768
	ProbeURL string
	// ProbeClient connects to the probe port of the api (plain http)
	ProbeClient client.Connection
	// pgConn holds the connection parameters for postgres, without the port
	pgConn PgConnParams
}

// Start starts a cluster with numKeepers keepers and initializes it (new) through the api. It fills tc while it
// starts the containers, so terminate can clean up (and show the logs) after a failure halfway. Tests that change
// the cluster (e.g. fail or remove a keeper) should start their own cluster; tests that only read can use
// sharedCluster.
func (tc *Cluster) Start(ctx context.Context, numKeepers int) {
	tc.StartWithSpec(ctx, numKeepers, DefaultSpec())
}

// DefaultSpec returns the spec Start creates a test cluster with: a new cluster, with the defaults of orion
func DefaultSpec() *apiv1.Spec {
	return &apiv1.Spec{
		DefaultSUReplAccessMode: util.ToPtr(apiv1.SUReplAccessAll),
		PGParameters:            apiv1.PGParameters{},
		PGHBA:                   []string{},
		InitMode:                util.ToPtr(apiv1.New),
	}
}

// StartWithSpec starts a cluster like Start, created with spec
func (tc *Cluster) StartWithSpec(ctx context.Context, numKeepers int, spec *apiv1.Spec) {
	tc.StartAPI(ctx)

	httpCode, err := tc.APIClient.PostCluster(spec)
	Ω(err).NotTo(HaveOccurred())
	Ω(httpCode).To(Equal(http.StatusCreated))

	tc.StartMembers(ctx, numKeepers)
}

// StartMembers starts the sentinel, numKeepers keepers and the proxy of a cluster whose api runs (startAPI). It is the
// last part of start; on its own, it is for tests that create the cluster themselves.
func (tc *Cluster) StartMembers(ctx context.Context, numKeepers int) {
	var err error
	tc.Sentinel, err = runSentinel(ctx, tc.store, tc.Network)
	tc.add(tc.Sentinel)
	Ω(err).NotTo(HaveOccurred())

	for i := range numKeepers {
		alias := fmt.Sprintf("keeper_%d", i)
		settings := map[string]string{
			"pg-repl-password":  pgPassword,
			"pg-su-password":    pgPassword,
			"pg-listen-address": alias,
		}
		cnt, keeperErr := runKeeper(ctx, tc.store, tc.Network, map[string][]string{tc.Network.Name: {alias}},
			maps.Clone(settings))
		tc.add(cnt)
		Ω(keeperErr).NotTo(HaveOccurred())
		tc.Keepers = append(tc.Keepers, cnt)
	}

	tc.Proxy, err = runProxy(ctx, tc.store, tc.Network, map[string][]string{tc.Network.Name: {"proxy"}})
	tc.add(tc.Proxy)
	Ω(err).NotTo(HaveOccurred())
}

// StartAPI starts the network, etcd and the api (with new certificates), and connects the clients of the tests to the
// api. It is the first part of start; on its own, it is for tests of the api that need no keepers.
func (tc *Cluster) StartAPI(ctx context.Context) {
	tc.StartStore(ctx)

	// create the certificates for the api and the client of this test framework
	// the api certificate is also valid for apiAlias, so a client in another container (e.g. orion-cli) can connect
	tc.PKI = pki.InitOptions{CLIDir: filepath.Join(tc.tmpDir, "cli"), APIDir: filepath.Join(tc.tmpDir, "api"),
		APISANs: []string{apiAlias}}
	_, err := pki.Init(tc.PKI)
	Ω(err).NotTo(HaveOccurred())

	tc.RunAPI(ctx)
}

// StartStore starts the network and etcd: the first part of StartAPI. On its own, it is for tests that create the
// certificates of the api themselves (e.g. with PKIInit), and then start the api with RunAPI.
func (tc *Cluster) StartStore(ctx context.Context) {
	// RYUK requires permissions we don't need and don't want to implement
	Ω(os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")).To(Succeed())
	tc.tmpDir = GinkgoT().TempDir()

	tc.pgConn = PgConnParams{
		"host":     "localhost",
		"user":     pgUser,
		"password": pgPassword,
		"dbname":   pgDatabase,
	}
	var err error
	tc.Network, err = network.New(ctx)
	Ω(err).NotTo(HaveOccurred())

	var storeTLS *pki.InitOptions
	if tc.StoreTLS {
		// certificates for etcd and its clients, like the ones of the api: a server certificate for etcd, and a
		// client certificate for the orion containers
		storeTLS = &pki.InitOptions{CLIDir: filepath.Join(tc.tmpDir, "store-client"),
			APIDir: filepath.Join(tc.tmpDir, "store-server"), APISANs: []string{"etcd"}}
		_, err = pki.Init(*storeTLS)
		Ω(err).NotTo(HaveOccurred())
	}
	tc.Etcd, tc.store, err = runEtcd(ctx, etcdImage, tc.Network, storeTLS)
	if tc.Etcd != nil {
		tc.all = []testcontainers.Container{tc.Etcd}
	}
	Ω(err).NotTo(HaveOccurred())
}

// RunAPI starts the api with the certificates in tc.PKI.APIDir, and connects the clients of the tests to it, the api
// client with the cli certificate in tc.PKI.CLIDir: the last part of StartAPI
func (tc *Cluster) RunAPI(ctx context.Context) {
	clientTLS, err := pki.CLIFiles(tc.PKI.CLIDir).ClientConfig()
	Ω(err).NotTo(HaveOccurred())

	// run the api to control orion from this test framework
	tc.API, err = runAPI(ctx, tc.store, tc.Network, map[string][]string{tc.Network.Name: {apiAlias}},
		tc.PKI.APIDir)
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

// WithOwnClusterSpec is WithOwnCluster for a cluster created with spec (e.g. DefaultSpec with sync replication)
func WithOwnClusterSpec(numKeepers int, spec *apiv1.Spec) *Cluster {
	return withOwn(func(tc *Cluster, ctx context.Context) { tc.StartWithSpec(ctx, numKeepers, spec) })
}

// WithOwnClusterStoreTLS is WithOwnCluster with an etcd that requires TLS with client certificates
func WithOwnClusterStoreTLS(numKeepers int) *Cluster {
	return withOwn(func(tc *Cluster, ctx context.Context) {
		tc.StoreTLS = true
		tc.Start(ctx, numKeepers)
	})
}

// Etcdctl runs etcdctl in the etcd container of a cluster with StoreTLS, against https://localhost:2379 with the root
// CA of the store, and with the client certificate when withClientCert is set. It returns the exit code and the
// output.
func (tc *Cluster) Etcdctl(ctx context.Context, withClientCert bool, args ...string) (int, string) {
	cmd := []string{"etcdctl", "--endpoints=https://localhost:2379",
		"--cacert=" + path.Join(storeClientPKIDir, pki.CLICAFile)}
	if withClientCert {
		cmd = append(cmd, "--cert="+path.Join(storeClientPKIDir, pki.CLICertFile),
			"--key="+path.Join(storeClientPKIDir, pki.CLIKeyFile))
	}
	code, out, err := tc.Etcd.Exec(ctx, append(cmd, args...), tcexec.Multiplexed())
	Ω(err).NotTo(HaveOccurred())
	output, err := io.ReadAll(out)
	Ω(err).NotTo(HaveOccurred())
	return code, string(output)
}

// WithOwnAPI starts etcd and the api (no sentinel, keepers or proxy) for the specs of the (Ordered) container it is
// called in, and removes them after them. Use it for tests that break the api or its store, e.g. pause etcd.
func WithOwnAPI() *Cluster {
	return withOwn((*Cluster).StartAPI)
}

// WithOwnStore starts the network and etcd (no api) for the specs of the (Ordered) container it is called in, and
// removes them after them. Use it for tests that start the api themselves, e.g. with certificates from PKIInit.
func WithOwnStore() *Cluster {
	return withOwn((*Cluster).StartStore)
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

// ConvergedFor is how long WaitForConverged wants the keepers to stay converged (two sentinel checks)
const ConvergedFor = 10 * time.Second

// WaitForConverged waits until all numKeepers keepers are healthy with their db at its wanted generation, and stay
// so for ConvergedFor (so they aren't between two generations). It returns the primary.
func (tc *Cluster) WaitForConverged(numKeepers int) string {
	var primary string
	converged := func(g Gomega) {
		status, _, err := tc.APIClient.GetStatus()
		g.Ω(err).NotTo(HaveOccurred())
		g.Ω(status.Keepers).To(HaveLen(numKeepers))
		for _, k := range status.Keepers {
			g.Ω(k.Healthy && k.PgHealthy).To(BeTrue(), "keeper %s", k.UID)
			g.Ω(k.PgCurrentGeneration).To(Equal(k.PgWantedGeneration), "keeper %s converged", k.UID)
		}
		var ok bool
		primary, ok = status.Status["primaryKeeper"].(string)
		g.Ω(ok && primary != "").To(BeTrue(), "no primary keeper in %v", status.Status)
	}
	Eventually(converged).WithTimeout(PrimaryTimeout).WithPolling(PollInterval).Should(Succeed())
	Consistently(converged).WithTimeout(ConvergedFor).WithPolling(PollInterval).Should(Succeed())
	return primary
}

// keeperAddressRe matches the listen address of a keeper of a test cluster: its network alias keeper_<index>
var keeperAddressRe = regexp.MustCompile(`^keeper_(\d+):\d+$`)

// KeeperIndex returns the index in tc.Keepers of the keeper with uid, from its listen address (keeper_<index>:port).
// Call it while the keeper is healthy: once the sentinel fails a keeper over, its listen address is gone.
func (tc *Cluster) KeeperIndex(uid string) int {
	keeper, httpCode, err := tc.APIClient.GetKeeper(uid)
	Ω(err).NotTo(HaveOccurred())
	Ω(httpCode).To(Equal(http.StatusOK))
	m := keeperAddressRe.FindStringSubmatch(keeper.ListenAddress)
	Ω(m).NotTo(BeNil(), "keeper %s listens on %q", uid, keeper.ListenAddress)
	i, err := strconv.Atoi(m[1])
	Ω(err).NotTo(HaveOccurred())
	Ω(i).To(BeNumerically("<", len(tc.Keepers)))
	return i
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
	cnt, err := runSentinel(ctx, tc.store, tc.Network)
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

// cliTimeout is how long a cli command in a container may run (failkeeper --wait waits for a failover)
const cliTimeout = 5 * time.Minute

// CLI runs orion-cli with args in a container on the network of the cluster, connected to the api and logged in with
// the client certificate of the cluster, like a user would. It returns the output (stdout and stderr) and the exit
// code.
func (tc *Cluster) CLI(ctx context.Context, args ...string) (string, int) {
	var files []testcontainers.ContainerFile
	for _, f := range []string{pki.CLICertFile, pki.CLIKeyFile, pki.CLICAFile} {
		files = append(files, testcontainers.ContainerFile{
			HostFilePath:      filepath.Join(tc.PKI.CLIDir, f),
			ContainerFilePath: path.Join(cliPKIDir, f),
			FileMode:          cliPKIFileMode,
		})
	}
	return tc.runCLI(ctx, files, nil, args...)
}

// cliAPIDir is where PKIInit lets orion-cli pki init write the api certificates, in the cli container
const cliAPIDir = "/tmp/orion-api"

// PKIInit creates the certificates like a user does, with `orion-cli pki init` in a cli container, and copies them
// out of it: the cli files to tc.PKI.CLIDir (for CLI and the api client of the tests), the api files to
// tc.PKI.APIDir (for RunAPI). The api certificate is also valid for apiAlias. It returns the output of the command.
func (tc *Cluster) PKIInit(ctx context.Context) string {
	tc.PKI = pki.InitOptions{CLIDir: filepath.Join(tc.tmpDir, "cli"), APIDir: filepath.Join(tc.tmpDir, "api")}
	copyOut := func(cnt testcontainers.Container) {
		for dir, files := range map[string][]string{
			tc.PKI.CLIDir: {pki.CLICertFile, pki.CLIKeyFile, pki.CLICAFile},
			tc.PKI.APIDir: {pki.APIServerCertFile, pki.APIServerKeyFile, pki.APIClientCAFile},
		} {
			from := cliPKIDir
			if dir == tc.PKI.APIDir {
				from = cliAPIDir
			}
			Ω(os.MkdirAll(dir, cliPKIDirMode)).To(Succeed())
			for _, f := range files {
				rc, err := cnt.CopyFileFromContainer(ctx, path.Join(from, f))
				Ω(err).NotTo(HaveOccurred(), "copying %s from the cli container", path.Join(from, f))
				data, err := io.ReadAll(rc)
				_ = rc.Close()
				Ω(err).NotTo(HaveOccurred())
				Ω(os.WriteFile(filepath.Join(dir, f), data, cliPKIFileMode)).To(Succeed())
			}
		}
	}
	out, code := tc.runCLI(ctx, nil, copyOut, "pki", "init", "--api-dir", cliAPIDir, "--api-san", apiAlias)
	Ω(code).To(Equal(0), out)
	return out
}

// runCLI runs orion-cli with args in a container on the network of the cluster, with files copied into it. after (when
// set) gets the container once the command exited, before it is removed. It returns the output and the exit code.
func (tc *Cluster) runCLI(ctx context.Context, files []testcontainers.ContainerFile,
	after func(testcontainers.Container), args ...string) (string, int) {
	cmd := append([]string{"--host", apiAlias, "--port", strconv.Itoa(APIInternalPort)}, args...)
	cnt, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:      "cli",
			Cmd:        cmd,
			Files:      files,
			Networks:   []string{tc.Network.Name},
			WaitingFor: wait.ForExit().WithExitTimeout(cliTimeout),
		},
		Started: true,
	})
	if cnt != nil {
		defer func() { _ = cnt.Terminate(ctx) }()
	}
	Ω(err).NotTo(HaveOccurred(), "orion-cli %v", args)
	state, err := cnt.State(ctx)
	Ω(err).NotTo(HaveOccurred())
	logs, err := cnt.Logs(ctx)
	Ω(err).NotTo(HaveOccurred())
	defer func() { _ = logs.Close() }()
	out, err := io.ReadAll(logs)
	Ω(err).NotTo(HaveOccurred())
	fmt.Fprintf(GinkgoWriter, "$ orion-cli %s\n%s(exit %d)\n", strings.Join(args, " "), out, state.ExitCode)
	if after != nil {
		after(cnt)
	}
	return string(out), state.ExitCode
}
