package system_test

import (
	"net/http"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/util"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

var _ = Describe("GET /v1/cluster", func() {
	It("returns the cluster data", func() {
		sharedCluster.WaitForPrimary()
		cd, httpCode, err := sharedCluster.APIClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(cd.Cluster).NotTo(BeNil())
		Ω(cd.Cluster.UID).NotTo(BeEmpty())
		Ω(cd.Cluster.Spec).NotTo(BeNil())
		Ω(cd.Cluster.Status.Master).NotTo(BeEmpty())
		Ω(cd.Keepers).To(HaveLen(helpers.DefaultKeepers))
		Ω(cd.DBs).To(HaveKey(cd.Cluster.Status.Master))
	})
})

var _ = Describe("PUT and POST /v1/cluster", Ordered, func() {
	// these tests change the cluster data, so they get their own cluster
	tc := helpers.WithOwnCluster(1)

	It("POST refuses to replace an existing cluster", func() {
		tc.WaitForPrimary()
		before, _, err := tc.APIClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())

		httpCode, err := tc.APIClient.PostCluster(&apiv1.Spec{InitMode: util.ToPtr(apiv1.New)})
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusConflict))

		after, _, err := tc.APIClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())
		Ω(after.Cluster.UID).To(Equal(before.Cluster.UID))
	})
	It("PUT replaces the cluster data", func() {
		cd, _, err := tc.APIClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())
		if cd.Cluster.Spec.PGParameters == nil {
			cd.Cluster.Spec.PGParameters = apiv1.PGParameters{}
		}
		cd.Cluster.Spec.PGParameters["application_name"] = "put-by-e2e"

		httpCode, err := tc.APIClient.PutCluster(cd)
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))

		after, _, err := tc.APIClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())
		Ω(after.Cluster.UID).To(Equal(cd.Cluster.UID))
		Ω(after.Cluster.Spec.PGParameters).To(HaveKeyWithValue("application_name", "put-by-e2e"))
	})
	It("PUT refuses cluster data without a cluster", func() {
		httpCode, err := tc.APIClient.PutCluster(nil)
		Ω(err).To(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusBadRequest))

		after, _, err := tc.APIClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())
		Ω(after.Cluster).NotTo(BeNil())
	})
	It("PUT refuses a cluster without a spec, or with an invalid spec", func() {
		cd, _, err := tc.APIClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())

		invalid := cd.DeepCopy()
		invalid.Cluster.Spec.SleepInterval = &apiv1.Duration{Duration: -time.Second}
		httpCode, err := tc.APIClient.PutCluster(invalid)
		Ω(err).To(MatchError(ContainSubstring("invalid spec")))
		Ω(httpCode).To(Equal(http.StatusBadRequest))

		noSpec := cd.DeepCopy()
		noSpec.Cluster.Spec = nil
		httpCode, err = tc.APIClient.PutCluster(noSpec)
		Ω(err).To(MatchError(ContainSubstring("no spec")))
		Ω(httpCode).To(Equal(http.StatusBadRequest))

		after, _, err := tc.APIClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())
		Ω(after.Cluster.Spec).To(Equal(cd.Cluster.Spec), "the stored spec doesn't change")
	})
})

// createConcurrently calls create n times at the same time, and returns the status codes of the calls, and their
// errors (as text, for the failure message)
func createConcurrently(n int, create func() (int, error)) ([]int, []string) {
	codes := make([]int, n)
	errs := make([]string, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer GinkgoRecover()
			defer wg.Done()
			<-start
			var err error
			codes[i], err = create()
			if err != nil {
				errs[i] = err.Error()
			}
		}()
	}
	close(start)
	wg.Wait()
	return codes, errs
}

var _ = Describe("POST /v1/cluster from several clients at the same time", Ordered, func() {
	// only etcd and the api: no cluster data yet
	tc := helpers.WithOwnAPI()

	It("creates the cluster once: one client gets 201, the others 409", func() {
		codes, errs := createConcurrently(10, func() (int, error) {
			return tc.APIClient.PostCluster(&apiv1.Spec{InitMode: util.ToPtr(apiv1.New)})
		})
		Ω(codes).To(HaveEach(BeElementOf(http.StatusCreated, http.StatusConflict)), "codes %v, errors %q", codes, errs)
		created := 0
		for _, c := range codes {
			if c == http.StatusCreated {
				created++
			}
		}
		Ω(created).To(Equal(1), "codes %v", codes)

		cd, _, err := tc.APIClient.GetCluster()
		Ω(err).NotTo(HaveOccurred())
		Ω(cd.Cluster).NotTo(BeNil())
	})
})

var _ = Describe("POST /v1/cluster without a cluster", Ordered, func() {
	// only etcd and the api: no cluster data yet
	tc := helpers.WithOwnAPI()

	It("refuses null, something that isn't a spec, and an invalid spec", func() {
		for name, body := range map[string]any{
			"null":         nil,
			"not a spec":   "not a spec",
			"invalid spec": &apiv1.Spec{SleepInterval: &apiv1.Duration{Duration: -time.Second}},
		} {
			httpCode, err := tc.APIClient.Post(endpoints.ClusterEndPoint, body)
			Ω(err).To(HaveOccurred(), name)
			Ω(httpCode).To(Equal(http.StatusBadRequest), name)
		}
	})
	It("creates the cluster: 201, with the cluster data", func() {
		httpCode, err := tc.APIClient.PostCluster(&apiv1.Spec{InitMode: util.ToPtr(apiv1.New)})
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusCreated))

		spec, _, err := tc.APIClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		Ω(spec.InitMode).To(HaveValue(Equal(apiv1.New)))
	})
})
