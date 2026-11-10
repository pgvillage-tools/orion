package etcdv3_test

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
)

// The path a user takes, with orion-cli in a container instead of the api client of the tests: connect to the api,
// create a cluster, see it come up, and fail over. The certificates are created like `orion-cli pki init` does.
var _ = Describe("bootstrapping a cluster with orion-cli", Ordered, func() {
	// etcd and the api; the cluster is created by the cli
	tc := withOwnAPI()

	// cliStatus returns the status of the cluster, from `orion-cli status -f json`
	cliStatus := func(ctx context.Context, g Gomega) apiv1.InfoCluster {
		out, code := tc.cli(ctx, "status", "-f", "json")
		g.Ω(code).To(Equal(0), out)
		var status apiv1.InfoCluster
		g.Ω(json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &status)).To(Succeed(), out)
		return status
	}
	var primary string

	It("connects to the api with the client certificate", func(ctx context.Context) {
		out, code := tc.cli(ctx, "connect")
		Ω(code).To(Equal(0), out)
		Ω(out).To(ContainSubstring("Connected to the api as orion-cli (client-certificate)"))
	})
	It("creates a new cluster with the default settings, without a spec", func(ctx context.Context) {
		out, code := tc.cli(ctx, "init")
		Ω(code).To(Equal(0), out)

		spec, _, err := tc.apiClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		Ω(spec.InitMode).To(HaveValue(Equal(apiv1.New)))

		out, code = tc.cli(ctx, "init")
		Ω(code).To(Equal(1), out)
		Ω(out).To(ContainSubstring("cannot initialize an already existing cluster"))
	})
	It("shows the cluster once its members run", func(ctx context.Context) {
		tc.startMembers(ctx, 2)
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
		}).WithContext(ctx).WithTimeout(primaryTimeout).WithPolling(pollInterval).Should(Succeed())

		out, code := tc.cli(ctx, "status")
		Ω(code).To(Equal(0), out)
		Ω(out).To(ContainSubstring("Master Keeper: " + primary))
		Ω(out).To(ContainSubstring(primary + " (master)"))
	})
	It("fails over with failkeeper --wait", func(ctx context.Context) {
		// the standby takes over only once it converged; stay converged for two sentinel checks
		Consistently(func(g Gomega) {
			for _, k := range cliStatus(ctx, g).Keepers {
				g.Ω(k.Healthy && k.PgHealthy && k.PgCurrentGeneration == k.PgWantedGeneration).To(BeTrue())
			}
		}).WithContext(ctx).WithTimeout(10 * time.Second).WithPolling(pollInterval).Should(Succeed())

		out, code := tc.cli(ctx, "failkeeper", primary, "--wait", "--wait-timeout", "3m")
		Ω(code).To(Equal(0), out)
		Ω(out).To(ContainSubstring("the cluster applied the change"))

		out, code = tc.cli(ctx, "status")
		Ω(code).To(Equal(0), out)
		Ω(out).To(ContainSubstring("Master Keeper: "))
		Ω(out).NotTo(ContainSubstring("Master Keeper: " + primary))
	})
})
