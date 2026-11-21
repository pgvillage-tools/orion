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
	"time"

	. "github.com/onsi/ginkgo/v2"
	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/util"
)

var _ = DescribeTable("updateCluster copying spec changes to the db specs (#14)",
	checkUpdateCluster,
	Entry("#14 Changed clusterSpec parameters. RequestTimeout, MaxStandbys, UsePgrewind, "+
		"PGParameters should bet updated in the DBSpecs.", updateClusterCase{
		cd: &cluster.Data{
			Cluster: &cluster.Cluster{
				UID:        c1,
				Generation: g1,
				Spec: &cluster.Spec{
					ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
					InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
					SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
					RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
					MaxStandbys:            util.ToPtr(cluster.DefaultMaxStandbys * 2),
					MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
					AdditionalWalSenders:   util.ToPtr(uint16(cluster.DefaultAdditionalWalSenders) * 2),
					SynchronousReplication: util.ToPtr(true),
					UsePgrewind:            util.ToPtr(true),
					PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
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
						UsePgrewind:                 false,
						PGParameters:                nil,
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
						UsePgrewind:            false,
						PGParameters:           nil,
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
					RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
					MaxStandbys:            util.ToPtr(cluster.DefaultMaxStandbys * 2),
					MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
					AdditionalWalSenders:   util.ToPtr(uint16(cluster.DefaultAdditionalWalSenders) * 2),
					SynchronousReplication: util.ToPtr(true),
					UsePgrewind:            util.ToPtr(true),
					PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
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
						KeeperUID:              k1,
						RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            cluster.DefaultMaxStandbys * 2,
						AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders * 2,
						InitMode:               cluster.NoDB,
						SynchronousReplication: true,
						UsePgrewind:            true,
						PGParameters: cluster.PGParameters{
							p1: v1,
							p2: v2,
						},
						Role:                        common.RolePrimary,
						Followers:                   []string{d2},
						SynchronousStandbys:         []string{d2},
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
					Generation: g2,
					ChangeTime: time.Time{},
					Spec: &cluster.DBSpec{
						KeeperUID:              k2,
						RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            cluster.DefaultMaxStandbys * 2,
						AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders * 2,
						InitMode:               cluster.NoDB,
						SynchronousReplication: false,
						UsePgrewind:            true,
						PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
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
	}),
)
