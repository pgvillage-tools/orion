package cli_test

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

// The path a user takes, front to back, with orion-cli in a container: create the certificates, connect to the api,
// create a cluster, see it come up, use it, change its spec, and fail over. The tests only start the containers
// (etcd, the api, the members) and use the api client of the tests to check what the cli did.
var _ = Describe("bootstrapping a cluster with orion-cli", Ordered, func() {
	// etcd only: the certificates of the api come from orion-cli pki init
	tc := helpers.WithOwnStore()

	// cliStatus returns the status of the cluster, from `orion-cli status -f json`
	cliStatus := func(ctx context.Context, g Gomega) apiv1.InfoCluster {
		out, code := tc.CLI(ctx, "status", "-f", "json")
		g.Ω(code).To(Equal(0), out)
		var status apiv1.InfoCluster
		g.Ω(json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &status)).To(Succeed(), out)
		return status
	}
	var primary string

	It("creates the certificates with pki init, for the cli and the api", func(ctx context.Context) {
		out := tc.PKIInit(ctx)
		Ω(out).To(ContainSubstring("Created the CA and the cli client certificate in "))
		Ω(out).To(ContainSubstring("Created the api certificates in "))
		// the api starts with the certificates the cli created
		tc.RunAPI(ctx)
	})
	It("connects to the api with the client certificate", func(ctx context.Context) {
		out, code := tc.CLI(ctx, "connect")
		Ω(code).To(Equal(0), out)
		Ω(out).To(ContainSubstring("Connected to the api as orion-cli (client-certificate)"))
	})
	It("creates a new cluster with the default settings, without a spec", func(ctx context.Context) {
		out, code := tc.CLI(ctx, "init")
		Ω(code).To(Equal(0), out)

		spec, _, err := tc.APIClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		Ω(spec.InitMode).To(HaveValue(Equal(apiv1.New)))

		out, code = tc.CLI(ctx, "init")
		Ω(code).To(Equal(1), out)
		Ω(out).To(ContainSubstring("cannot initialize an already existing cluster"))
	})
	It("shows the cluster once its members run", func(ctx context.Context) {
		tc.StartMembers(ctx, 2)
		Eventually(func(g Gomega) {
			status := cliStatus(ctx, g)
			g.Ω(status.Keepers).To(HaveLen(2))
			for _, k := range status.Keepers {
				g.Ω(k.Healthy && k.PgHealthy).To(BeTrue(), "keeper %s", k.UID)
				g.Ω(k.PgCurrentGeneration).To(Equal(k.PgWantedGeneration), "keeper %s converged", k.UID)
			}
			var ok bool
			primary, ok = status.Status["primaryKeeper"].(string)
			g.Ω(ok && primary != "").To(BeTrue(), "no primary in %v", status.Status)
		}).WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())

		out, code := tc.CLI(ctx, "status")
		Ω(code).To(Equal(0), out)
		Ω(out).To(ContainSubstring("Master Keeper: " + primary))
		Ω(out).To(ContainSubstring(primary + " (master)"))
	})
	It("serves postgres through the proxy", func(ctx context.Context) {
		Eventually(func() error { return helpers.PgPing(ctx, tc.ProxyConn(ctx)) }).
			WithContext(ctx).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
	})
	It("changes the spec with update --patch, and shows it with spec", func(ctx context.Context) {
		out, code := tc.CLI(ctx, "update", "--patch", "--wait", `{"sleepInterval":"6s"}`)
		Ω(code).To(Equal(0), out)
		Ω(out).To(ContainSubstring("the cluster applied the change"))

		out, code = tc.CLI(ctx, "spec")
		Ω(code).To(Equal(0), out)
		Ω(out).To(MatchRegexp(`"sleepInterval":\s*"6s"`))
	})
	It("fails over with failkeeper --wait", func(ctx context.Context) {
		// the standby takes over only once it converged; stay converged for two sentinel checks
		Consistently(func(g Gomega) {
			for _, k := range cliStatus(ctx, g).Keepers {
				g.Ω(k.Healthy && k.PgHealthy && k.PgCurrentGeneration == k.PgWantedGeneration).To(BeTrue())
			}
		}).WithContext(ctx).WithTimeout(15 * time.Second).WithPolling(helpers.PollInterval).Should(Succeed())

		out, code := tc.CLI(ctx, "failkeeper", primary, "--wait", "--wait-timeout", "3m")
		Ω(code).To(Equal(0), out)
		Ω(out).To(ContainSubstring("the cluster applied the change"))

		out, code = tc.CLI(ctx, "status")
		Ω(code).To(Equal(0), out)
		Ω(out).To(ContainSubstring("Master Keeper: "))
		Ω(out).NotTo(ContainSubstring("Master Keeper: " + primary))
	})
})
