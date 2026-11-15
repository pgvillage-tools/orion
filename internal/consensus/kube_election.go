// Copyright 2026 PgVillage
// Copyright 2018 Sorint.lab
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
	"fmt"
	"sync"

	"github.com/pgvillage-tools/orion/internal/util"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
)

// KubeElection takes care of the election proces for orion on k8s
type KubeElection struct {
	client       *kubernetes.Clientset
	podName      string
	namespace    string
	resourceName string

	running bool
	cancel  context.CancelFunc

	rl resourcelock.Interface
}

// NewKubeElection returns a freshly initialized KubeElection resource
func NewKubeElection(
	kubecli *kubernetes.Clientset,
	podName, namespace, clusterName, candidateUID string,
) (*KubeElection, error) {
	resourceName := fmt.Sprintf("%s-%s", util.KubeResourcePrefix, clusterName)
	// client-go only supports Lease locks (coordination.k8s.io/v1): it removed the configmaps lock that stolon used.
	// So the sentinels (and the api, for Leader) need get, create and update on leases.
	rl, err := resourcelock.New(resourcelock.LeasesResourceLock,
		namespace,
		resourceName,
		kubecli.CoreV1(),
		kubecli.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity:      candidateUID,
			EventRecorder: createRecorder(kubecli, "orion-sentinel", namespace),
		})
	if err != nil {
		return nil, fmt.Errorf("error creating lock: %v", err)
	}

	return &KubeElection{
		client:       kubecli,
		podName:      podName,
		namespace:    namespace,
		resourceName: resourceName,
		rl:           rl,
	}, nil
}

// RunForElection starts a campaign for getting elected
func (e *KubeElection) RunForElection(ctx context.Context) (<-chan bool, <-chan error) {
	if e.running {
		panic("already running")
	}

	electedCh := make(chan bool)
	errCh := make(chan error)
	ctx, e.cancel = context.WithCancel(ctx)

	e.running = true
	// The campaign gets its own context and channels: a next RunForElection doesn't touch those of a stopped one.
	go e.campaign(ctx, electedCh, errCh)

	return electedCh, errCh
}

// Stop cancels campaign
func (e *KubeElection) Stop() {
	if !e.running {
		panic("not running")
	}
	e.cancel()
	e.running = false
}

// Leader returns the current leader, or ErrElectionNoLeader when there is none (like the etcd election): no sentinel
// created the lease yet, or it has no holder.
func (e *KubeElection) Leader(ctx context.Context) (string, error) {
	ler, _, err := e.rl.Get(ctx)
	if apierrors.IsNotFound(err) {
		return "", ErrElectionNoLeader
	} else if err != nil {
		return "", fmt.Errorf("failed to get leader election record: %w", err)
	}
	if ler == nil || ler.HolderIdentity == "" {
		return "", ErrElectionNoLeader
	}

	return ler.HolderIdentity, nil
}

// campaign runs for leadership until ctx is canceled (by Stop), and reports on electedCh whether this candidate leads.
// It closes both channels when it stops.
func (e *KubeElection) campaign(ctx context.Context, electedCh chan<- bool, errCh chan<- error) {
	// OnStartedLeading runs in a goroutine of its own, so it can still be sending when the campaign returns: mu makes
	// the close wait for that send (which ends, since the campaign only returns once ctx is done), and later sends skip
	// the closed channel.
	var (
		mu     sync.Mutex
		closed bool
	)
	defer func() {
		mu.Lock()
		defer mu.Unlock()
		closed = true
		close(electedCh)
		close(errCh)
	}()

	// send reports elected, unless the campaign is stopped: then nobody may read electedCh anymore.
	send := func(elected bool) bool {
		mu.Lock()
		defer mu.Unlock()
		// checked first: with a reader waiting, select would pick either case once ctx is done
		if closed || ctx.Err() != nil {
			return false
		}
		select {
		case electedCh <- elected:
			return true
		case <-ctx.Done():
			return false
		}
	}
	for send(false) {
		// RunOrDie returns when leadership is lost, or right away when ctx is canceled.
		leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
			Lock:          e.rl,
			LeaseDuration: leaseDuration,
			RenewDeadline: renewDeadline,
			RetryPeriod:   retryPeriod,
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: func(context.Context) { send(true) },
				OnStoppedLeading: func() { send(false) },
			},
		})
		if ctx.Err() != nil {
			return
		}
	}
}

func createRecorder(kubecli kubernetes.Interface, name, namespace string) record.EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(kubecli.CoreV1().RESTClient()).Events(namespace)})
	return eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: name})
}
