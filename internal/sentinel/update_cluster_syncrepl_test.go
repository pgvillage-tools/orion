// Copyright 2026 PgVillage
// Copyright 2015 Sorint.lab
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

package sentinel

import (
	"testing"
	"time"

	cluster "github.com/pgvillage-tools/orion/api/v1"

	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/util"
)

// TestUpdateClusterSyncRepl: synchronous replication, from the updateCluster table (#15-#31)
func TestUpdateClusterSyncRepl(t *testing.T) {
	runUpdateClusterCases(t, []updateClusterCase{
		// #15 One master and one standby all healthy. Synchronous replication
		// enabled right now in the cluster spec.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d2,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k2,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{},
							ExternalSynchronousStandbys: []string{fakeStandbyName},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #16 One master and one standby. Synchronous replication enabled right
		// now in the cluster spec.
		// master db not healthy: standby elected as new master since
		// dbSpec.SynchronousReplication is false yet. The new master will have
		// SynchronousReplication true and a fake sync stanby.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d2,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k2,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{},
							ExternalSynchronousStandbys: []string{fakeStandbyName},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #17 One master and one standby. Synchronous replication already
		// enabled.
		// master db not healthy: standby elected as new master since it's in
		// the SynchronousStandbys.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d2,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k2,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{d1},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #18 One master and one standby. Synchronous replication already
		// enabled.
		// stanby db not healthy: standby kept inside synchronousStandbys since
		// there's not better standby to choose
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             true,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             true,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #19 One master and two standbys. Synchronous replication already
		// enabled with MinSynchronousStandbys and MaxSynchronousStandbys to 1
		// (default).
		// sync standby db2 not healthy: the other standby db3 choosed as new
		// sync standby. Both will appear as SynchronousStandbys
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             true,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2, d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             true,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #20 From previous. master haven't yet reported the new sync standbys (db2, db3).
		// master db is not healty. db2 is the unique in common between the
		// reported (db2) and the required in the spec (db2, db3) but it's not
		// healty so no master could be elected.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2, d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2, d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #21 From #19. master have not yet reported the new sync standbys as in sync (db3).
		// db2 will remain the unique real in sync db in db1.Status.SynchronousStandbys
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2, d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      2,
							SynchronousStandbys:    []string{d2},
							CurSynchronousStandbys: []string{},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2, d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      2,
							SynchronousStandbys:    []string{d2},
							CurSynchronousStandbys: []string{},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #22 From #21. master have not yet reported the new sync standbys as in sync (db3) and db3 is failed.
		// db2 will remain the unique real in sync db in
		// db1.Status.SynchronousStandbys and also db1.Spec.SynchronousStandbys
		// will contain only db2 (db3 removed)
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2, d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      2,
							SynchronousStandbys:    []string{d2},
							CurSynchronousStandbys: []string{},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g3,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      2,
							SynchronousStandbys:    []string{d2},
							CurSynchronousStandbys: []string{},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #23 From #19. master have reported the new sync standbys as in sync (db2, db3).
		// db2 will be removed from synchronousStandbys
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2, d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      2,
							SynchronousStandbys:    []string{d2, d3},
							CurSynchronousStandbys: []string{d3},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g3,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      2,
							SynchronousStandbys:    []string{d3},
							CurSynchronousStandbys: []string{d3},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #24 From previous.
		// master db is not healty. db3 is the unique in common between the
		// reported (db2, db3) and the required in the spec (db3) so it'll be
		// elected as new master.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g3,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   2,
							SynchronousStandbys: []string{d2, d3},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d3,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g4,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   2,
							SynchronousStandbys: []string{d2, d3},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k3,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{d1},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #25 One master and two standbys. Synchronous replication already
		// enabled with MinSynchronousStandbys and MaxSynchronousStandbys to 2.
		// master (db1) and db2 failed, db3 elected as master.
		// This test checks that the db3 synchronousStandbys are correctly sorted
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2, d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2, d3},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d3,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{d2, d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2, d3},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k3,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{d1, d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #26 One master (unhealthy) and an async standby. Synchronous replication already enabled
		// with MinSynchronousStandbys and MaxSynchronousStandbys to 1 (default)
		// master (db1) and async (db2) with --never-synchronous-replica.
		// db2 is never elected as new sync.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:                 true,
							LastHealthyTime:         now,
							CanBeSynchronousReplica: util.ToPtr(false),
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{},
							ExternalSynchronousStandbys: []string{"orionfakestandby"},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      1,
							SynchronousStandbys:    []string{},
							CurSynchronousStandbys: []string{},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:                 true,
							LastHealthyTime:         now,
							CanBeSynchronousReplica: util.ToPtr(false),
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{},
							ExternalSynchronousStandbys: []string{"orionfakestandby"},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      1,
							SynchronousStandbys:    []string{},
							CurSynchronousStandbys: []string{},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #27 One master (unhealthy) and a sync standby. Synchronous replication already
		// enabled with MinSynchronousStandbys and MaxSynchronousStandbys to 1 (default)
		// master (db1) and sync (db2) with --never-master.
		// db2 is never promoted as new master.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
							CanBeMaster:     util.ToPtr(false),
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:                false,
							CurrentGeneration:      1,
							SynchronousStandbys:    []string{d2},
							CurSynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
							CanBeMaster:     util.ToPtr(false),
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:                false,
							CurrentGeneration:      1,
							SynchronousStandbys:    []string{d2},
							CurSynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #28 One master and one standby. Synchronous replication enabled right
		// now in the cluster spec, with MinSynchronousStandbys=0.
		// master db not healthy: standby elected as new master since
		// dbSpec.SynchronousReplication is false yet. The new master will have
		// SynchronousReplication true and NO fake sync standby.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
						MinSynchronousStandbys: util.ToPtr(uint16(0)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
						MinSynchronousStandbys: util.ToPtr(uint16(0)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d2,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k2,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #29 One master and one standby. Synchronous replication already
		// enabled, with MinSynchronousStandbys=0.
		// master db not healthy: standby elected as new master since it's in
		// the SynchronousStandbys. No fake replica is added.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
						MinSynchronousStandbys: util.ToPtr(uint16(0)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
						MinSynchronousStandbys: util.ToPtr(uint16(0)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d2,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k2,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{d1},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #30 One master and one standby. Synchronous replication already
		// enabled, with MinSynchronousStandbys=0.
		// standby db not healthy: standby removed from synchronousStandbys even though
		// there's not better standby to choose
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
						MinSynchronousStandbys: util.ToPtr(uint16(0)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             true,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
						MinSynchronousStandbys: util.ToPtr(uint16(0)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             true,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #31 One master (unhealthy) and an async standby. Synchronous replication already enabled
		// enabled, with MinSynchronousStandbys=0.
		// master (db1) and async (db2) with --never-synchronous-replica.
		// StrictSyncRepl is set to false. Db2 is never elected as new sync, and fake replica
		// is removed from db1
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
						MinSynchronousStandbys: util.ToPtr(uint16(0)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:                 true,
							LastHealthyTime:         now,
							CanBeSynchronousReplica: util.ToPtr(false),
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{},
							ExternalSynchronousStandbys: []string{"orionfakestandby"},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      1,
							SynchronousStandbys:    []string{},
							CurSynchronousStandbys: []string{},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
						MinSynchronousStandbys: util.ToPtr(uint16(0)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:                 true,
							LastHealthyTime:         now,
							CanBeSynchronousReplica: util.ToPtr(false),
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      1,
							SynchronousStandbys:    []string{},
							CurSynchronousStandbys: []string{},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
	})
}
