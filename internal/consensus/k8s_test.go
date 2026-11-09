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
	"testing"
	"time"

	jsonpatch "github.com/evanphx/json-patch"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/util"
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
)

// fakeKubeAPI is a minimal Kubernetes API server for the configmaps and pods of one namespace,
// enough for a real *kubernetes.Clientset (the KubeStore doesn't accept the client-go fake
// clientset). Requests matching a key ("<METHOD> <resource>", e.g. "GET configmaps") in failures
// fail with that status code; conflicts makes that many updates fail with a conflict first.
type fakeKubeAPI struct {
	mu         sync.Mutex
	configMaps map[string]*corev1.ConfigMap
	pods       map[string]*corev1.Pod
	failures   map[string]int
	conflicts  int
	version    int
}

func newFakeKubeAPI(t *testing.T) (*fakeKubeAPI, *kubernetes.Clientset) {
	t.Helper()
	api := &fakeKubeAPI{
		configMaps: map[string]*corev1.ConfigMap{},
		pods:       map[string]*corev1.Pod{},
		failures:   map[string]int{},
	}
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	client, err := kubernetes.NewForConfig(&rest.Config{
		Host: srv.URL,
		ContentConfig: rest.ContentConfig{
			ContentType:        "application/json",
			AcceptContentTypes: "application/json",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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

func newTestKubeStore(t *testing.T) (*fakeKubeAPI, *KubeStore) {
	t.Helper()
	api, client := newFakeKubeAPI(t)
	s, err := NewKubeStore(client, testPod, testNamespace, testCluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.resourceName != testConfigMap {
		t.Fatalf("expected resource name %q, got %q", testConfigMap, s.resourceName)
	}
	return api, s
}

func TestKubeStoreHealthy(t *testing.T) {
	api, s := newTestKubeStore(t)
	if err := s.Healthy(context.Background()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	api.fail("GET version", http.StatusInternalServerError)
	if err := s.Healthy(context.Background()); err == nil {
		t.Error("expected an error")
	}
}

func TestKubeStoreGetClusterData(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		setup   func(*fakeKubeAPI)
		wantCD  bool
		wantErr bool
	}{
		{name: "no configmap", setup: func(*fakeKubeAPI) {}},
		{name: "no annotation", setup: func(a *fakeKubeAPI) { a.addConfigMap(nil) }},
		{
			name: "cluster data",
			setup: func(a *fakeKubeAPI) {
				a.addConfigMap(map[string]string{util.KubeClusterDataAnnotation: `{"formatVersion":1}`})
			},
			wantCD: true,
		},
		{
			name: "invalid cluster data",
			setup: func(a *fakeKubeAPI) {
				a.addConfigMap(map[string]string{util.KubeClusterDataAnnotation: "{"})
			},
			wantErr: true,
		},
		{
			name:    "api error",
			setup:   func(a *fakeKubeAPI) { a.fail("GET configmaps", http.StatusInternalServerError) },
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, s := newTestKubeStore(t)
			tc.setup(api)
			cd, pair, err := s.GetClusterData(ctx)
			if (err != nil) != tc.wantErr {
				t.Fatalf("expected error %t, got %v", tc.wantErr, err)
			}
			if (cd != nil) != tc.wantCD || (pair != nil) != tc.wantCD {
				t.Fatalf("expected cluster data %t, got %v, %v", tc.wantCD, cd, pair)
			}
			if tc.wantCD && (cd.FormatVersion != 1 || string(pair.Value) != `{"formatVersion":1}`) {
				t.Errorf("unexpected cluster data %v, pair %v", cd, pair)
			}
		})
	}
}

func TestKubeStorePutClusterData(t *testing.T) {
	ctx := context.Background()
	api, s := newTestKubeStore(t)

	// creates the configmap
	if err := s.PutClusterData(ctx, &cluster.Data{FormatVersion: 1}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cd, _, err := s.GetClusterData(ctx); err != nil || cd.FormatVersion != 1 {
		t.Fatalf("unexpected cluster data %v, %v", cd, err)
	}
	// updates it, retrying on a conflict
	api.conflicts = 1
	if err := s.PutClusterData(ctx, &cluster.Data{FormatVersion: 2}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cd, _, err := s.GetClusterData(ctx); err != nil || cd.FormatVersion != 2 {
		t.Fatalf("unexpected cluster data %v, %v", cd, err)
	}
	// adds the annotation to a configmap without annotations
	api.addConfigMap(nil)
	if err := s.PutClusterData(ctx, &cluster.Data{FormatVersion: 3}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cd, _, err := s.GetClusterData(ctx); err != nil || cd.FormatVersion != 3 {
		t.Fatalf("unexpected cluster data %v, %v", cd, err)
	}

	api.fail("GET configmaps", http.StatusInternalServerError)
	if err := s.PutClusterData(ctx, &cluster.Data{}); err == nil {
		t.Error("expected an error")
	}
}

func TestKubeStoreAtomicPutClusterData(t *testing.T) {
	ctx := context.Background()
	cdAnnotation := func(v string) map[string]string {
		return map[string]string{util.KubeClusterDataAnnotation: v}
	}
	for _, tc := range []struct {
		name     string
		setup    func(*fakeKubeAPI)
		previous *KVPair
		// wantErr is the expected error, nil for success
		wantErr error
	}{
		{name: "create", setup: func(*fakeKubeAPI) {}},
		{
			name:     "previous but no configmap",
			setup:    func(*fakeKubeAPI) {},
			previous: &KVPair{Value: []byte("{}")},
			wantErr:  ErrKeyModified,
		},
		{
			name:    "create but cluster data exists",
			setup:   func(a *fakeKubeAPI) { a.addConfigMap(cdAnnotation("{}")) },
			wantErr: ErrKeyModified,
		},
		{name: "create on a configmap without annotations", setup: func(a *fakeKubeAPI) { a.addConfigMap(nil) }},
		{
			name:  "create on a configmap with other annotations",
			setup: func(a *fakeKubeAPI) { a.addConfigMap(map[string]string{"other": "x"}) },
		},
		{
			name:     "previous but no annotations",
			setup:    func(a *fakeKubeAPI) { a.addConfigMap(nil) },
			previous: &KVPair{Value: []byte("{}")},
			wantErr:  ErrKeyModified,
		},
		{
			name:     "previous but no cluster data",
			setup:    func(a *fakeKubeAPI) { a.addConfigMap(map[string]string{"other": "x"}) },
			previous: &KVPair{Value: []byte("{}")},
			wantErr:  ErrKeyModified,
		},
		{
			name:     "previous differs",
			setup:    func(a *fakeKubeAPI) { a.addConfigMap(cdAnnotation(`{"formatVersion":2}`)) },
			previous: &KVPair{Value: []byte("{}")},
			wantErr:  ErrKeyModified,
		},
		{
			name:     "previous matches",
			setup:    func(a *fakeKubeAPI) { a.addConfigMap(cdAnnotation("{}")) },
			previous: &KVPair{Value: []byte("{}")},
		},
		{
			name: "previous matches after a conflict",
			setup: func(a *fakeKubeAPI) {
				a.addConfigMap(cdAnnotation("{}"))
				a.conflicts = 1
			},
			previous: &KVPair{Value: []byte("{}")},
		},
		{
			name:    "api error",
			setup:   func(a *fakeKubeAPI) { a.fail("GET configmaps", http.StatusInternalServerError) },
			wantErr: errors.New("any"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, s := newTestKubeStore(t)
			tc.setup(api)
			pair, err := s.AtomicPutClusterData(ctx, &cluster.Data{FormatVersion: 5}, tc.previous)
			switch {
			case tc.wantErr == nil && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr == nil:
				const expected = `"formatVersion":5`
				if !strings.Contains(string(pair.Value), expected) {
					t.Errorf("expected the new cluster data in the pair, got %s", pair.Value)
				}
				if v, _ := api.configMapAnnotation(); !strings.Contains(v, expected) {
					t.Errorf("expected the new cluster data in the configmap, got %s", v)
				}
			case err == nil:
				t.Fatalf("expected error %v", tc.wantErr)
			case errors.Is(tc.wantErr, ErrKeyModified) && !errors.Is(err, ErrKeyModified):
				t.Errorf("expected ErrKeyModified, got %v", err)
			}
		})
	}
}

func TestKubeStoreSetInfo(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name     string
		set      func(*KubeStore) error
		expected string
	}{
		{
			name: "keeper",
			set: func(s *KubeStore) error {
				return s.SetKeeperInfo(ctx, "k1", &cluster.KeeperInfo{UID: "k1"}, time.Minute)
			},
			expected: `"uid":"k1"`,
		},
		{
			name:     "sentinel",
			set:      func(s *KubeStore) error { return s.SetSentinelInfo(ctx, &cluster.SentinelInfo{UID: "s1"}, time.Minute) },
			expected: `"UID":"s1"`,
		},
		{
			name:     "proxy",
			set:      func(s *KubeStore) error { return s.SetProxyInfo(ctx, &cluster.ProxyInfo{UID: "p1"}, time.Minute) },
			expected: `"UID":"p1"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, s := newTestKubeStore(t)

			// the pod doesn't exist
			if err := tc.set(s); err == nil {
				t.Error("expected an error")
			}

			api.addPod(testPod, KeeperLabelValue, nil)
			if err := tc.set(s); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := api.podAnnotation(testPod); !strings.Contains(got, tc.expected) {
				t.Errorf("expected the status annotation to contain %s, got %s", tc.expected, got)
			}

			api.fail("PATCH pods", http.StatusInternalServerError)
			if err := tc.set(s); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestKubeStoreGetKeepersInfo(t *testing.T) {
	ctx := context.Background()
	api, s := newTestKubeStore(t)

	keepers, err := s.GetKeepersInfo(ctx)
	if err != nil || keepers == nil || len(keepers) != 0 {
		t.Fatalf("expected an empty keepers info, got %v, %v", keepers, err)
	}

	api.addPod("keeper1", KeeperLabelValue, &cluster.KeeperInfo{UID: "k1"})
	api.addPod("keeper2", KeeperLabelValue, &cluster.KeeperInfo{UID: "k2"})
	// a keeper that didn't publish its status yet
	api.addPod("keeper3", KeeperLabelValue, nil)
	api.addPod("sentinel1", SentinelLabelValue, &cluster.SentinelInfo{UID: "s1"})
	keepers, err = s.GetKeepersInfo(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keepers) != 2 || keepers["k1"] == nil || keepers["k2"] == nil {
		t.Errorf("unexpected keepers info %v", keepers)
	}

	api.addPod("keeper4", KeeperLabelValue, "{")
	if _, err = s.GetKeepersInfo(ctx); err == nil {
		t.Error("expected a json error")
	}
	api.fail("GET pods", http.StatusInternalServerError)
	if _, err = s.GetKeepersInfo(ctx); err == nil {
		t.Error("expected an error")
	}
}

func TestKubeStoreGetSentinelsInfo(t *testing.T) {
	ctx := context.Background()
	api, s := newTestKubeStore(t)

	sentinels, err := s.GetSentinelsInfo(ctx)
	if err != nil || sentinels == nil || len(sentinels) != 0 {
		t.Fatalf("expected an empty sentinels info, got %v, %v", sentinels, err)
	}

	api.addPod("sentinel1", SentinelLabelValue, &cluster.SentinelInfo{UID: "s1"})
	api.addPod("keeper1", KeeperLabelValue, &cluster.KeeperInfo{UID: "k1"})
	sentinels, err = s.GetSentinelsInfo(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sentinels) != 1 || sentinels[0].UID != "s1" {
		t.Errorf("unexpected sentinels info %v", sentinels)
	}

	api.addPod("sentinel2", SentinelLabelValue, "{")
	if _, err = s.GetSentinelsInfo(ctx); err == nil {
		t.Error("expected a json error")
	}
	api.fail("GET pods", http.StatusInternalServerError)
	if _, err = s.GetSentinelsInfo(ctx); err == nil {
		t.Error("expected an error")
	}
}

func TestKubeStoreGetProxiesInfo(t *testing.T) {
	ctx := context.Background()
	api, s := newTestKubeStore(t)

	proxies, err := s.GetProxiesInfo(ctx)
	if err != nil || proxies == nil || len(proxies) != 0 {
		t.Fatalf("expected an empty proxies info, got %v, %v", proxies, err)
	}

	api.addPod("proxy1", ProxyLabelValue, &cluster.ProxyInfo{UID: "p1"})
	// a proxy that didn't publish its status yet
	api.addPod("proxy2", ProxyLabelValue, nil)
	api.addPod("keeper1", KeeperLabelValue, &cluster.KeeperInfo{UID: "k1"})
	proxies, err = s.GetProxiesInfo(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(proxies) != 1 || proxies["p1"] == nil {
		t.Errorf("unexpected proxies info %v", proxies)
	}

	api.addPod("proxy3", ProxyLabelValue, "{")
	if _, err = s.GetProxiesInfo(ctx); err == nil {
		t.Error("expected a json error")
	}
	api.fail("GET pods", http.StatusInternalServerError)
	if _, err = s.GetProxiesInfo(ctx); err == nil {
		t.Error("expected an error")
	}
}

func TestNewKubeElection(t *testing.T) {
	_, client := newFakeKubeAPI(t)
	// client-go removed the configmaps resource lock, so this always fails
	if _, err := NewKubeElection(client, testPod, testNamespace, testCluster, "uid"); err == nil {
		t.Error("expected an error")
	}
}

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

func TestKubeElectionLeader(t *testing.T) {
	lock := &fakeResourceLock{}
	e := &KubeElection{rl: lock}

	if _, err := e.Leader(); err == nil {
		t.Error("expected an error without a record")
	}
	lock.record = &resourcelock.LeaderElectionRecord{HolderIdentity: "keeper2"}
	if leader, err := e.Leader(); err != nil || leader != "keeper2" {
		t.Errorf("expected leader keeper2, got %q, %v", leader, err)
	}
	if leader, err := (&KubeElection{rl: &nilRecordLock{}}).Leader(); err != nil || leader != "" {
		t.Errorf("expected no leader, got %q, %v", leader, err)
	}
}

func TestKubeElectionRunForElection(t *testing.T) {
	e := &KubeElection{rl: &fakeResourceLock{}}

	electedCh, _ := e.RunForElection(context.Background())
	expectElected(t, electedCh, false)
	expectElected(t, electedCh, true)
	if leader, err := e.Leader(); err != nil || leader != "keeper1" {
		t.Errorf("expected leader keeper1, got %q, %v", leader, err)
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected a panic when running twice")
			}
		}()
		e.RunForElection(context.Background())
	}()

	e.Stop()
	expectElected(t, electedCh, false)
	// the campaign doesn't stop on a canceled context: it keeps sending false, so the channels are
	// never closed and its goroutine stays blocked once nobody reads them anymore

	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected a panic when stopping twice")
			}
		}()
		e.Stop()
	}()
}
