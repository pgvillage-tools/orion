// Copyright 2026 PgVillage
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied
// See the License for the specific language governing permissions and
// limitations under the License.

package consensus

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"

	jsonpatch "github.com/evanphx/json-patch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/util"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

const (
	testNamespace = "ns"
	testCluster   = "test"
	testPod       = "pod1"
	// testConfigMap is the name of the configmap of testCluster
	testConfigMap = "orion-cluster-test"
	// leasesPath is the path of the leases of testNamespace
	leasesPath = "/apis/coordination.k8s.io/v1/namespaces/" + testNamespace + "/leases"
)

// fakeKubeAPI is a minimal Kubernetes API server for the configmaps, leases and pods of one namespace,
// enough for a real *kubernetes.Clientset (the KubeStore doesn't accept the client-go fake
// clientset). Requests matching a key ("<METHOD> <resource>", e.g. "GET configmaps") in failures
// fail with that status code; conflicts makes that many updates fail with a conflict first.
type fakeKubeAPI struct {
	mu         sync.Mutex
	configMaps map[string]*corev1.ConfigMap
	leases     map[string]*coordinationv1.Lease
	pods       map[string]*corev1.Pod
	failures   map[string]int
	conflicts  int
	version    int
}

// newFakeKubeAPI starts a fakeKubeAPI for the current spec, and returns a clientset for it
func newFakeKubeAPI() (*fakeKubeAPI, *kubernetes.Clientset) {
	GinkgoHelper()
	api := &fakeKubeAPI{
		configMaps: map[string]*corev1.ConfigMap{},
		leases:     map[string]*coordinationv1.Lease{},
		pods:       map[string]*corev1.Pod{},
		failures:   map[string]int{},
	}
	srv := httptest.NewServer(api)
	DeferCleanup(srv.Close)
	client, err := kubernetes.NewForConfig(&rest.Config{
		Host: srv.URL,
		ContentConfig: rest.ContentConfig{
			ContentType:        "application/json",
			AcceptContentTypes: "application/json",
		},
	})
	Ω(err).ShouldNot(HaveOccurred())
	return api, client
}

func (a *fakeKubeAPI) fail(key string, code int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failures[key] = code
}

func (a *fakeKubeAPI) addConfigMap(annotations map[string]string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.version++
	a.configMaps[testConfigMap] = &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:            testConfigMap,
		Namespace:       testNamespace,
		Annotations:     annotations,
		ResourceVersion: strconv.Itoa(a.version),
	}}
}

func (a *fakeKubeAPI) configMapAnnotation() (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cm, ok := a.configMaps[testConfigMap]
	if !ok {
		return "", false
	}
	v, ok := cm.Annotations[util.KubeClusterDataAnnotation]
	return v, ok
}

func (a *fakeKubeAPI) addPod(name string, component ComponentLabelValue, status any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      name,
		Namespace: testNamespace,
		Labels: map[string]string{
			DefaultComponentLabel: string(component),
			util.KubeClusterLabel: testCluster,
		},
	}}
	switch status := status.(type) {
	case nil:
	case string:
		pod.Annotations = map[string]string{util.KubeStatusAnnnotation: status}
	default:
		j, _ := json.Marshal(status)
		pod.Annotations = map[string]string{util.KubeStatusAnnnotation: string(j)}
	}
	a.pods[name] = pod
}

func (a *fakeKubeAPI) podAnnotation(name string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pods[name].Annotations[util.KubeStatusAnnnotation]
}

func writeJSON(w http.ResponseWriter, code int, obj any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(obj)
}

func writeStatus(w http.ResponseWriter, code int, reason metav1.StatusReason) {
	writeJSON(w, code, &metav1.Status{
		TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"},
		Status:   metav1.StatusFailure,
		Reason:   reason,
		Code:     int32(code),
	})
}

func (a *fakeKubeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if r.URL.Path == "/version" {
		if code, ok := a.failures["GET version"]; ok {
			writeStatus(w, code, metav1.StatusReasonInternalError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"gitVersion": "v1.37.0"})
		return
	}
	if name, ok := strings.CutPrefix(r.URL.Path, leasesPath); ok {
		if code, ok := a.failures[r.Method+" leases"]; ok {
			writeStatus(w, code, metav1.StatusReasonInternalError)
			return
		}
		body, _ := io.ReadAll(r.Body)
		a.serveLease(w, r.Method, strings.TrimPrefix(name, "/"), body)
		return
	}

	// /api/v1/namespaces/<ns>/<resource>[/<name>]
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/namespaces/"+testNamespace+"/"), "/")
	resource, name := parts[0], ""
	if len(parts) > 1 {
		name = parts[1]
	}
	if code, ok := a.failures[r.Method+" "+resource]; ok {
		writeStatus(w, code, metav1.StatusReasonInternalError)
		return
	}
	body, _ := io.ReadAll(r.Body)

	switch resource {
	case "configmaps":
		a.serveConfigMap(w, r.Method, name, body)
	case "pods":
		a.servePod(w, r, name, body)
	default:
		writeStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound)
	}
}

func (a *fakeKubeAPI) serveConfigMap(w http.ResponseWriter, method, name string, body []byte) {
	cm, exists := a.configMaps[name]
	switch method {
	case http.MethodGet:
		if !exists {
			writeStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound)
			return
		}
		writeJSON(w, http.StatusOK, cm)
	case http.MethodPost, http.MethodPut:
		var newCM corev1.ConfigMap
		if err := json.Unmarshal(body, &newCM); err != nil {
			writeStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest)
			return
		}
		if method == http.MethodPost && a.configMaps[newCM.Name] != nil {
			writeStatus(w, http.StatusConflict, metav1.StatusReasonAlreadyExists)
			return
		}
		if method == http.MethodPut {
			if !exists {
				writeStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound)
				return
			}
			if a.conflicts > 0 || newCM.ResourceVersion != cm.ResourceVersion {
				a.conflicts--
				writeStatus(w, http.StatusConflict, metav1.StatusReasonConflict)
				return
			}
		}
		a.version++
		newCM.ResourceVersion = strconv.Itoa(a.version)
		newCM.Kind, newCM.APIVersion = "ConfigMap", "v1"
		a.configMaps[newCM.Name] = &newCM
		writeJSON(w, http.StatusOK, &newCM)
	default:
		writeStatus(w, http.StatusMethodNotAllowed, metav1.StatusReasonMethodNotAllowed)
	}
}

// serveLease serves the leases like serveConfigMap the configmaps: get, create, and update with a resource version
// check
func (a *fakeKubeAPI) serveLease(w http.ResponseWriter, method, name string, body []byte) {
	lease, exists := a.leases[name]
	switch method {
	case http.MethodGet:
		if !exists {
			writeStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound)
			return
		}
		writeJSON(w, http.StatusOK, lease)
	case http.MethodPost, http.MethodPut:
		var newLease coordinationv1.Lease
		if err := json.Unmarshal(body, &newLease); err != nil {
			writeStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest)
			return
		}
		if method == http.MethodPost && a.leases[newLease.Name] != nil {
			writeStatus(w, http.StatusConflict, metav1.StatusReasonAlreadyExists)
			return
		}
		if method == http.MethodPut && (!exists || newLease.ResourceVersion != lease.ResourceVersion) {
			writeStatus(w, http.StatusConflict, metav1.StatusReasonConflict)
			return
		}
		a.version++
		newLease.ResourceVersion = strconv.Itoa(a.version)
		newLease.Kind, newLease.APIVersion = "Lease", "coordination.k8s.io/v1"
		a.leases[newLease.Name] = &newLease
		writeJSON(w, http.StatusOK, &newLease)
	default:
		writeStatus(w, http.StatusMethodNotAllowed, metav1.StatusReasonMethodNotAllowed)
	}
}

// leaseHolder returns the holder of the lease of testCluster
func (a *fakeKubeAPI) leaseHolder() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	lease, ok := a.leases[testConfigMap]
	if !ok || lease.Spec.HolderIdentity == nil {
		return ""
	}
	return *lease.Spec.HolderIdentity
}

func (a *fakeKubeAPI) servePod(w http.ResponseWriter, r *http.Request, name string, body []byte) {
	if name == "" {
		selector, err := labels.Parse(r.URL.Query().Get("labelSelector"))
		if err != nil {
			writeStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest)
			return
		}
		list := &corev1.PodList{TypeMeta: metav1.TypeMeta{Kind: "PodList", APIVersion: "v1"}}
		for _, pod := range a.pods {
			if selector.Matches(labels.Set(pod.Labels)) {
				list.Items = append(list.Items, *pod)
			}
		}
		writeJSON(w, http.StatusOK, list)
		return
	}
	pod, exists := a.pods[name]
	if !exists {
		writeStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, pod)
	case http.MethodPatch:
		podJSON, _ := json.Marshal(pod)
		patched, err := jsonpatch.MergePatch(podJSON, body)
		if err != nil {
			writeStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest)
			return
		}
		var newPod corev1.Pod
		if err = json.Unmarshal(patched, &newPod); err != nil {
			writeStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest)
			return
		}
		a.pods[name] = &newPod
		writeJSON(w, http.StatusOK, &newPod)
	default:
		writeStatus(w, http.StatusMethodNotAllowed, metav1.StatusReasonMethodNotAllowed)
	}
}

var _ = Describe("KubeStore", func() {
	newTestKubeStore := func() (*fakeKubeAPI, *KubeStore) {
		GinkgoHelper()
		fakeAPI, client := newFakeKubeAPI()
		store, err := NewKubeStore(client, testPod, testNamespace, testCluster)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(store.resourceName).Should(Equal(testConfigMap))
		return fakeAPI, store
	}

	var (
		ctx context.Context
		api *fakeKubeAPI
		s   *KubeStore
	)

	BeforeEach(func() {
		ctx = context.Background()
		api, s = newTestKubeStore()
	})

	It("is healthy when the api server is", func() {
		Ω(s.Healthy(ctx)).Should(Succeed())
		api.fail("GET version", http.StatusInternalServerError)
		Ω(s.Healthy(ctx)).ShouldNot(Succeed())
	})

	DescribeTable("GetClusterData",
		func(setup func(*fakeKubeAPI), wantCD, wantErr bool) {
			setup(api)
			cd, pair, err := s.GetClusterData(ctx)
			if wantErr {
				Ω(err).Should(HaveOccurred())
			} else {
				Ω(err).ShouldNot(HaveOccurred())
			}
			if !wantCD {
				Ω(cd).Should(BeNil())
				Ω(pair).Should(BeNil())
				return
			}
			Ω(cd).ShouldNot(BeNil())
			Ω(pair).ShouldNot(BeNil())
			Ω(cd.FormatVersion).Should(Equal(uint64(1)))
			Ω(string(pair.Value)).Should(Equal(`{"formatVersion":1}`))
		},
		Entry("no configmap", func(*fakeKubeAPI) {}, false, false),
		Entry("no annotation", func(a *fakeKubeAPI) { a.addConfigMap(nil) }, false, false),
		Entry("cluster data", func(a *fakeKubeAPI) {
			a.addConfigMap(map[string]string{util.KubeClusterDataAnnotation: `{"formatVersion":1}`})
		}, true, false),
		Entry("invalid cluster data", func(a *fakeKubeAPI) {
			a.addConfigMap(map[string]string{util.KubeClusterDataAnnotation: "{"})
		}, false, true),
		Entry("api error",
			func(a *fakeKubeAPI) { a.fail("GET configmaps", http.StatusInternalServerError) }, false, true),
	)

	It("puts the cluster data", func() {
		formatVersion := func() uint64 {
			GinkgoHelper()
			cd, _, err := s.GetClusterData(ctx)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(cd).ShouldNot(BeNil())
			return cd.FormatVersion
		}

		// creates the configmap
		Ω(s.PutClusterData(ctx, &cluster.Data{FormatVersion: 1})).Should(Succeed())
		Ω(formatVersion()).Should(Equal(uint64(1)))
		// updates it, retrying on a conflict
		api.conflicts = 1
		Ω(s.PutClusterData(ctx, &cluster.Data{FormatVersion: 2})).Should(Succeed())
		Ω(formatVersion()).Should(Equal(uint64(2)))
		// adds the annotation to a configmap without annotations
		api.addConfigMap(nil)
		Ω(s.PutClusterData(ctx, &cluster.Data{FormatVersion: 3})).Should(Succeed())
		Ω(formatVersion()).Should(Equal(uint64(3)))

		api.fail("GET configmaps", http.StatusInternalServerError)
		Ω(s.PutClusterData(ctx, &cluster.Data{})).ShouldNot(Succeed())
	})

	cdAnnotation := func(v string) map[string]string {
		return map[string]string{util.KubeClusterDataAnnotation: v}
	}
	// wantErr is the expected error, nil for success
	DescribeTable("AtomicPutClusterData",
		func(setup func(*fakeKubeAPI), previous *KVPair, wantErr error) {
			setup(api)
			pair, err := s.AtomicPutClusterData(ctx, &cluster.Data{FormatVersion: 5}, previous)
			if wantErr != nil {
				Ω(err).Should(HaveOccurred())
				if errors.Is(wantErr, ErrKeyModified) {
					Ω(err).Should(MatchError(ErrKeyModified))
				}
				return
			}
			Ω(err).ShouldNot(HaveOccurred())
			const expected = `"formatVersion":5`
			Ω(string(pair.Value)).Should(ContainSubstring(expected), "expected the new cluster data in the pair")
			v, _ := api.configMapAnnotation()
			Ω(v).Should(ContainSubstring(expected), "expected the new cluster data in the configmap")
		},
		Entry("create", func(*fakeKubeAPI) {}, nil, nil),
		Entry("previous but no configmap", func(*fakeKubeAPI) {}, &KVPair{Value: []byte("{}")}, ErrKeyModified),
		Entry("create but cluster data exists",
			func(a *fakeKubeAPI) { a.addConfigMap(cdAnnotation("{}")) }, nil, ErrKeyModified),
		Entry("create on a configmap without annotations", func(a *fakeKubeAPI) { a.addConfigMap(nil) }, nil, nil),
		Entry("create on a configmap with other annotations",
			func(a *fakeKubeAPI) { a.addConfigMap(map[string]string{"other": "x"}) }, nil, nil),
		Entry("previous but no annotations",
			func(a *fakeKubeAPI) { a.addConfigMap(nil) }, &KVPair{Value: []byte("{}")}, ErrKeyModified),
		Entry("previous but no cluster data",
			func(a *fakeKubeAPI) { a.addConfigMap(map[string]string{"other": "x"}) },
			&KVPair{Value: []byte("{}")}, ErrKeyModified),
		Entry("previous differs",
			func(a *fakeKubeAPI) { a.addConfigMap(cdAnnotation(`{"formatVersion":2}`)) },
			&KVPair{Value: []byte("{}")}, ErrKeyModified),
		Entry("previous matches",
			func(a *fakeKubeAPI) { a.addConfigMap(cdAnnotation("{}")) }, &KVPair{Value: []byte("{}")}, nil),
		Entry("previous matches after a conflict", func(a *fakeKubeAPI) {
			a.addConfigMap(cdAnnotation("{}"))
			a.conflicts = 1
		}, &KVPair{Value: []byte("{}")}, nil),
		Entry("api error",
			func(a *fakeKubeAPI) { a.fail("GET configmaps", http.StatusInternalServerError) },
			nil, errors.New("any")),
	)

	DescribeTable("sets the info of a component in the status annotation of its pod",
		func(set func(*KubeStore) error, expected string) {
			// the pod doesn't exist
			Ω(set(s)).ShouldNot(Succeed())

			api.addPod(testPod, KeeperLabelValue, nil)
			Ω(set(s)).Should(Succeed())
			Ω(api.podAnnotation(testPod)).Should(ContainSubstring(expected))

			api.fail("PATCH pods", http.StatusInternalServerError)
			Ω(set(s)).ShouldNot(Succeed())
		},
		Entry("keeper", func(s *KubeStore) error {
			return s.SetKeeperInfo(context.Background(), "k1", &cluster.KeeperInfo{UID: "k1"}, time.Minute)
		}, `"uid":"k1"`),
		Entry("sentinel", func(s *KubeStore) error {
			return s.SetSentinelInfo(context.Background(), &cluster.SentinelInfo{UID: "s1"}, time.Minute)
		}, `"UID":"s1"`),
		Entry("proxy", func(s *KubeStore) error {
			return s.SetProxyInfo(context.Background(), &cluster.ProxyInfo{UID: "p1"}, time.Minute)
		}, `"UID":"p1"`),
	)

	It("gets the keepers info", func() {
		keepers, err := s.GetKeepersInfo(ctx)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(keepers).ShouldNot(BeNil())
		Ω(keepers).Should(BeEmpty())

		api.addPod("keeper1", KeeperLabelValue, &cluster.KeeperInfo{UID: "k1"})
		api.addPod("keeper2", KeeperLabelValue, &cluster.KeeperInfo{UID: "k2"})
		// a keeper that didn't publish its status yet
		api.addPod("keeper3", KeeperLabelValue, nil)
		api.addPod("sentinel1", SentinelLabelValue, &cluster.SentinelInfo{UID: "s1"})
		keepers, err = s.GetKeepersInfo(ctx)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(keepers).Should(HaveLen(2))
		Ω(keepers).Should(HaveKeyWithValue("k1", Not(BeNil())))
		Ω(keepers).Should(HaveKeyWithValue("k2", Not(BeNil())))

		api.addPod("keeper4", KeeperLabelValue, "{")
		_, err = s.GetKeepersInfo(ctx)
		Ω(err).Should(HaveOccurred(), "expected a json error")
		api.fail("GET pods", http.StatusInternalServerError)
		_, err = s.GetKeepersInfo(ctx)
		Ω(err).Should(HaveOccurred())
	})

	It("gets the sentinels info", func() {
		sentinels, err := s.GetSentinelsInfo(ctx)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(sentinels).ShouldNot(BeNil())
		Ω(sentinels).Should(BeEmpty())

		api.addPod("sentinel1", SentinelLabelValue, &cluster.SentinelInfo{UID: "s1"})
		// a sentinel that didn't publish its info yet
		api.addPod("sentinel0", SentinelLabelValue, nil)
		api.addPod("keeper1", KeeperLabelValue, &cluster.KeeperInfo{UID: "k1"})
		sentinels, err = s.GetSentinelsInfo(ctx)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(sentinels).Should(HaveLen(1))
		Ω(sentinels[0].UID).Should(Equal("s1"))

		api.addPod("sentinel2", SentinelLabelValue, "{")
		_, err = s.GetSentinelsInfo(ctx)
		Ω(err).Should(HaveOccurred(), "expected a json error")
		api.fail("GET pods", http.StatusInternalServerError)
		_, err = s.GetSentinelsInfo(ctx)
		Ω(err).Should(HaveOccurred())
	})

	It("gets the proxies info", func() {
		proxies, err := s.GetProxiesInfo(ctx)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(proxies).ShouldNot(BeNil())
		Ω(proxies).Should(BeEmpty())

		api.addPod("proxy1", ProxyLabelValue, &cluster.ProxyInfo{UID: "p1"})
		// a proxy that didn't publish its status yet
		api.addPod("proxy2", ProxyLabelValue, nil)
		api.addPod("keeper1", KeeperLabelValue, &cluster.KeeperInfo{UID: "k1"})
		proxies, err = s.GetProxiesInfo(ctx)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(proxies).Should(HaveLen(1))
		Ω(proxies).Should(HaveKeyWithValue("p1", Not(BeNil())))

		api.addPod("proxy3", ProxyLabelValue, "{")
		_, err = s.GetProxiesInfo(ctx)
		Ω(err).Should(HaveOccurred(), "expected a json error")
		api.fail("GET pods", http.StatusInternalServerError)
		_, err = s.GetProxiesInfo(ctx)
		Ω(err).Should(HaveOccurred())
	})
})

// fakeResourceLock is an in-memory leader election lock.
type fakeResourceLock struct {
	mu     sync.Mutex
	record *resourcelock.LeaderElectionRecord
	getErr error
}

func (l *fakeResourceLock) Get(context.Context) (*resourcelock.LeaderElectionRecord, []byte, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.getErr != nil {
		return nil, nil, l.getErr
	}
	if l.record == nil {
		return nil, nil, apierrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, testConfigMap)
	}
	record := *l.record
	j, _ := json.Marshal(record)
	return &record, j, nil
}

func (l *fakeResourceLock) Create(_ context.Context, ler resourcelock.LeaderElectionRecord) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.record = &ler
	return nil
}

func (l *fakeResourceLock) Update(ctx context.Context, ler resourcelock.LeaderElectionRecord) error {
	return l.Create(ctx, ler)
}

func (l *fakeResourceLock) RecordEvent(string) {}

func (l *fakeResourceLock) Identity() string { return "keeper1" }

func (l *fakeResourceLock) Describe() string { return testNamespace + "/" + testConfigMap }

// nilRecordLock returns no record and no error.
type nilRecordLock struct{ fakeResourceLock }

func (l *nilRecordLock) Get(context.Context) (*resourcelock.LeaderElectionRecord, []byte, error) {
	return nil, nil, nil
}

var _ = Describe("KubeElection", func() {
	// runs the election with the client-go Lease lock against the fake api
	It("runs with a lease", func(ctx SpecContext) {
		api, client := newFakeKubeAPI()
		e1, err := NewKubeElection(client, testPod, testNamespace, testCluster, "sentinel1")
		Ω(err).ShouldNot(HaveOccurred())
		_, err = e1.Leader(ctx)
		Ω(err).Should(MatchError(ErrElectionNoLeader), "expected no leader before any campaign")

		electedCh, errCh := e1.RunForElection(ctx)
		expectElected(electedCh, false)
		expectElected(electedCh, true)
		Ω(api.leaseHolder()).Should(Equal("sentinel1"), "expected the lease to be held by sentinel1")

		// another candidate sees the leader, and doesn't get elected while the lease is renewed
		e2, err := NewKubeElection(client, "pod2", testNamespace, testCluster, "sentinel2")
		Ω(err).ShouldNot(HaveOccurred())
		Ω(e2.Leader(ctx)).Should(Equal("sentinel1"))
		electedCh2, errCh2 := e2.RunForElection(ctx)
		expectElected(electedCh2, false)
		Consistently(electedCh2).WithTimeout(2*retryPeriod).ShouldNot(Receive(),
			"expected no election result while sentinel1 holds the lease")

		e1.Stop()
		expectClosed(electedCh, errCh)
		e2.Stop()
		expectClosed(electedCh2, errCh2)

		api.fail("GET leases", http.StatusInternalServerError)
		_, err = e1.Leader(ctx)
		Ω(err).Should(HaveOccurred())
		Ω(err).ShouldNot(MatchError(ErrElectionNoLeader))
	})

	It("reads the leader from the lock", func(ctx SpecContext) {
		lock := &fakeResourceLock{}
		e := &KubeElection{rl: lock}

		_, err := e.Leader(ctx)
		Ω(err).Should(MatchError(ErrElectionNoLeader), "expected no leader without a lease")
		lock.record = &resourcelock.LeaderElectionRecord{}
		_, err = e.Leader(ctx)
		Ω(err).Should(MatchError(ErrElectionNoLeader), "expected no leader for a lease without a holder")
		lock.record = &resourcelock.LeaderElectionRecord{HolderIdentity: "keeper2"}
		Ω(e.Leader(ctx)).Should(Equal("keeper2"))
		_, err = (&KubeElection{rl: &nilRecordLock{}}).Leader(ctx)
		Ω(err).Should(MatchError(ErrElectionNoLeader))
		lock.getErr = errors.New("boom")
		_, err = e.Leader(ctx)
		Ω(err).Should(HaveOccurred())
		Ω(err).ShouldNot(MatchError(ErrElectionNoLeader))
	})

	It("runs for election", func(ctx SpecContext) {
		e := &KubeElection{rl: &fakeResourceLock{}}

		electedCh, errCh := e.RunForElection(context.Background())
		expectElected(electedCh, false)
		expectElected(electedCh, true)
		Ω(e.Leader(ctx)).Should(Equal("keeper1"))

		Ω(func() { e.RunForElection(context.Background()) }).Should(Panic(), "expected a panic when running twice")

		e.Stop()
		expectClosed(electedCh, errCh)

		Ω(e.Stop).Should(Panic(), "expected a panic when stopping twice")

		// a new campaign after Stop gets new channels; stopping it while nobody reads them ends it too
		electedCh, errCh = e.RunForElection(context.Background())
		expectElected(electedCh, false)
		time.Sleep(100 * time.Millisecond) // it is elected, and waits to report that
		e.Stop()
		expectClosed(electedCh, errCh)
	})
})
