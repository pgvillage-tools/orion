// Copyright 2026 PgVillage
// Copyright 2017 Sorint.lab
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

// Package store builds the store and the election of an orion component from its common configuration
package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/consensus"
	"github.com/pgvillage-tools/orion/internal/util"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/kubernetes"
)

// CommonConfig is a struct specifying if certain objects are strings or booleans for example
type CommonConfig struct {
	IsCLI bool

	StoreBackend         string
	StoreEndpoints       string
	StorePrefix          string
	StoreCertFile        string
	StoreKeyFile         string
	StoreCAFile          string
	StoreSkipTLSVerify   bool
	ClusterName          string
	MetricsListenAddress string
	LogColor             bool
	LogLevel             string
	LogComponents        string
	Debug                bool
	KubeConfig           string
	KubeContext          string
	KubeNamespace        string
	StoreTimeout         time.Duration
}

var (
	// clusterIdentifier provides a Prometheus metric that should uniquely identify the
	// cluster that any orion component is associated with. Users can then join between
	// various metric series for the same cluster without making assumptions about service
	// discovery labels.
	clusterIdentifier = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "orion_cluster_identifier",
			Help: "Set to 1, is labelled with the cluster_name and component",
		},
		[]string{"cluster_name", "component"},
	)
)

func init() {
	prometheus.MustRegister(clusterIdentifier)
}

// CheckCommonConfig is a function that checks if two configs match and if not returns an error
func CheckCommonConfig(cfg *CommonConfig) error {
	if cfg.ClusterName == "" {
		return errors.New("cluster name required")
	}
	if cfg.StoreBackend == "" {
		return errors.New("store backend type required")
	}

	switch cfg.StoreBackend {
	case "etcdv3", "kubernetes":
	case "etcd", "etcdv2", "consul":
		return fmt.Errorf("store backend %q is no longer supported: use etcdv3 or kubernetes", cfg.StoreBackend)
	default:
		return fmt.Errorf("Unknown store backend: %q", cfg.StoreBackend)
	}

	return nil
}

// SetMetrics should be called by any orion component that outputs application metrics.
// It sets the clusterIdentifier metric, which is key to joining across all the other
// metric series.
func SetMetrics(cfg *CommonConfig, component string) {
	clusterIdentifier.WithLabelValues(cfg.ClusterName, component).Set(1)
}

// NewKVStore returns a new KVStore function object
func NewKVStore(ctx context.Context, cfg *CommonConfig) (consensus.KVStore, error) {
	return consensus.NewKVStore(
		ctx,
		consensus.Config{
			Backend:       consensus.BackendType(cfg.StoreBackend),
			Endpoints:     cfg.StoreEndpoints,
			Timeout:       cfg.StoreTimeout,
			CertFile:      cfg.StoreCertFile,
			KeyFile:       cfg.StoreKeyFile,
			CAFile:        cfg.StoreCAFile,
			SkipTLSVerify: cfg.StoreSkipTLSVerify,
		})
}

// NewStore is function that returns a new store object
func NewStore(ctx context.Context, cfg *CommonConfig) (consensus.Store, error) {
	var s consensus.Store

	switch cfg.StoreBackend {
	case "etcdv3":
		kvstore, err := NewKVStore(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("NewStore: cannot create etcdv3 store: %v", err)
		}
		s = consensus.NewKVBackedStore(kvstore, storePath(cfg))
	case "kubernetes":
		kubecli, podName, namespace, err := getKubeValues(cfg, !cfg.IsCLI)
		if err != nil {
			return nil, err
		}
		s, err = consensus.NewKubeStore(kubecli, podName, namespace, cfg.ClusterName)
		if err != nil {
			return nil, fmt.Errorf("NewStore: cannot create k8s store: %v", err)
		}
	}

	return s, nil
}

// NewElection is  function that returns a new election object
func NewElection(ctx context.Context, cfg *CommonConfig, uid string) (consensus.Election, error) {
	var election consensus.Election

	switch cfg.StoreBackend {
	case "etcdv3":
		kvstore, err := NewKVStore(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("NewElection: cannot create kv store: %v", err)
		}
		election = newKVBackedElection(kvstore, cfg, uid)
	case "kubernetes":
		kubecli, podName, namespace, err := getKubeValues(cfg, !cfg.IsCLI)
		if err != nil {
			return nil, err
		}
		election, err = consensus.NewKubeElection(kubecli, podName, namespace, cfg.ClusterName, uid)
		if err != nil {
			return nil, err
		}
	}

	return election, nil
}

// NewStoreAndElection returns a store and an election that share one connection to the store backend. It is meant for
// a long running process that needs both, like the api: NewStore and NewElection each open their own connection. The
// process is not a member of the cluster (it doesn't publish its own state), so with the kubernetes backend it needs
// no POD_NAME.
func NewStoreAndElection(ctx context.Context, cfg *CommonConfig, uid string) (consensus.Store, consensus.Election,
	error) {
	switch cfg.StoreBackend {
	case "etcdv3":
		kvstore, err := NewKVStore(ctx, cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("cannot create kv store: %w", err)
		}
		return consensus.NewKVBackedStore(kvstore, storePath(cfg)), newKVBackedElection(kvstore, cfg, uid), nil
	case "kubernetes":
		kubecli, podName, namespace, err := getKubeValues(cfg, false)
		if err != nil {
			return nil, nil, err
		}
		s, err := consensus.NewKubeStore(kubecli, podName, namespace, cfg.ClusterName)
		if err != nil {
			return nil, nil, fmt.Errorf("cannot create k8s store: %w", err)
		}
		e, err := consensus.NewKubeElection(kubecli, podName, namespace, cfg.ClusterName, uid)
		if err != nil {
			return nil, nil, fmt.Errorf("cannot create k8s election: %w", err)
		}
		return s, e, nil
	}
	return nil, nil, fmt.Errorf("unknown store backend %q", cfg.StoreBackend)
}

// storePath is the path of the cluster in a kv store
func storePath(cfg *CommonConfig) string {
	return filepath.Join(cfg.StorePrefix, cfg.ClusterName)
}

func newKVBackedElection(kvstore consensus.KVStore, cfg *CommonConfig, uid string) consensus.Election {
	return consensus.NewKVBackedElection(kvstore, filepath.Join(storePath(cfg), common.SentinelLeaderKey), uid,
		cfg.StoreTimeout)
}

// getKubeValues returns a kubernetes client, the name of the own pod (POD_NAME, required for a member: a keeper,
// sentinel or proxy publishes its state on its pod), and the namespace
func getKubeValues(cfg *CommonConfig, member bool) (*kubernetes.Clientset, string, string, error) {
	kubeClientConfig := util.NewKubeClientConfig(cfg.KubeConfig, cfg.KubeContext, cfg.KubeNamespace)
	kubecfg, err := kubeClientConfig.ClientConfig()
	if err != nil {
		return nil, "", "", err
	}
	kubecfg.Timeout = cfg.StoreTimeout
	kubecli, err := kubernetes.NewForConfig(kubecfg)
	if err != nil {
		return nil, "", "", fmt.Errorf("cannot create kubernetes client: %v", err)
	}
	var podName string
	if member {
		podName, err = util.PodName()
		if err != nil {
			return nil, "", "", err
		}
	}
	namespace, _, err := kubeClientConfig.Namespace()
	if err != nil {
		return nil, "", "", err
	}
	return kubecli, podName, namespace, nil
}
