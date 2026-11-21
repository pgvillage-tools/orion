// Copyright 2026 PgVillage
// Copyright 2016 Sorint.lab
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

// Package v1 defines all resources for version v1
package v1

// TODO: Split into multiple modules
import (
	"errors"
	"fmt"
	"time"

	"github.com/pgvillage-tools/orion/internal/util"
)

// TODO: Rename XLOG to WAL everywhere

const (
	// CurrentCDFormatVersion represents the current version of the Cluster Data Format.
	// It will be raised whenever we change the API
	CurrentCDFormatVersion uint64 = 1

	// DefaultStoreTimeout sets the default for a store timeout
	DefaultStoreTimeout = 5 * time.Second

	// DefaultDBNotIncreasingXLogPosTimes sets a default for the number of checks that it is ok for orion
	// not to detect XLog position increases on a standby. If WAL position is not increased more then this value,
	// the standby is assumed not to be syncing properly.
	DefaultDBNotIncreasingXLogPosTimes = 10

	// DefaultSleepInterval is the default for sleeps during checks
	DefaultSleepInterval = 5 * time.Second

	// DefaultRequestTimeout is the default for a request to time out
	DefaultRequestTimeout = 10 * time.Second

	// DefaultConvergenceTimeout sets the timeout for convergence (of primaries and replica's) to be successful.
	DefaultConvergenceTimeout = 30 * time.Second

	// DefaultInitTimeout sets the default timeout fo initializing a new cluster
	DefaultInitTimeout = 5 * time.Minute

	// DefaultSyncTimeout sets the default timeout for waiting for a database recovery
	// (including the replay of WAL files in case of Point-In-Time-Recovery)
	DefaultSyncTimeout = 0

	// DefaultDBWaitReadyTimeout sets the default for Ready status (being able to connect to PostgreSQL)
	DefaultDBWaitReadyTimeout = 60 * time.Second

	// DefaultFailInterval sets the default for the cluster to be assumed unhealthy
	DefaultFailInterval = 20 * time.Second

	// DefaultDeadKeeperRemovalInterval is the interval after which a keeper is assumed dead and is removed from the
	// config.
	DefaultDeadKeeperRemovalInterval = 48 * time.Hour

	// DefaultProxyCheckInterval is the default for the interval for the proxy to check the endpoint
	DefaultProxyCheckInterval = 5 * time.Second

	// DefaultProxyTimeout is the default for the proxy check timeout. Once expired, all connections will be closed.
	DefaultProxyTimeout = 15 * time.Second

	// DefaultMaxStandbys sets the default for teh number of standby's per keeper (input for max_replication_slots and
	// max_wal_senders)
	DefaultMaxStandbys uint16 = 20

	// DefaultMaxStandbysPerSender sets the default for number of standby's before cleanup of old standby's is triggered
	DefaultMaxStandbysPerSender uint16 = 3

	// DefaultMaxStandbyLag sets the default for the maximum lag of a standby (1MiB of WAL): a standby that lags more
	// is not elected as new primary, nor chosen as synchronous standby (yet)
	DefaultMaxStandbyLag = 1024 * 1024

	// DefaultSynchronousReplication sets the default for sync replication when not set by config
	DefaultSynchronousReplication = false

	// DefaultMinSynchronousStandbys sets the default for minimum number of sync standby's
	DefaultMinSynchronousStandbys uint16 = 1

	// DefaultMaxSynchronousStandbys sets the default for maximum number of sync standby's
	DefaultMaxSynchronousStandbys uint16 = 1

	// DefaultAdditionalWalSenders sets the default for additional wal-senders on top of as required for orion managed
	DefaultAdditionalWalSenders = 5

	// DefaultUsePgrewind sets the default for using PgRewind (over starting over)
	// TODO: enable pgrewind by default
	DefaultUsePgrewind = false

	// DefaultResyncFastCheckpoint sets the default for an immediate checkpoint on the primary when a resync starts
	DefaultResyncFastCheckpoint = false

	// DefaultMergePGParameter sets the default for merging parameters from local cluster to config after initialization
	DefaultMergePGParameter = true

	// DefaultRole sets the default role for this cluster. A primary cluster has a primary, a Standby has only replica's
	// and is replicating from another cluster
	DefaultRole Role = Primary

	// DefaultSUReplAccess sets the default for replication access (strict: only from other standby's in this cluster to
	// primary; all: any replication conenction is accepted when properly authenticated)
	DefaultSUReplAccess SUReplAccessMode = SUReplAccessAll

	// DefaultAutomaticPgRestart sets the default for restarting postgres when required to apply config changes
	DefaultAutomaticPgRestart = false
)

const (
	// NoGeneration is the default Generation when not converging
	NoGeneration int64 = 0
	// InitialGeneration is the default generation when starting convergence
	InitialGeneration int64 = 1
)

// ClusterStatus stores the status info for this cluster
type ClusterStatus struct {
	CurrentGeneration int64 `json:"currentGeneration,omitempty"`
	Phase             Phase `json:"phase,omitempty"`
	// Master DB UID
	Master string `json:"master,omitempty"`
}

// Cluster wraps cluster config, cluster status, UUID, etc together
type Cluster struct {
	UID        string    `json:"uid,omitempty"`
	Generation int64     `json:"generation,omitempty"`
	ChangeTime time.Time `json:"changeTime,omitempty"`

	Spec *Spec `json:"spec,omitempty"`

	Status ClusterStatus `json:"status,omitempty"`
}

// DeepCopy copies the entire structure and returns a complete clone
func (c *Cluster) DeepCopy() (dc *Cluster) {
	return util.DeepCopy(c)
}

// DefSpec returns a new Spec with unspecified values populated with
// their defaults
func (c *Cluster) DefSpec() *Spec {
	s := c.Spec
	if s == nil {
		s = &Spec{}
	}
	return s.WithDefaults()
}

// UpdateSpec will update the spec of a cluster
func (c *Cluster) UpdateSpec(ns *Spec) error {
	s := c.Spec
	if err := ns.Validate(); err != nil {
		return fmt.Errorf("invalid cluster spec: %v", err)
	}
	ds := s.WithDefaults()
	dns := ns.WithDefaults()
	if *ds.InitMode != *dns.InitMode {
		return errors.New("cannot change cluster init mode")
	}
	if *ds.Role == Primary && *dns.Role == Replica {
		return errors.New("cannot update a cluster from master role to standby role")
	}
	c.Spec = ns
	return nil
}

// NewCluster returns a freshly initialized cluster object
func NewCluster(uid string, cs *Spec) *Cluster {
	c := &Cluster{
		UID:        uid,
		Generation: InitialGeneration,
		ChangeTime: time.Now(),
		Spec:       cs,
		Status: ClusterStatus{
			Phase: Initializing,
		},
	}
	return c
}
