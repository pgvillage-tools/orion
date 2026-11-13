package etcdv3_test

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

var _ = Describe("DELETE /v1/cluster/keepers/{uid}", Ordered, func() {
	// these tests remove keepers, so they get their own cluster: a primary, a standby that is stopped and removed, and
	// a standby that is removed while it runs
	const numKeepers = 3
	tc := helpers.WithOwnCluster(numKeepers)
	var primary string
	var standbys []apiv1.InfoKeeper

	// hasDB returns whether the cluster data has a db for keeper
	hasDB := func(cd *apiv1.Data, keeper string) bool {
		for _, db := range cd.DBs {
			if db.Spec != nil && db.Spec.KeeperUID == keeper {
				return true
			}
		}
		return false
	}

	BeforeAll(func() {
		primary = tc.WaitForPrimary()
		// wait until every keeper reported its listen address (it is "(unknown)" until its db reports it)
		Eventually(func(g Gomega) {
			status, _, err := tc.APIClient.GetStatus()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(status.Keepers).To(HaveLen(numKeepers))
			g.Ω(status.Keepers).To(HaveEach(HaveField("ListenAddress", MatchRegexp(`^keeper_\d+:\d+$`))))
			standbys = nil
			for _, k := range status.Keepers {
				if k.UID != primary {
					standbys = append(standbys, k)
				}
			}
		}).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
	})

	It("refuses to remove the keeper of the primary", func() {
		httpCode, err := tc.APIClient.DeleteKeeper(primary)
		Ω(err).To(MatchError(ContainSubstring("fail it first")))
		Ω(httpCode).To(Equal(http.StatusConflict))

		cd, _, err := tc.APIClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())
		Ω(cd.Keepers).To(HaveKey(primary))
	})
	It("removes a stopped keeper, and its db, from the cluster data", func(ctx context.Context) {
		stopped := standbys[0]
		// its listen address is keeper_<index>:<port>
		alias, _, found := strings.Cut(stopped.ListenAddress, ":")
		Ω(found).To(BeTrue(), "listen address %q", stopped.ListenAddress)
		index, err := strconv.Atoi(strings.TrimPrefix(alias, "keeper_"))
		Ω(err).NotTo(HaveOccurred())
		Ω(tc.Keepers[index].Stop(ctx, nil)).To(Succeed())

		httpCode, err := tc.APIClient.DeleteKeeper(stopped.UID)
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusAccepted))

		cd, _, err := tc.APIClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())
		Ω(cd.Keepers).NotTo(HaveKey(stopped.UID))
		Ω(hasDB(cd, stopped.UID)).To(BeFalse(), "the db of the removed keeper")
		Ω(cd.Keepers).To(HaveKey(primary))
	})
	It("doesn't keep out a keeper that still runs: it registers itself again", func() {
		running := standbys[1]
		httpCode, err := tc.APIClient.DeleteKeeper(running.UID)
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusAccepted))

		// the sentinel adds it back from the keeper's own registration
		Eventually(func(g Gomega) {
			cd, _, err := tc.APIClient.GetCluster()
			g.Ω(err).NotTo(HaveOccurred())
			g.Ω(cd.Keepers).To(HaveKey(running.UID))
		}).WithTimeout(helpers.PrimaryTimeout).WithPolling(helpers.PollInterval).Should(Succeed())
	})
	It("returns 404 for an unknown keeper", func() {
		httpCode, err := tc.APIClient.DeleteKeeper("unknown")
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusNotFound))
	})
	It("returns 500 while the store doesn't respond", func(ctx context.Context) {
		helpers.PauseContainer(ctx, tc.Etcd)
		httpCode, err := tc.APIClient.DeleteKeeper(standbys[1].UID)
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusInternalServerError))
	})
})

var _ = Describe("DELETE /v1/cluster/keepers/{uid} without a cluster", Ordered, func() {
	// only etcd and the api: no cluster data yet
	tc := helpers.WithOwnAPI()

	It("returns 404", func() {
		httpCode, err := tc.APIClient.DeleteKeeper("keeper0")
		Ω(err).To(MatchError(ContainSubstring("no cluster yet")))
		Ω(httpCode).To(Equal(http.StatusNotFound))
	})
})
