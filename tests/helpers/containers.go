package helpers

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/pgvillage-tools/orion/internal/pki"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/etcd"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	etcdImage = "quay.io/coreos/etcd:v3.6.7"
	// APIInternalPort is the api port inside its container
	APIInternalPort = 8443
	// apiProbeInternalPort is the probe port of the api (/livez, /readyz, /status, /metrics)
	apiProbeInternalPort = 8080
	// APIStoreTimeout is the store timeout of the api in the tests
	APIStoreTimeout = 3 * time.Second
	// ClusterTimeout is the time to start a test cluster
	ClusterTimeout = 10 * time.Minute
	apiPKIDir      = "/etc/orion/api"
	// storeClientPKIDir holds the client certificate for a store with TLS, in the orion containers (and in etcd)
	storeClientPKIDir = "/etc/orion/store"
	// storeServerPKIDir holds the server certificate of etcd, for a store with TLS
	storeServerPKIDir = "/etc/etcd/pki"
	// apiAlias is the name of the api on the network of a cluster
	apiAlias = "api"
	// cliPKIDir is the default --pki-dir of orion-cli in its image (it runs as root)
	cliPKIDir = "/root/.orion/cli"
	// cliPKIFileMode is the mode of the cli files in the cli container, like orion-cli pki init creates them
	cliPKIFileMode = 0o600
	// cliPKIDirMode is the mode of the directories PKIInit copies the certificates to (like pki init creates them)
	cliPKIDirMode = 0o700
	// apiPKIFileMode makes the api files readable for the api, which runs as orion (the files are owned by root)
	apiPKIFileMode     = 0o644
	proxyInternalPort  = 25432
	keeperInternalPort = 5432
	pgUser             = "postgres"
	pgDatabase         = "postgres"
)

// store is how the orion containers connect to etcd
type store struct {
	endpoints string
	// tls holds the certificates when etcd requires TLS with client certificates (see runEtcd): the orion containers
	// use the client certificate of the cli in tls.CLIDir, and its root CA to verify etcd
	tls *pki.InitOptions
}

// env returns the store settings for an orion container whose environment variables start with prefix (e.g.
// ORIONKEEPER)
func (s store) env(prefix string) map[string]string {
	env := map[string]string{prefix + "_STORE_ENDPOINTS": s.endpoints}
	if s.tls != nil {
		env[prefix+"_STORE_CERT_FILE"] = path.Join(storeClientPKIDir, pki.CLICertFile)
		env[prefix+"_STORE_KEY"] = path.Join(storeClientPKIDir, pki.CLIKeyFile)
		env[prefix+"_STORE_CA_FILE"] = path.Join(storeClientPKIDir, pki.CLICAFile)
	}
	return env
}

// files returns the client certificate files for the store, for an orion container
func (s store) files() []testcontainers.ContainerFile {
	if s.tls == nil {
		return nil
	}
	return pkiFiles(s.tls.CLIDir, storeClientPKIDir, pki.CLICertFile, pki.CLIKeyFile, pki.CLICAFile)
}

// pkiFiles returns the files names in hostDir, for containerDir in a container
func pkiFiles(hostDir, containerDir string, names ...string) []testcontainers.ContainerFile {
	var files []testcontainers.ContainerFile
	for _, name := range names {
		files = append(files, testcontainers.ContainerFile{
			HostFilePath:      filepath.Join(hostDir, name),
			ContainerFilePath: path.Join(containerDir, name),
			FileMode:          apiPKIFileMode,
		})
	}
	return files
}

// runEtcd runs etcd. With tlsOpts (as created by pki.Init, with "etcd" in the APISANs) it only serves TLS, with the
// server certificate of the api in tlsOpts.APIDir, and only accepts client certificates signed like the one of the
// cli in tlsOpts.CLIDir; the client files are in the container too (storeClientPKIDir), for etcdctl.
func runEtcd(
	ctx context.Context,
	etcdImage string,
	nw *testcontainers.DockerNetwork,
	tlsOpts *pki.InitOptions,
) (
	cnt *etcd.EtcdContainer,
	st store,
	err error,
) {
	scheme := "http"
	var tlsArgs []string
	var files []testcontainers.ContainerFile
	if tlsOpts != nil {
		scheme = "https"
		tlsArgs = []string{
			"--cert-file", path.Join(storeServerPKIDir, pki.APIServerCertFile),
			"--key-file", path.Join(storeServerPKIDir, pki.APIServerKeyFile),
			"--client-cert-auth",
			"--trusted-ca-file", path.Join(storeServerPKIDir, pki.APIClientCAFile),
		}
		files = append(pkiFiles(tlsOpts.APIDir, storeServerPKIDir,
			pki.APIServerCertFile, pki.APIServerKeyFile, pki.APIClientCAFile),
			pkiFiles(tlsOpts.CLIDir, storeClientPKIDir, pki.CLICertFile, pki.CLIKeyFile, pki.CLICAFile)...)
	}
	// setup etcd; the arguments come after the ones of the module, and win
	etcdContainer, startErr := etcd.Run(ctx,
		etcdImage,
		network.WithNetwork([]string{"etcd"}, nw),
		testcontainers.WithFiles(files...),
		etcd.WithAdditionalArgs(append([]string{
			"--listen-client-urls", scheme + "://0.0.0.0:2379",
			"--advertise-client-urls", scheme + "://etcd:2379",
			"--initial-advertise-peer-urls", "http://etcd:2380",
			"--initial-cluster", "default=http://etcd:2380",
		}, tlsArgs...)...),
	)
	if startErr != nil {
		return nil, store{}, startErr
	}
	if tlsOpts != nil {
		// the name on the certificate
		return etcdContainer, store{endpoints: "https://etcd:2379", tls: tlsOpts}, nil
	}
	ips, ipErr := etcdContainer.ContainerIPs(ctx)
	if ipErr != nil {
		return nil, store{}, ipErr
	}
	return etcdContainer, store{endpoints: fmt.Sprintf("http://%s:2379", ips[0])}, nil
}

func runKeeper(
	ctx context.Context,
	st store,
	nw *testcontainers.DockerNetwork,
	aliasses map[string][]string,
	settings map[string]string,
) (testcontainers.Container, error) {
	pgVersion := os.Getenv("PGVERSION")
	if pgVersion == "" {
		pgVersion = "18"
	}
	envSettings := st.env("ORIONKEEPER")
	envSettings["ORIONKEEPER_LOG_LEVEL"] = "debug"
	for k, v := range settings {
		k = fmt.Sprintf("ORIONKEEPER_%s",
			strings.ReplaceAll(strings.ToUpper(k), "-", "_"))
		envSettings[k] = v
	}
	image := fmt.Sprintf("keeper-%s", pgVersion)
	fmt.Fprintf(GinkgoWriter, "DEBUG - Keeper image: %s", image)
	return testcontainers.GenericContainer(
		ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Env:            envSettings,
				Image:          image,
				Files:          st.files(),
				Networks:       []string{nw.Name},
				NetworkAliases: aliasses,
				WaitingFor: wait.ForLog(
					"postgres hba entries not changed"),
			},
			Started: true,
		})
}

func runSentinel(
	ctx context.Context,
	st store,
	nw *testcontainers.DockerNetwork,
) (testcontainers.Container, error) {
	envSettings := st.env("ORIONSENTINEL")
	envSettings["ORIONSENTINEL_LOG_LEVEL"] = "debug"
	return testcontainers.GenericContainer(
		ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Env:        envSettings,
				Files:      st.files(),
				Networks:   []string{nw.Name},
				ExtraHosts: []string{},
				Image:      "sentinel",
			},
			Started: true,
		})
}

func runProxy(
	ctx context.Context,
	st store,
	nw *testcontainers.DockerNetwork,
	aliasses map[string][]string,
) (testcontainers.Container, error) {
	envSettings := st.env("ORIONPROXY")
	envSettings["ORIONPROXY_LOG_LEVEL"] = "debug"
	return testcontainers.GenericContainer(
		ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Env:            envSettings,
				Image:          "proxy",
				Files:          st.files(),
				Networks:       []string{nw.Name},
				NetworkAliases: aliasses,
				WaitingFor: wait.ForLog(
					"proxying to master address"),
				// WaitingFor: wait.ForListeningPort(
				//     nat.Port(fmt.Sprintf("%d/tcp", proxyPort))),
			},
			Started: true,
		})
}

// runAPI runs the api, with the api files (as created by pki.Init) from pkiDir
func runAPI(
	ctx context.Context,
	st store,
	nw *testcontainers.DockerNetwork,
	aliasses map[string][]string,
	pkiDir string,
) (testcontainers.Container, error) {
	envSettings := st.env("ORIONAPI")
	envSettings["ORIONAPI_LOG_LEVEL"] = "debug"
	envSettings["ORIONAPI_PKI_DIR"] = apiPKIDir
	// shorter than APITimeout (the client), so a store that doesn't respond gives a 500 and not a client timeout;
	// long enough for the first call to etcd after the api starts
	envSettings["ORIONAPI_STORE_TIMEOUT"] = APIStoreTimeout.String()
	files := append(pkiFiles(pkiDir, apiPKIDir, pki.APIServerCertFile, pki.APIServerKeyFile, pki.APIClientCAFile),
		st.files()...)
	return testcontainers.GenericContainer(
		ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Env:   envSettings,
				Image: "api",
				Files: files,
				ExposedPorts: []string{
					fmt.Sprintf("%d/tcp", APIInternalPort),
					fmt.Sprintf("%d/tcp", apiProbeInternalPort),
				},
				Networks:       []string{nw.Name},
				NetworkAliases: aliasses,
				WaitingFor: wait.ForLog(
					"server ready"),
				// WaitingFor: wait.ForListeningPort(
				//     nat.Port(fmt.Sprintf("%d/tcp", proxyPort))),
			},
			Started: true,
		})
}
