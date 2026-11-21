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

// TestUpdateClusterFailover: the Normal phase, failover and standbys, from the updateCluster table (#4-#13)
func TestUpdateClusterFailover(t *testing.T) {
	runUpdateClusterCases(t, []updateClusterCase{
		// Normal phase
		// #4 One master and one standby, both healthy: no change from previous cd
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
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
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
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
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
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
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
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
		// #5 One master and one standby, master db not healthy: standby elected as new master.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
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
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
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
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
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
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #6 From the previous test, new master (db2) converged. Old master setup to follow new master (db2).
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
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
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k2,
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
							Healthy:           true,
							CurrentGeneration: g2,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
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
					d2: &cluster.DB{
						UID:        d2,
						Generation: g3,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k2,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							Role:                        common.RolePrimary,
							SynchronousReplication:      false,
							Followers:                   []string{d3},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g2,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k1,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.ResyncDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d2,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: 0,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d2,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #7 One master and one standby, master db not healthy, standby not converged
		// (old clusterview): no standby elected as new master, clusterview not changed.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
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
							CurrentGeneration: 0,
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
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
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
							CurrentGeneration: 0,
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
		// #8 One master and one standby, master healthy but not converged: standby elected as new master.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
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
							Healthy:           true,
							CurrentGeneration: 0,
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
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
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
							Healthy:           true,
							CurrentGeneration: 0,
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
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
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
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #9 One master and one standby, 3 keepers (one available).
		// Standby ok.
		// No new standby db on free keeper created.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(uint16(1)),
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
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
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
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(uint16(1)),
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
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
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
		// #10 One master and one standby, 3 keepers (one available).
		// Standby failed to converge (keeper healthy).
		// New standby db on free keeper created.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(uint16(1)),
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
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
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
							CurrentGeneration: 0,
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
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(uint16(1)),
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
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
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
							CurrentGeneration: 0,
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
							InitMode:               cluster.ResyncDB,
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
							CurrentGeneration: 0,
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
		// #11 From previous test.
		// new standby db d3 converged, old standby db removed since exceeds MaxStandbysPerSender.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(uint16(1)),
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
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g2,
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
							CurrentGeneration: 0,
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
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(uint16(1)),
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
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d3},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g2,
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
		// #12 One master and one standby, 2 keepers. Standby failed to converge (keeper healthy).
		// No standby db created since there's no free keeper.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(uint16(1)),
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
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
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
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(uint16(1)),
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
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
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
		// #13 One master and one keeper without db assigned.
		// keeper2 dead for more then DeadKeeperRemovalInterval: keeper2 removed.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
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
							LastHealthyTime: now.Add(-100 * time.Hour),
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
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
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
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
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
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
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
