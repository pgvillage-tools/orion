package k8s_test

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/pki"
	"github.com/pgvillage-tools/orion/internal/util"
	client "github.com/pgvillage-tools/orion/pkg/api_client"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

const (
	clusterName = "mycluster"
	// leaseName is the Lease of the sentinel election: orion-cluster-<cluster name>
	leaseName = "orion-cluster-" + clusterName
	// suPassword and replPassword go in the Secret orion-pg
	suPassword   = "su-secret"
	replPassword = "repl-secret"
	// deployTimeout is the time for the pods to start, and for a deployment to roll out
	deployTimeout = 5 * time.Minute
	// convergedFor is how long the keepers must stay converged before a failover (two sentinel checks)
	convergedFor = 15 * time.Second
	// fencingTimeout is the time for a proxy to stop serving without its store (proxyTimeout is 15s)
	fencingTimeout = time.Minute
	apiPort        = 8443
	proxyPort      = 5432
)

// The path of docs/deploy/k8s.md, front to back, on kind: deploy the manifests, create a cluster through the api, see
// it come up with the sentinel leader in a Lease, use it through the proxy Service, fail over, and lose the sentinel
// leader. Then what Kubernetes does to pods (a node drain, an eviction): delete the pod of a standby and of the
// primary, and see the StatefulSet bring back the same keepers, with their data.
var _ = Describe("orion on Kubernetes", Ordered, func() {
	var (
		apiClient client.Connection
		primary   string
		failed    bool
	)
	// status returns the status of the cluster from the api
	status := func(g Gomega) *apiv1.InfoCluster {
		st, code, err := apiClient.GetStatus()
		g.Ω(err).NotTo(HaveOccurred())
		g.Ω(code).To(Equal(http.StatusOK))
		return st
	}
	// converged checks that both keepers are healthy and run the wanted generation, and returns the primary
	converged := func(g Gomega) string {
		st := status(g)
		g.Ω(st.Keepers).To(HaveLen(2))
		for _, k := range st.Keepers {
			g.Ω(k.Healthy && k.PgHealthy).To(BeTrue(), "keeper %s healthy", k.UID)
			g.Ω(k.PgCurrentGeneration).To(Equal(k.PgWantedGeneration), "keeper %s converged", k.UID)
		}
		p, ok := st.Status["primaryKeeper"].(string)
		g.Ω(ok && p != "").To(BeTrue(), "no primary in %v", st.Status)
		return p
	}
	// keeperUIDs returns the uids of the keepers in the status of the cluster
	keeperUIDs := func(g Gomega) []string {
		var uids []string
		for _, k := range status(g).Keepers {
			uids = append(uids, k.UID)
		}
		return uids
	}
	// podOf returns the pod of a keeper, sentinel or proxy: each writes its uid in the orion-status annotation of its pod
	podOf := func(ctx context.Context, component, uid string) (string, error) {
		pods := mustKubectl(ctx, "get", "pods", "-l", "component="+component, "-o",
			`jsonpath={range .items[*]}{.metadata.name}{" "}{.metadata.annotations.orion-status}{"\n"}{end}`)
		for line := range strings.Lines(pods) {
			if name, annotation, ok := strings.Cut(strings.TrimSpace(line), " "); ok &&
				strings.Contains(annotation, fmt.Sprintf("%q", uid)) {
				return name, nil
			}
		}
		// a new pod has no annotation until its component writes it
		return "", fmt.Errorf("no pod for %s %s in\n%s", component, uid, pods)
	}
	// mustPodOf waits for the pod of a keeper, sentinel or proxy
	mustPodOf := func(ctx context.Context, component, uid string) string {
		var pod string
		Eventually(func() (err error) {
			pod, err = podOf(ctx, component, uid)
			return err
		}).WithContext(ctx).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).Should(Succeed())
		return pod
	}
	// onProxy runs fn with a connection to postgres through the proxy Service. Each call has a port-forward of its
	// own: kubectl port-forward stops when a proxy closes its connections, as it does when the primary changes.
	onProxy := func(ctx context.Context, fn func(helpers.PgConnParams) error) error {
		forwardCtx, stopForward := context.WithCancel(context.Background())
		defer stopForward()
		return fn(helpers.PgConnParams{"host": helpers.LocalHost, "user": "postgres", "password": suPassword,
			"dbname": "postgres", "port": strconv.Itoa(int(portForward(forwardCtx, "svc/orion-proxy", proxyPort)))})
	}
	// pgExec runs sql through the proxy
	pgExec := func(ctx context.Context, sql string) error {
		return onProxy(ctx, func(conn helpers.PgConnParams) error { return helpers.PgExec(ctx, conn, sql) })
	}
	// count returns the number of rows in table through the proxy
	count := func(ctx context.Context, table string) (n int, err error) {
		err = onProxy(ctx, func(conn helpers.PgConnParams) error {
			n, err = helpers.PgQueryInt(ctx, conn, "SELECT count(*) FROM "+table)
			return err
		})
		return n, err
	}
	// leaseHolder returns the holder of the Lease of the sentinel election
	leaseHolder := func(ctx context.Context) string {
		out, err := kubectl(ctx, "get", "lease", leaseName, "-o", "jsonpath={.spec.holderIdentity}")
		if err != nil {
			return ""
		}
		return out
	}

	BeforeAll(func(ctx context.Context) {
		// a namespace of its own, without what an earlier run left
		_, _ = kubectl(ctx, "delete", "namespace", namespace, "--ignore-not-found", "--wait")
		mustKubectl(ctx, "create", "namespace", namespace)

		// Certificates for the api: the names of the api Service are on the api certificate
		dir := GinkgoT().TempDir()
		opts := pki.InitOptions{CLIDir: filepath.Join(dir, "cli"), APIDir: filepath.Join(dir, "api"),
			APISANs: []string{"orion-api", "orion-api." + namespace + ".svc"}}
		_, err := pki.Init(opts)
		Ω(err).NotTo(HaveOccurred())
		mustKubectl(ctx, "create", "secret", "generic", "orion-api-pki", "--from-file="+opts.APIDir+"/")

		// Passwords for PostgreSQL
		mustKubectl(ctx, "create", "secret", "generic", "orion-pg",
			"--from-literal=su-password="+suPassword, "--from-literal=repl-password="+replPassword)

		// RBAC, sentinels, keepers, proxies and the api
		mustKubectl(ctx, "apply", "--kustomize", "manifests")
		timeout := "--timeout=" + deployTimeout.String()
		for _, workload := range []string{"deployment/orion-sentinel", "deployment/orion-proxy", "deployment/orion-api",
			"statefulset/orion-keeper"} {
			mustKubectl(ctx, "rollout", "status", workload, timeout)
		}

		// the api through its Service, with the client certificate of the cli
		clientTLS, err := pki.CLIFiles(opts.CLIDir).ClientConfig()
		Ω(err).NotTo(HaveOccurred())
		// the forward lives until the end of the specs; the context of BeforeAll ends with it
		forwardCtx, stopForward := context.WithCancel(context.Background())
		DeferCleanup(stopForward)
		apiClient = client.NewTLSConnection(helpers.LocalHost, portForward(forwardCtx, "svc/orion-api", apiPort),
			helpers.APITimeout, clientTLS)
	}, NodeTimeout(deployTimeout+time.Minute))

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			failed = true
		}
	})

	AfterAll(func(ctx context.Context) {
		if failed {
			// keep the namespace, to look into it
			showState(ctx)
			return
		}
		_, _ = kubectl(ctx, "delete", "namespace", namespace, "--wait=false")
	})

	It("connects to the api through its Service, with the client certificate", func() {
		Eventually(func(g Gomega) {
			identity, code, err := apiClient.WhoAmI()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(code).To(Equal(http.StatusOK))
			g.Ω(identity.Name).To(Equal(pki.CLIClientName))
		}).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).Should(Succeed())
	})

	It("creates a new cluster with the default settings", func() {
		// what orion-cli init sends
		code, err := apiClient.PostCluster(&apiv1.Spec{InitMode: util.ToPtr(apiv1.New)})
		Ω(err).NotTo(HaveOccurred())
		Ω(code).To(Equal(http.StatusCreated))
	})

	It("comes up with a primary and a standby, two sentinels and two proxies", func(ctx context.Context) {
		Eventually(func(g Gomega) {
			primary = converged(g)
			st := status(g)
			g.Ω(st.Sentinels).To(HaveLen(2))
			g.Ω(st.Proxies).To(HaveLen(2))
		}).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
	})

	It("keeps the sentinel leader in the Lease", func(ctx context.Context) {
		Eventually(func(g Gomega) {
			holder := leaseHolder(ctx)
			g.Ω(holder).NotTo(BeEmpty())
			g.Ω(status(g).Sentinels).To(ContainElement(apiv1.InfoSentinel{UID: holder, Leader: true}))
		}).WithContext(ctx).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).Should(Succeed())
	})

	It("serves postgres through the proxy Service", func(ctx context.Context) {
		Eventually(func() error {
			return onProxy(ctx, func(conn helpers.PgConnParams) error { return helpers.PgPing(ctx, conn) })
		}).
			WithContext(ctx).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).Should(Succeed())
		Ω(pgExec(ctx, "CREATE TABLE pods (id int)")).To(Succeed())
		Ω(pgExec(ctx, "INSERT INTO pods SELECT generate_series(1, 100)")).To(Succeed())
	})

	It("fails over to the standby when the primary is failed", func(ctx context.Context) {
		// the standby takes over only once it converged
		Consistently(func(g Gomega) { converged(g) }).
			WithContext(ctx).WithTimeout(convergedFor).WithPolling(helpers.PollInterval).Should(Succeed())

		code, err := apiClient.FailKeeper(primary)
		Ω(err).NotTo(HaveOccurred())
		Ω(code).To(Equal(http.StatusAccepted))

		Eventually(func(g Gomega) {
			g.Ω(converged(g)).NotTo(Equal(primary))
		}).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
	})

	It("elects the other sentinel when the pod of the leader is deleted", func(ctx context.Context) {
		leader := leaseHolder(ctx)
		Ω(leader).NotTo(BeEmpty())

		mustKubectl(ctx, "delete", "pod", mustPodOf(ctx, "orion-sentinel", leader), "--wait=false")

		Eventually(func(g Gomega) {
			holder := leaseHolder(ctx)
			g.Ω(holder).NotTo(BeEmpty())
			g.Ω(holder).NotTo(Equal(leader))
			sentinels := status(g).Sentinels
			g.Ω(sentinels).To(ContainElement(apiv1.InfoSentinel{UID: holder, Leader: true}))
			// the Deployment starts a new sentinel, and the deleted one goes from the status
			g.Ω(sentinels).To(HaveLen(2))
			g.Ω(sentinels).NotTo(ContainElement(HaveField("UID", leader)))
		}).WithContext(ctx).WithTimeout(2 * time.Minute).WithPolling(helpers.PollInterval).Should(Succeed())
		// the cluster still has its primary
		Eventually(func(g Gomega) { converged(g) }).
			WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
	})

	It("brings back a standby whose pod is deleted, with its uid and its data", func(ctx context.Context) {
		var uids []string
		Eventually(func(g Gomega) {
			primary = converged(g)
			uids = keeperUIDs(g)
		}).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
		var standby string
		for _, uid := range uids {
			if uid != primary {
				standby = uid
			}
		}
		pod := mustPodOf(ctx, "orion-keeper", standby)
		Ω(pgExec(ctx, "INSERT INTO pods VALUES (101)")).To(Succeed())

		mustKubectl(ctx, "delete", "pod", pod, "--wait")

		// the StatefulSet starts the pod again on its volumes: the same keeper, still a standby
		Eventually(func(g Gomega) {
			g.Ω(converged(g)).To(Equal(primary))
			g.Ω(keeperUIDs(g)).To(ConsistOf(uids))
			g.Ω(podOf(ctx, "orion-keeper", standby)).To(Equal(pod))
		}).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
		Ω(count(ctx, "pods")).To(Equal(101))
	})

	It("keeps the data when the pod of the primary is deleted", func(ctx context.Context) {
		var uids []string
		Eventually(func(g Gomega) {
			primary = converged(g)
			uids = keeperUIDs(g)
		}).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
		// the standby takes over only once it converged
		Consistently(func(g Gomega) { converged(g) }).
			WithContext(ctx).WithTimeout(convergedFor).WithPolling(helpers.PollInterval).Should(Succeed())
		Ω(pgExec(ctx, "INSERT INTO pods VALUES (102)")).To(Succeed())

		mustKubectl(ctx, "delete", "pod", mustPodOf(ctx, "orion-keeper", primary), "--wait")

		// whether the pod is back before the sentinel fails it over or not: the same two keepers, converged, one of
		// them primary, with every row, and writable through the proxy
		Eventually(func(g Gomega) {
			converged(g)
			g.Ω(keeperUIDs(g)).To(ConsistOf(uids))
		}).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
		Eventually(func() error { return pgExec(ctx, "INSERT INTO pods VALUES (103)") }).
			WithContext(ctx).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).Should(Succeed())
		Ω(count(ctx, "pods")).To(Equal(103))
	})

	It("stops serving through the proxy while orion can't use the Kubernetes api, and recovers", func(ctx context.Context) {
		Eventually(func(g Gomega) { converged(g) }).
			WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
		Ω(pgExec(ctx, "INSERT INTO pods VALUES (104)")).To(Succeed())

		// Without the RoleBinding every call of orion to the Kubernetes api (its store) is refused: the store is gone
		// for orion, while kubectl and the port-forwards keep working
		mustKubectl(ctx, "delete", "rolebinding", "orion")
		restored := false
		restore := func(ctx context.Context) {
			if !restored {
				mustKubectl(ctx, "apply", "--kustomize", "manifests")
				restored = true
			}
		}
		DeferCleanup(restore)

		// the proxies fence: without the store they close their connections, and refuse new ones
		Eventually(func() error {
			return onProxy(ctx, func(conn helpers.PgConnParams) error { return helpers.PgPing(ctx, conn) })
		}).WithContext(ctx).WithTimeout(fencingTimeout).WithPolling(helpers.PollInterval).ShouldNot(Succeed())
		Consistently(func() error {
			return onProxy(ctx, func(conn helpers.PgConnParams) error { return helpers.PgPing(ctx, conn) })
		}).WithContext(ctx).WithTimeout(10 * time.Second).WithPolling(helpers.PollInterval).ShouldNot(Succeed())
		// and the api can't read the status
		Eventually(func(g Gomega) {
			_, code, _ := apiClient.GetStatus()
			g.Ω(code).To(Equal(http.StatusInternalServerError))
		}).WithContext(ctx).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).Should(Succeed())

		restore(ctx)

		// everything comes back by itself: a converged cluster, every row, writable through the proxy
		Eventually(func(g Gomega) {
			converged(g)
			g.Ω(status(g).Sentinels).To(ContainElement(HaveField("Leader", true)))
		}).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
		Eventually(func() error { return pgExec(ctx, "INSERT INTO pods VALUES (105)") }).
			WithContext(ctx).WithTimeout(time.Minute).WithPolling(helpers.PollInterval).Should(Succeed())
		Ω(count(ctx, "pods")).To(Equal(105))
	})
})
