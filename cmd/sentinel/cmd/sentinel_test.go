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

package cmd

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/timer"
	"github.com/pgvillage-tools/orion/internal/util"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/go-cmp/cmp"
)

var curUID int

var newCluster = cluster.New

var now = time.Now()

func TestUpdateCluster(t *testing.T) {
	const (
		p1 = "param01"
		v1 = "value01"
		p2 = "param02"
		v2 = "value02"

		k1 = "keeper1"
		k2 = "keeper2"
		k3 = "keeper3"

		c1 = "cluster1"

		d1 = "db1"
		d2 = "db2"
		d3 = "db3"

		g1 = 1
		g2 = 2
		g3 = 3
		g4 = 4
	)
	ctx := context.Background()
	tests := []struct {
		cd    *cluster.Data
		outcd *cluster.Data
		err   error
	}{
		// Init phase, also test dbSpec parameters copied from clusterSpec.
		// #0 cluster initialization, no keepers
		{
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
						InitMode:               &newCluster,
						MergePgParameters:      util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
					},
				},
				Keepers: cluster.Keepers{},
				DBs:     cluster.DBs{},
				Proxy:   &cluster.Proxy{},
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
						InitMode:               &newCluster,
						MergePgParameters:      util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
					},
				},
				Keepers: cluster.Keepers{},
				DBs:     cluster.DBs{},
				Proxy:   &cluster.Proxy{},
			},
			err: errors.New("cannot choose initial master: no keepers registered"),
		},
		// #1 cluster initialization, one keeper
		{
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
						InitMode:               &newCluster,
						MergePgParameters:      util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
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
				DBs:   cluster.DBs{},
				Proxy: &cluster.Proxy{},
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
						InitMode:               &newCluster,
						MergePgParameters:      util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
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
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k1,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
							MaxStandbys:            cluster.DefaultMaxStandbys * 2,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders * 2,
							SynchronousReplication: false,
							UsePgrewind:            true,
							// revive:disable-next-line
							PGParameters:                cluster.PGParameters{p1: v1, p2: v2},
							InitMode:                    cluster.NewDB,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							IncludeConfig:               true,
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
					},
				},
				Proxy: &cluster.Proxy{},
			},
		},
		// #2 cluster initialization, more than one keeper, the first will be chosen to be the new master.
		{
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
						SynchronousReplication: util.ToPtr(true),
						UsePgrewind:            util.ToPtr(true),
						PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
						InitMode:               &newCluster,
						MergePgParameters:      util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
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
				DBs:   cluster.DBs{},
				Proxy: &cluster.Proxy{},
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
						SynchronousReplication: util.ToPtr(true),
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						UsePgrewind:            util.ToPtr(true),
						PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
						InitMode:               &newCluster,
						MergePgParameters:      util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
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
							KeeperUID:              k1,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
							MaxStandbys:            cluster.DefaultMaxStandbys * 2,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							UsePgrewind:            true,
							PGParameters: cluster.PGParameters{
								p1: v1,
								p2: v2,
							},
							InitMode:                    cluster.NewDB,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							IncludeConfig:               true,
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
					},
				},
				Proxy: &cluster.Proxy{},
			},
		},
		// #3 cluster initialization, keeper initialization failed
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
						InitMode:             &newCluster,
						MergePgParameters:    util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
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
							InitMode:                    cluster.NewDB,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							CurrentGeneration: 0,
						},
					},
				},
				Proxy: &cluster.Proxy{},
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
						InitMode:             &newCluster,
						MergePgParameters:    util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
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
				DBs:   cluster.DBs{},
				Proxy: &cluster.Proxy{},
			},
		},

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
		// #14 Changed clusterSpec parameters.
		// RequestTimeout, MaxStandbys, UsePgrewind, PGParameters should bet updated in the DBSpecs.
		{
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
		},
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
	}

	for i, tt := range tests {
		s := &Sentinel{uid: "sentinel01", UIDFn: testUIDFn, RandFn: testRandFn,
			dbConvergenceInfos: map[string]*DBConvergenceInfo{}}

		// reset curUID func value to latest db uid
		curUID = 0
		for _, db := range tt.cd.DBs {
			uid, _ := strconv.Atoi(strings.TrimPrefix(db.UID, "db"))
			if uid > curUID {
				curUID = uid
			}
		}

		// Populate db convergence timers, these are populated with a negative
		// timer to make them result like not converged.
		for _, db := range tt.cd.DBs {
			s.dbConvergenceInfos[db.UID] = &DBConvergenceInfo{Generation: 0, Timer: int64(-1000 * time.Hour)}
		}

		fmt.Printf("test #%d\n", i)
		t.Logf("test #%d", i)

		outcd, err := s.updateCluster(ctx, tt.cd, cluster.ProxiesInfo{})
		if tt.err != nil {
			if err == nil {
				t.Errorf("got no error, wanted error: %v", tt.err)
			} else if tt.err.Error() != err.Error() {
				t.Errorf("got error: %v, wanted error: %v", err, tt.err)
			}
		} else {
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			} else if !testEqualCD(outcd, tt.outcd) {
				t.Errorf("wrong outcd: got:\n%s\nwant:\n%s", spew.Sdump(outcd), spew.Sdump(tt.outcd))
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.outcd, outcd))
			}
		}
	}
}

func TestActiveProxiesInfos(t *testing.T) {
	const (
		pr1 = "proxy01"
		pr2 = "proxy02"
	)
	proxyInfo1 := cluster.ProxyInfo{UID: pr1, InfoUID: "infoUID1", ProxyTimeout: cluster.DefaultProxyTimeout}
	proxyInfo2 := cluster.ProxyInfo{UID: pr2, InfoUID: "infoUID2", ProxyTimeout: cluster.DefaultProxyTimeout}
	proxyInfoWithDifferentInfoUID := cluster.ProxyInfo{UID: pr2, InfoUID: "differentInfoUID"}
	var (
		secToNanoSecondMultiplier int64 = 1000000000

		td15ns = 15 * secToNanoSecondMultiplier
		td45ns = 45 * secToNanoSecondMultiplier
	)
	tests := []struct {
		name                       string
		proxyInfoHistories         ProxyInfoHistories
		proxiesInfos               cluster.ProxiesInfo
		expectedActiveProxies      cluster.ProxiesInfo
		expectedProxyInfoHistories ProxyInfoHistories
	}{
		{
			name:                       "should do nothing when called with empty proxyInfos",
			proxyInfoHistories:         nil,
			proxiesInfos:               cluster.ProxiesInfo{},
			expectedActiveProxies:      cluster.ProxiesInfo{},
			expectedProxyInfoHistories: nil,
		},
		{
			name:                  "should append to histories when called with proxyInfos",
			proxyInfoHistories:    make(ProxyInfoHistories),
			proxiesInfos:          cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfo2},
			expectedActiveProxies: cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfo2},
			expectedProxyInfoHistories: ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2},
			},
		},
		{
			name: "should update to histories if infoUID is different",
			proxyInfoHistories: ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1, Timer: timer.Now()},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2, Timer: timer.Now()},
			},
			proxiesInfos:          cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfoWithDifferentInfoUID},
			expectedActiveProxies: cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfoWithDifferentInfoUID},
			expectedProxyInfoHistories: ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfoWithDifferentInfoUID},
			},
		},
		{
			name: "should remove from active proxies if is not updated for twice the DefaultProxyTimeout",
			proxyInfoHistories: ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1, Timer: timer.Now() - (td45ns)},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2, Timer: timer.Now() - (td15ns)},
			},
			proxiesInfos:          cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfo2},
			expectedActiveProxies: cluster.ProxiesInfo{pr2: &proxyInfo2},
			expectedProxyInfoHistories: ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2},
			},
		},
		{
			name: "should remove proxy from sentinel's local history if the proxy is removed in store",
			proxyInfoHistories: ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1, Timer: timer.Now()},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2, Timer: timer.Now()},
			},
			proxiesInfos:               cluster.ProxiesInfo{pr2: &proxyInfo2},
			expectedActiveProxies:      cluster.ProxiesInfo{pr2: &proxyInfo2},
			expectedProxyInfoHistories: ProxyInfoHistories{pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := &Sentinel{
				uid:                "sentinel01",
				UIDFn:              testUIDFn,
				RandFn:             testRandFn,
				dbConvergenceInfos: map[string]*DBConvergenceInfo{},
				proxyInfoHistories: test.proxyInfoHistories,
			}
			actualActiveProxies := s.activeProxiesInfos(test.proxiesInfos)

			if !reflect.DeepEqual(actualActiveProxies, test.expectedActiveProxies) {
				t.Errorf("Expected proxiesInfos to be %v but got %v", test.expectedActiveProxies, actualActiveProxies)
			}
			if !isProxyInfoHistoriesEqual(s.proxyInfoHistories, test.expectedProxyInfoHistories) {
				t.Errorf("Expected proxyInfoHistories to be %v but got %v",
					test.expectedProxyInfoHistories, s.proxyInfoHistories)
			}
		})
	}
}

func isProxyInfoHistoriesEqual(actualProxyInfoHistories ProxyInfoHistories,
	expectedProxyInfoHistories ProxyInfoHistories) bool {
	if len(actualProxyInfoHistories) != len(expectedProxyInfoHistories) {
		return false
	}
	for k, expectedProxyInfoHistory := range expectedProxyInfoHistories {
		actualProxyInfoHistory, ok := actualProxyInfoHistories[k]
		if !ok {
			return false
		}
		if actualProxyInfoHistory.ProxyInfo.InfoUID != expectedProxyInfoHistory.ProxyInfo.InfoUID ||
			actualProxyInfoHistory.ProxyInfo.UID != expectedProxyInfoHistory.ProxyInfo.UID {
			return false
		}
	}
	return true
}

func testUIDFn() string {
	curUID++
	return fmt.Sprintf("%s%d", "db", curUID)
}

func testRandFn(_ int) int {
	return 0
}

func testEqualCD(cd1, cd2 *cluster.Data) bool {
	// ignore times
	for _, cd := range []*cluster.Data{cd1, cd2} {
		cd.Cluster.ChangeTime = time.Time{}
		for _, k := range cd.Keepers {
			k.ChangeTime = time.Time{}
		}
		for _, db := range cd.DBs {
			db.ChangeTime = time.Time{}
		}
		cd.Proxy.ChangeTime = time.Time{}
	}
	return reflect.DeepEqual(cd1, cd2)
}

// Fixtures for the focused election tests below. They build a cluster in the
// Normal phase with db1 (on keeper1) as master and a configurable set of
// standbys, all healthy and converged, so each test only has to change the
// fields it is about.
const (
	etCluster = "cluster1"
	etK1      = "keeper1"
	etK2      = "keeper2"
	etK3      = "keeper3"
	etD1      = "db1"
	etD2      = "db2"
	etD3      = "db3"
	etD4      = "db4"
	etGen1    = 1
	etGen2    = 2
	etTL1     = 1
	etTL2     = 2
	etTL3     = 3
)

func etKeeper(uid string) *cluster.Keeper {
	return &cluster.Keeper{
		UID:    uid,
		Spec:   &cluster.KeeperSpec{},
		Status: cluster.KeeperStatus{Healthy: true, LastHealthyTime: now},
	}
}

func etMaster(uid, keeperUID string, xlogPos uint64, followers ...string) *cluster.DB {
	if followers == nil {
		followers = []string{}
	}
	return &cluster.DB{
		UID:        uid,
		Generation: etGen1,
		Spec: &cluster.DBSpec{
			KeeperUID: keeperUID,
			Role:      common.RolePrimary,
			Followers: followers,
		},
		Status: cluster.DBStatus{
			Healthy:           true,
			CurrentGeneration: etGen1,
			TimelineID:        etTL1,
			XLogPos:           xlogPos,
		},
	}
}

func etStandby(uid, keeperUID, masterUID string, xlogPos uint64) *cluster.DB {
	return &cluster.DB{
		UID:        uid,
		Generation: etGen1,
		Spec: &cluster.DBSpec{
			KeeperUID: keeperUID,
			Role:      common.RoleReplica,
			Followers: []string{},
			FollowConfig: &cluster.FollowConfig{
				Type:  cluster.FollowTypeInternal,
				DBUID: masterUID,
			},
		},
		Status: cluster.DBStatus{
			Healthy:           true,
			CurrentGeneration: etGen1,
			TimelineID:        etTL1,
			XLogPos:           xlogPos,
		},
	}
}

// etClusterData returns a Normal phase cluster with the given master and dbs.
// A keeper is created for every db. Extra keepers without a db can be passed in
// extraKeepers.
func etClusterData(spec *cluster.Spec, masterUID string, dbs []*cluster.DB, extraKeepers ...string) *cluster.Data {
	cd := &cluster.Data{
		Cluster: &cluster.Cluster{
			UID:        etCluster,
			Generation: etGen1,
			Spec:       spec,
			Status: cluster.ClusterStatus{
				CurrentGeneration: etGen1,
				Phase:             cluster.Normal,
				Master:            masterUID,
			},
		},
		Keepers: cluster.Keepers{},
		DBs:     cluster.DBs{},
		Proxy: &cluster.Proxy{
			Generation: etGen1,
			Spec:       cluster.ProxySpec{MasterDBUID: masterUID, EnabledProxies: []string{}},
		},
	}
	for _, db := range dbs {
		cd.DBs[db.UID] = db
		cd.Keepers[db.Spec.KeeperUID] = etKeeper(db.Spec.KeeperUID)
	}
	for _, k := range extraKeepers {
		cd.Keepers[k] = etKeeper(k)
	}
	return cd
}

func etAsyncSpec() *cluster.Spec {
	return &cluster.Spec{
		ConvergenceTimeout: &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
		InitTimeout:        &cluster.Duration{Duration: cluster.DefaultInitTimeout},
		SyncTimeout:        &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
	}
}

func etSyncSpec() *cluster.Spec {
	spec := etAsyncSpec()
	spec.SynchronousReplication = util.ToPtr(true)
	return spec
}

// etSentinel returns a sentinel set up like the TestUpdateCluster loop does:
// reproducible UIDs (continuing after the highest dbN in cd) and convergence
// timers far in the past, so a db that has not converged counts as failed.
func etSentinel(cd *cluster.Data) *Sentinel {
	s := &Sentinel{
		uid:                    "sentinel01",
		UIDFn:                  testUIDFn,
		RandFn:                 testRandFn,
		keeperErrorTimers:      map[string]int64{},
		dbErrorTimers:          map[string]int64{},
		dbNotIncreasingXLogPos: map[string]int64{},
		dbConvergenceInfos:     map[string]*DBConvergenceInfo{},
		keeperInfoHistories:    KeeperInfoHistories{},
	}
	curUID = 0
	for _, db := range cd.DBs {
		uid, _ := strconv.Atoi(strings.TrimPrefix(db.UID, "db"))
		if uid > curUID {
			curUID = uid
		}
		s.dbConvergenceInfos[db.UID] = &DBConvergenceInfo{Generation: 0, Timer: int64(-1000 * time.Hour)}
	}
	return s
}

func etUpdateCluster(t *testing.T, cd *cluster.Data) *cluster.Data {
	t.Helper()
	s := etSentinel(cd)
	outcd, err := s.updateCluster(context.Background(), cd, cluster.ProxiesInfo{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return outcd
}

func etUIDs(dbs []*cluster.DB) []string {
	uids := []string{}
	for _, db := range dbs {
		uids = append(uids, db.UID)
	}
	return uids
}

// TestElectionMaxStandbyLag checks that, with asynchronous replication, a
// standby that is further behind the failed master than MaxStandbyLag is never
// elected, and that one within the limit is.
func TestElectionMaxStandbyLag(t *testing.T) {
	const (
		maxLag        = 1000
		masterXLogPos = 100000
	)
	tests := []struct {
		name       string
		standbyLag map[string]uint64
		wantMaster string
	}{
		{
			name:       "lag over max: no new master, failed master stays",
			standbyLag: map[string]uint64{etD2: maxLag + 1},
			wantMaster: etD1,
		},
		{
			name:       "lag exactly max: elected",
			standbyLag: map[string]uint64{etD2: maxLag},
			wantMaster: etD2,
		},
		{
			name:       "lag under max: elected",
			standbyLag: map[string]uint64{etD2: 1},
			wantMaster: etD2,
		},
		{
			name:       "one standby over max, one under: only the one under max is elected",
			standbyLag: map[string]uint64{etD2: 10 * maxLag, etD3: maxLag / 2},
			wantMaster: etD3,
		},
	}
	keepers := map[string]string{etD2: etK2, etD3: etK3}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := etAsyncSpec()
			spec.MaxStandbyLag = util.ToPtr(uint32(maxLag))
			master := etMaster(etD1, etK1, masterXLogPos)
			master.Status.Healthy = false
			dbs := []*cluster.DB{master}
			for dbUID, lag := range tt.standbyLag {
				master.Spec.Followers = append(master.Spec.Followers, dbUID)
				dbs = append(dbs, etStandby(dbUID, keepers[dbUID], etD1, masterXLogPos-lag))
			}
			outcd := etUpdateCluster(t, etClusterData(spec, etD1, dbs))
			if got := outcd.Cluster.Status.Master; got != tt.wantMaster {
				t.Errorf("got master %q, want %q", got, tt.wantMaster)
			}
		})
	}
}

// TestIsLagBelowMax tests the lag check directly, including a standby that
// reports a higher xlogpos than the master (the uint64 difference wraps but is
// read as a negative int64, so it counts as below max).
func TestIsLagBelowMax(t *testing.T) {
	const (
		maxLag        = 1000
		masterXLogPos = 5000
	)
	spec := etAsyncSpec()
	spec.MaxStandbyLag = util.ToPtr(uint32(maxLag))
	cd := etClusterData(spec, etD1, nil)
	s := etSentinel(cd)
	master := &cluster.DB{UID: etD1, Status: cluster.DBStatus{XLogPos: masterXLogPos}}
	for _, tt := range []struct {
		xlogPos uint64
		want    bool
	}{
		{masterXLogPos, true},
		{masterXLogPos - maxLag, true},
		{masterXLogPos - maxLag - 1, false},
		{0, false},
		{masterXLogPos + maxLag, true},
	} {
		db := &cluster.DB{UID: etD2, Status: cluster.DBStatus{XLogPos: tt.xlogPos}}
		if got := s.isLagBelowMax(context.Background(), cd, master, db); got != tt.want {
			t.Errorf("xlogpos %d: got %t, want %t", tt.xlogPos, got, tt.want)
		}
	}
}

// TestFindBestNewMastersOrder documents the order in which candidates are
// returned. With async replication updateCluster elects bestNewMasters[0].
// NOTE: dbSlice sorts by XLogPos ascending, so the candidate that is FURTHEST
// BEHIND comes first and gets elected. This test pins the current behaviour.
func TestFindBestNewMastersOrder(t *testing.T) {
	const masterXLogPos = 10000
	master := etMaster(etD1, etK1, masterXLogPos, etD2, etD3)
	master.Status.Healthy = false
	cd := etClusterData(etAsyncSpec(), etD1, []*cluster.DB{
		master,
		etStandby(etD2, etK2, etD1, masterXLogPos-10),
		etStandby(etD3, etK3, etD1, masterXLogPos-500),
	})
	s := etSentinel(cd)
	best := s.findBestNewMasters(context.Background(), cd, cd.DBs[etD1])
	if got, want := etUIDs(best), []string{etD3, etD2}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	outcd := etUpdateCluster(t, cd)
	if got := outcd.Cluster.Status.Master; got != etD3 {
		t.Errorf("got elected master %q, want %q (the standby with the lowest xlogpos)", got, etD3)
	}
}

// TestFindBestNewMastersCanBeMaster checks that a keeper started with
// --can-be-master=false is never a candidate.
func TestFindBestNewMastersCanBeMaster(t *testing.T) {
	const xlogPos = 1000
	master := etMaster(etD1, etK1, xlogPos, etD2, etD3)
	master.Status.Healthy = false
	cd := etClusterData(etAsyncSpec(), etD1, []*cluster.DB{
		master,
		etStandby(etD2, etK2, etD1, xlogPos),
		etStandby(etD3, etK3, etD1, xlogPos),
	})
	cd.Keepers[etK2].Status.CanBeMaster = util.ToPtr(false)
	s := etSentinel(cd)
	best := s.findBestNewMasters(context.Background(), cd, cd.DBs[etD1])
	if got, want := etUIDs(best), []string{etD3}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// etTLH returns a timelines history from pairs of timelineID, switchPoint.
func etTLH(entries ...uint64) cluster.PostgresTimelinesHistory {
	tlsh := cluster.PostgresTimelinesHistory{}
	for i := 0; i+1 < len(entries); i += 2 {
		tlsh = append(tlsh, &cluster.PostgresTimelineHistory{TimelineID: entries[i], SwitchPoint: entries[i+1]})
	}
	return tlsh
}

// TestIsDifferentTimelineBranch tests all branches of isDifferentTimelineBranch
// (followed is the master, db the standby).
func TestIsDifferentTimelineBranch(t *testing.T) {
	const switchPoint = 1000
	tests := []struct {
		name     string
		followed cluster.DBStatus
		db       cluster.DBStatus
		want     bool
	}{
		{
			name:     "standby on a higher timeline than the master",
			followed: cluster.DBStatus{TimelineID: etTL1},
			db:       cluster.DBStatus{TimelineID: etTL2},
			want:     true,
		},
		{
			name:     "both on timeline 1 (no history)",
			followed: cluster.DBStatus{TimelineID: etTL1},
			db:       cluster.DBStatus{TimelineID: etTL1},
			want:     false,
		},
		{
			name:     "same timeline, same switchpoint",
			followed: cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
			db:       cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
			want:     false,
		},
		{
			name:     "same timeline, forked at a different switchpoint",
			followed: cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
			db:       cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint+1)},
			want:     true,
		},
		{
			name:     "same timeline, standby has no history",
			followed: cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
			db:       cluster.DBStatus{TimelineID: etTL2},
			want:     false,
		},
		{
			name:     "standby on an older timeline, behind the master's switchpoint",
			followed: cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
			db:       cluster.DBStatus{TimelineID: etTL1, XLogPos: switchPoint - 1},
			want:     false,
		},
		{
			name:     "standby on an older timeline, exactly at the master's switchpoint",
			followed: cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
			db:       cluster.DBStatus{TimelineID: etTL1, XLogPos: switchPoint},
			want:     false,
		},
		{
			name:     "standby on an older timeline, past the master's switchpoint",
			followed: cluster.DBStatus{TimelineID: etTL2, TimelinesHistory: etTLH(etTL1, switchPoint)},
			db:       cluster.DBStatus{TimelineID: etTL1, XLogPos: switchPoint + 1},
			want:     true,
		},
		{
			name:     "standby on an older timeline, master has no history for it",
			followed: cluster.DBStatus{TimelineID: etTL3, TimelinesHistory: etTLH(etTL2, switchPoint)},
			db:       cluster.DBStatus{TimelineID: etTL1, XLogPos: switchPoint + 1},
			want:     false,
		},
	}
	s := &Sentinel{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			followed := &cluster.DB{UID: etD1, Status: tt.followed}
			db := &cluster.DB{UID: etD2, Status: tt.db}
			if got := s.isDifferentTimelineBranch(context.Background(), followed, db); got != tt.want {
				t.Errorf("got %t, want %t", got, tt.want)
			}
		})
	}
}

// etForkedCluster returns a cluster where master db1 is on timeline 2 (forked
// from timeline 1 at switchPoint), db2 is on the same branch and db3 is on a
// timeline 2 that forked at a different point. db3 is further ahead than db2.
func etForkedCluster(switchPoint uint64) *cluster.Data {
	const (
		masterAhead     = 1000
		sameBranchAhead = 100
		forkedAhead     = 900
		forkOffset      = 500
	)
	master := etMaster(etD1, etK1, switchPoint+masterAhead, etD2, etD3)
	master.Status.TimelineID = etTL2
	master.Status.TimelinesHistory = etTLH(etTL1, switchPoint)
	sameBranch := etStandby(etD2, etK2, etD1, switchPoint+sameBranchAhead)
	sameBranch.Status.TimelineID = etTL2
	sameBranch.Status.TimelinesHistory = etTLH(etTL1, switchPoint)
	forked := etStandby(etD3, etK3, etD1, switchPoint+forkedAhead)
	forked.Status.TimelineID = etTL2
	forked.Status.TimelinesHistory = etTLH(etTL1, switchPoint+forkOffset)
	return etClusterData(etAsyncSpec(), etD1, []*cluster.DB{master, sameBranch, forked})
}

// TestElectionTimelineFork checks that, when the master fails, a standby on a
// different timeline branch is not a candidate even though it is further
// ahead, and that a standby on the same branch is elected.
func TestElectionTimelineFork(t *testing.T) {
	const switchPoint = 10000
	cd := etForkedCluster(switchPoint)
	cd.DBs[etD1].Status.Healthy = false
	s := etSentinel(cd)
	if v := s.dbValidity(context.Background(), cd, etD3); v != dbValidityInvalid {
		t.Errorf("forked db3 validity: got %d, want invalid", v)
	}
	best := s.findBestNewMasters(context.Background(), cd, cd.DBs[etD1])
	if got, want := etUIDs(best), []string{etD2}; !reflect.DeepEqual(got, want) {
		t.Errorf("got candidates %v, want %v", got, want)
	}
	outcd := etUpdateCluster(t, cd)
	if got := outcd.Cluster.Status.Master; got != etD2 {
		t.Errorf("got master %q, want %q", got, etD2)
	}
}

// TestElectionOlderTimelineNotCandidate checks that a standby still on the
// previous timeline, behind the switchpoint (so on the same branch and able to
// follow), is valid but is not a candidate for master: findBestStandbys only
// takes standbys on the master's timeline.
func TestElectionOlderTimelineNotCandidate(t *testing.T) {
	const switchPoint = 10000
	cd := etForkedCluster(switchPoint)
	cd.DBs[etD1].Status.Healthy = false
	older := cd.DBs[etD3]
	older.Status.TimelineID = etTL1
	older.Status.TimelinesHistory = nil
	older.Status.XLogPos = switchPoint - 1
	s := etSentinel(cd)
	if v := s.dbValidity(context.Background(), cd, etD3); v != dbValidityValid {
		t.Errorf("db3 validity: got %d, want valid", v)
	}
	best := s.findBestNewMasters(context.Background(), cd, cd.DBs[etD1])
	if got, want := etUIDs(best), []string{etD2}; !reflect.DeepEqual(got, want) {
		t.Errorf("got candidates %v, want %v", got, want)
	}
}

// TestTimelineForkedStandbyResynced checks that, with a healthy master, a
// standby on a different timeline branch is removed and a new db is created on
// its keeper with InitMode resync.
func TestTimelineForkedStandbyResynced(t *testing.T) {
	const switchPoint = 10000
	outcd := etUpdateCluster(t, etForkedCluster(switchPoint))
	if got := outcd.Cluster.Status.Master; got != etD1 {
		t.Fatalf("got master %q, want %q", got, etD1)
	}
	if _, ok := outcd.DBs[etD3]; ok {
		t.Errorf("forked db %s was not removed", etD3)
	}
	if _, ok := outcd.DBs[etD2]; !ok {
		t.Errorf("same branch db %s was removed", etD2)
	}
	newDB, ok := outcd.DBs[etD4]
	if !ok {
		t.Fatalf("no new db %s created, dbs: %v", etD4, outcd.DBs)
	}
	if newDB.Spec.KeeperUID != etK3 || newDB.Spec.InitMode != cluster.ResyncDB {
		t.Errorf("new db: got keeper %q initMode %q, want keeper %q initMode %q",
			newDB.Spec.KeeperUID, newDB.Spec.InitMode, etK3, cluster.ResyncDB)
	}
	if newDB.Spec.FollowConfig == nil || newDB.Spec.FollowConfig.DBUID != etD1 {
		t.Errorf("new db does not follow %s: %v", etD1, newDB.Spec.FollowConfig)
	}
	if got, want := outcd.DBs[etD1].Spec.Followers, []string{etD2, etD4}; !reflect.DeepEqual(got, want) {
		t.Errorf("master followers: got %v, want %v", got, want)
	}
}

// etFailedElectionCluster returns the state right after db2 was elected to
// replace the failed master db1 (see TestUpdateCluster #5), where db2 then
// failed before it promoted (not healthy, not converged), and db1 is back
// healthy. db1 still has the master role (updateCluster keeps the old
// master's role on election) and has the most WAL.
func etFailedElectionCluster(spec *cluster.Spec, syncRepl bool) *cluster.Data {
	const (
		xlogPos = 10000
		behind  = 10
	)
	oldMaster := etMaster(etD1, etK1, xlogPos)
	elected := etMaster(etD2, etK2, xlogPos-behind)
	elected.Generation = etGen2
	elected.Status.Healthy = false
	if syncRepl {
		// on election, the old master replaces the new master in its list
		// of synchronous standbys
		oldMaster.Spec.SynchronousReplication = true
		oldMaster.Spec.SynchronousStandbys = []string{etD2}
		oldMaster.Status.SynchronousStandbys = []string{etD2}
		elected.Spec.SynchronousReplication = true
		elected.Spec.SynchronousStandbys = []string{etD1}
	}
	cd := etClusterData(spec, etD2, []*cluster.DB{oldMaster, elected})
	cd.Proxy.Spec.MasterDBUID = ""
	return cd
}

// TestElectionElectedStandbyFailsBeforePromotion covers an elected standby
// that fails before it promotes, while the old master comes back healthy.
func TestElectionElectedStandbyFailsBeforePromotion(t *testing.T) {
	t.Run("async: old master elected again", func(t *testing.T) {
		outcd := etUpdateCluster(t, etFailedElectionCluster(etAsyncSpec(), false))
		if got := outcd.Cluster.Status.Master; got != etD1 {
			t.Errorf("got master %q, want %q", got, etD1)
		}
	})
	t.Run("async: old master further behind than MaxStandbyLag not elected", func(t *testing.T) {
		const maxLag = 10
		spec := etAsyncSpec()
		spec.MaxStandbyLag = util.ToPtr(uint32(maxLag))
		cd := etFailedElectionCluster(spec, false)
		cd.DBs[etD1].Status.XLogPos = cd.DBs[etD2].Status.XLogPos - maxLag - 1
		outcd := etUpdateCluster(t, cd)
		if got := outcd.Cluster.Status.Master; got != etD2 {
			t.Errorf("got master %q, want %q", got, etD2)
		}
	})
	t.Run("async: old master on an older timeline not elected", func(t *testing.T) {
		cd := etFailedElectionCluster(etAsyncSpec(), false)
		cd.DBs[etD2].Status.TimelineID = etTL2
		outcd := etUpdateCluster(t, cd)
		if got := outcd.Cluster.Status.Master; got != etD2 {
			t.Errorf("got master %q, want %q", got, etD2)
		}
	})
	t.Run("sync: old master not elected again, elected db stays master", func(t *testing.T) {
		// db2 never was a master, so it has no reported (in sync)
		// synchronous standbys: there is nothing in common with its spec, so
		// nobody can be elected.
		outcd := etUpdateCluster(t, etFailedElectionCluster(etSyncSpec(), true))
		if got := outcd.Cluster.Status.Master; got != etD2 {
			t.Errorf("got master %q, want %q", got, etD2)
		}
	})
	t.Run("sync: elected db with stale status from an earlier term elects the old master", func(t *testing.T) {
		// Status.SynchronousStandbys is only maintained for the current
		// master and is not cleared when a db becomes a standby. If db2 was
		// master before (with db1 as sync standby), the stale status matches
		// its new spec and db1 is elected again. Pinned as current behaviour.
		cd := etFailedElectionCluster(etSyncSpec(), true)
		cd.DBs[etD2].Status.SynchronousStandbys = []string{etD1}
		outcd := etUpdateCluster(t, cd)
		if got := outcd.Cluster.Status.Master; got != etD1 {
			t.Errorf("got master %q, want %q", got, etD1)
		}
	})
}

// TestDeadKeeperWithDBNotRemoved is the negative of TestUpdateCluster #13: a
// keeper that has been dead for longer than DeadKeeperRemovalInterval is not
// removed while it still has a db assigned. A dead keeper without a db in the
// same cluster is removed.
func TestDeadKeeperWithDBNotRemoved(t *testing.T) {
	const (
		deadFor = 100 * time.Hour
		xlogPos = 1000
	)
	t.Run("master keeper dead", func(t *testing.T) {
		master := etMaster(etD1, etK1, xlogPos, etD2)
		master.Status.Healthy = false
		cd := etClusterData(etAsyncSpec(), etD1,
			[]*cluster.DB{master, etStandby(etD2, etK2, etD1, xlogPos)}, etK3)
		cd.Keepers[etK1].Status.Healthy = false
		cd.Keepers[etK1].Status.LastHealthyTime = now.Add(-deadFor)
		cd.Keepers[etK3].Status.Healthy = false
		cd.Keepers[etK3].Status.LastHealthyTime = now.Add(-deadFor)
		outcd := etUpdateCluster(t, cd)
		if _, ok := outcd.Keepers[etK1]; !ok {
			t.Errorf("dead keeper %s with db %s was removed", etK1, etD1)
		}
		if _, ok := outcd.Keepers[etK3]; ok {
			t.Errorf("dead keeper %s without db was not removed", etK3)
		}
		if got := outcd.Cluster.Status.Master; got != etD2 {
			t.Errorf("got master %q, want %q", got, etD2)
		}
	})
	t.Run("standby keeper dead", func(t *testing.T) {
		standby := etStandby(etD2, etK2, etD1, xlogPos)
		standby.Status.Healthy = false
		cd := etClusterData(etAsyncSpec(), etD1, []*cluster.DB{etMaster(etD1, etK1, xlogPos, etD2), standby})
		cd.Keepers[etK2].Status.Healthy = false
		cd.Keepers[etK2].Status.LastHealthyTime = now.Add(-deadFor)
		outcd := etUpdateCluster(t, cd)
		if _, ok := outcd.Keepers[etK2]; !ok {
			t.Errorf("dead keeper %s with db %s was removed", etK2, etD2)
		}
		if _, ok := outcd.DBs[etD2]; !ok {
			t.Errorf("db %s of the dead keeper was removed", etD2)
		}
	})
}

// etTimerCD returns a cluster data used by the health helpers to read
// FailInterval.
func etTimerCD(failInterval time.Duration) *cluster.Data {
	spec := etAsyncSpec()
	spec.FailInterval = &cluster.Duration{Duration: failInterval}
	return etClusterData(spec, etD1, nil)
}

// TestKeeperErrorTimer checks isKeeperHealthy with SetKeeperError and
// CleanKeeperError. Time is controlled by writing the timer map directly.
func TestKeeperErrorTimer(t *testing.T) {
	const failInterval = time.Minute
	cd := etTimerCD(failInterval)
	s := etSentinel(cd)
	k := etKeeper(etK1)

	if !s.isKeeperHealthy(cd, k) {
		t.Error("keeper without error timer: want healthy")
	}
	s.SetKeeperError(etK1)
	if !s.isKeeperHealthy(cd, k) {
		t.Error("keeper just in error: want healthy until failInterval passed")
	}
	// SetKeeperError does not restart a running timer
	started := timer.Now() - int64(2*failInterval)
	s.keeperErrorTimers[etK1] = started
	s.SetKeeperError(etK1)
	if s.keeperErrorTimers[etK1] != started {
		t.Error("SetKeeperError restarted a running timer")
	}
	if s.isKeeperHealthy(cd, k) {
		t.Error("keeper in error for longer than failInterval: want unhealthy")
	}
	s.CleanKeeperError(etK1)
	if _, ok := s.keeperErrorTimers[etK1]; ok {
		t.Error("CleanKeeperError did not remove the timer")
	}
	if !s.isKeeperHealthy(cd, k) {
		t.Error("keeper after CleanKeeperError: want healthy")
	}
}

// TestDBErrorTimer checks isDBHealthy with SetDBError and CleanDBError.
func TestDBErrorTimer(t *testing.T) {
	const failInterval = time.Minute
	cd := etTimerCD(failInterval)
	s := etSentinel(cd)
	db := &cluster.DB{UID: etD1}

	if !s.isDBHealthy(cd, db) {
		t.Error("db without error timer: want healthy")
	}
	s.SetDBError(etD1)
	if !s.isDBHealthy(cd, db) {
		t.Error("db just in error: want healthy until failInterval passed")
	}
	started := timer.Now() - int64(2*failInterval)
	s.dbErrorTimers[etD1] = started
	s.SetDBError(etD1)
	if s.dbErrorTimers[etD1] != started {
		t.Error("SetDBError restarted a running timer")
	}
	if s.isDBHealthy(cd, db) {
		t.Error("db in error for longer than failInterval: want unhealthy")
	}
	s.CleanDBError(etD1)
	if !s.isDBHealthy(cd, db) {
		t.Error("db after CleanDBError: want healthy")
	}
}

// TestDBNotIncreasingXLogPos checks isDBIncreasingXLogPos: the db counts as
// increasing until SetDBNotIncreasingXLogPos was called more than
// DefaultDBNotIncreasingXLogPosTimes times (it is a counter of checks, not a
// timer), and Clean resets it.
func TestDBNotIncreasingXLogPos(t *testing.T) {
	s := etSentinel(etTimerCD(time.Minute))
	db := &cluster.DB{UID: etD1}
	if !s.isDBIncreasingXLogPos(db) {
		t.Error("db without counter: want increasing")
	}
	for range cluster.DefaultDBNotIncreasingXLogPosTimes {
		s.SetDBNotIncreasingXLogPos(etD1)
	}
	if !s.isDBIncreasingXLogPos(db) {
		t.Errorf("db not increasing %d times: want still increasing", cluster.DefaultDBNotIncreasingXLogPosTimes)
	}
	s.SetDBNotIncreasingXLogPos(etD1)
	if s.isDBIncreasingXLogPos(db) {
		t.Errorf("db not increasing %d times: want not increasing", cluster.DefaultDBNotIncreasingXLogPosTimes+1)
	}
	s.CleanDBNotIncreasingXLogPos(etD1)
	if !s.isDBIncreasingXLogPos(db) {
		t.Error("db after Clean: want increasing")
	}
}

func etKeeperInfo(keeperUID, infoUID string, state *cluster.PostgresState) *cluster.KeeperInfo {
	return &cluster.KeeperInfo{
		InfoUID:       infoUID,
		UID:           keeperUID,
		ClusterUID:    etCluster,
		BootUUID:      "boot-" + keeperUID,
		PostgresState: state,
	}
}

func etHealthyState(dbUID string, xlogPos uint64) *cluster.PostgresState {
	return &cluster.PostgresState{
		UID:                 dbUID,
		Generation:          etGen1,
		Healthy:             true,
		TimelineID:          etTL1,
		XLogPos:             xlogPos,
		SynchronousStandbys: []string{},
	}
}

// TestUpdateKeepersStatus covers updateKeepersStatus over a few rounds: the
// first run only records history, an unchanged keeper info puts the keeper
// in error, ForceFail makes a keeper unhealthy for one check and is reset,
// keepers of another cluster are ignored, a new keeper is added, db states
// are copied, and a keeper in error for longer than FailInterval is
// unhealthy together with its db.
func TestUpdateKeepersStatus(t *testing.T) {
	const (
		xlogPos  = 5000
		staleFor = time.Hour
		k4       = "keeper4"
	)
	ctx := context.Background()
	spec := etAsyncSpec()
	spec.FailInterval = &cluster.Duration{Duration: time.Minute}
	cd := etClusterData(spec, etD1, []*cluster.DB{
		etMaster(etD1, etK1, xlogPos, etD2),
		etStandby(etD2, etK2, etD1, xlogPos),
	})
	cd.Keepers[etK1].Status.LastHealthyTime = time.Time{}
	cd.Keepers[etK2].Status.ForceFail = true
	s := etSentinel(cd)

	kis := cluster.KeepersInfo{
		etK1: etKeeperInfo(etK1, "info1", etHealthyState(etD1, xlogPos+1)),
		etK2: etKeeperInfo(etK2, "info1", etHealthyState(etD2, xlogPos)),
		etK3: etKeeperInfo(etK3, "info1", nil),
	}
	kis[etK3].ClusterUID = "othercluster"

	// round 1, first run: history only, cd unchanged
	firstcd, kihs := s.updateKeepersStatus(ctx, cd, kis, true)
	if !reflect.DeepEqual(firstcd, cd) {
		t.Errorf("first run changed cd")
	}
	if len(kihs) != 2 || kihs[etK1].Seen || kihs[etK2].Seen {
		t.Errorf("first run: want 2 unseen histories, got %v", kihs)
	}
	s.keeperInfoHistories = kihs

	// round 2, same infos: not updated, so in error (but within failInterval)
	newcd, kihs := s.updateKeepersStatus(ctx, cd, kis, false)
	if _, ok := s.keeperErrorTimers[etK1]; !ok {
		t.Error("keeper1 with unchanged info: want error timer")
	}
	if _, ok := s.dbErrorTimers[etD1]; !ok {
		t.Error("db1 of keeper1 with unchanged info: want error timer")
	}
	if !newcd.Keepers[etK1].Status.Healthy {
		t.Error("keeper1 within failInterval: want healthy")
	}
	if newcd.Keepers[etK1].Status.LastHealthyTime.IsZero() {
		t.Error("zero LastHealthyTime: want set")
	}
	// ForceFail: unhealthy for this check, flag reset, its db unhealthy too
	if k := newcd.Keepers[etK2]; k.Status.Healthy || k.Status.ForceFail {
		t.Errorf("keeper2 with ForceFail: got healthy %t forceFail %t, want false/false",
			k.Status.Healthy, k.Status.ForceFail)
	}
	if newcd.DBs[etD2].Status.Healthy {
		t.Error("db2 on a ForceFail keeper: want unhealthy")
	}
	if _, ok := newcd.Keepers[etK3]; ok {
		t.Error("keeper3 of another cluster was added")
	}
	s.keeperInfoHistories = kihs

	// round 3, new infos: keepers updated and healthy again (ForceFail was
	// reset), db status copied, new keeper added
	kis = cluster.KeepersInfo{
		etK1: etKeeperInfo(etK1, "info2", etHealthyState(etD1, xlogPos+1)),
		etK2: etKeeperInfo(etK2, "info2", etHealthyState(etD2, xlogPos)),
		k4:   etKeeperInfo(k4, "info1", nil),
	}
	kis[etK2].PostgresState.Healthy = false
	before := time.Now()
	newcd2, kihs := s.updateKeepersStatus(ctx, newcd, kis, false)
	for _, uid := range []string{etK1, etK2} {
		if _, ok := s.keeperErrorTimers[uid]; ok {
			t.Errorf("%s with new info: want error timer cleaned", uid)
		}
		k := newcd2.Keepers[uid]
		if !k.Status.Healthy || k.Status.LastHealthyTime.Before(before) {
			t.Errorf("%s with new info: want healthy with updated LastHealthyTime, got %v", uid, k.Status)
		}
		if k.Status.BootUUID != "boot-"+uid {
			t.Errorf("%s: BootUUID not copied: %q", uid, k.Status.BootUUID)
		}
	}
	if _, ok := newcd2.Keepers[k4]; !ok {
		t.Error("new keeper4 not added")
	}
	if _, ok := s.dbErrorTimers[etD1]; ok {
		t.Error("db1 healthy: want error timer cleaned")
	}
	if got := newcd2.DBs[etD1].Status.XLogPos; got != xlogPos+1 {
		t.Errorf("db1 xlogpos: got %d, want %d", got, xlogPos+1)
	}
	if _, ok := s.dbNotIncreasingXLogPos[etD1]; ok {
		t.Error("db1 xlogpos increased: want counter cleaned")
	}
	if got := s.dbNotIncreasingXLogPos[etD2]; got != 1 {
		t.Errorf("db2 xlogpos unchanged: want counter 1, got %d", got)
	}
	if _, ok := s.dbErrorTimers[etD2]; !ok {
		t.Error("db2 reported unhealthy: want error timer")
	}
	s.keeperInfoHistories = kihs

	// round 4: keeper1 info seen but not updated for longer than
	// sleepInterval, in error for longer than failInterval: unhealthy, and
	// its db unhealthy too
	s.sleepInterval = time.Second
	s.keeperInfoHistories[etK1].Timer = timer.Now() - int64(staleFor)
	s.keeperErrorTimers[etK1] = timer.Now() - int64(staleFor)
	newcd3, _ := s.updateKeepersStatus(ctx, newcd2, kis, false)
	if newcd3.Keepers[etK1].Status.Healthy {
		t.Error("keeper1 stale for longer than failInterval: want unhealthy")
	}
	if newcd3.DBs[etD1].Status.Healthy {
		t.Error("db1 on an unhealthy keeper: want unhealthy")
	}

	// round 5: keeper2 reports the state of an unexpected db uid
	kis[etK2] = etKeeperInfo(etK2, "info3", etHealthyState(etD3, xlogPos))
	delete(s.dbErrorTimers, etD2)
	_, _ = s.updateKeepersStatus(ctx, newcd2, kis, false)
	if _, ok := s.dbErrorTimers[etD2]; !ok {
		t.Error("keeper2 reporting another db uid: want error timer for db2")
	}
}

// TestUpdateDBConvergenceInfos checks that a convergence timer is started for
// a db that has not converged, kept while the generation stays the same,
// restarted on a new generation and removed once converged.
func TestUpdateDBConvergenceInfos(t *testing.T) {
	db := etMaster(etD1, etK1, 0)
	cd := etClusterData(etAsyncSpec(), etD1, []*cluster.DB{db})
	s := etSentinel(cd)
	s.dbConvergenceInfos = map[string]*DBConvergenceInfo{}

	s.updateDBConvergenceInfos(cd)
	if _, ok := s.dbConvergenceInfos[etD1]; ok {
		t.Error("converged db: want no convergence info")
	}

	db.Generation = etGen2
	s.updateDBConvergenceInfos(cd)
	ci, ok := s.dbConvergenceInfos[etD1]
	if !ok || ci.Generation != etGen2 {
		t.Fatalf("not converged db: want convergence info for generation %d, got %v", etGen2, ci)
	}
	const old = int64(-1)
	ci.Timer = old
	s.updateDBConvergenceInfos(cd)
	if s.dbConvergenceInfos[etD1].Timer != old {
		t.Error("same generation: timer was restarted")
	}
	db.Generation = etGen2 + 1
	s.updateDBConvergenceInfos(cd)
	if ci = s.dbConvergenceInfos[etD1]; ci.Timer == old || ci.Generation != etGen2+1 {
		t.Errorf("new generation: want restarted timer, got %v", ci)
	}

	db.Status.CurrentGeneration = db.Generation
	s.updateDBConvergenceInfos(cd)
	if _, ok := s.dbConvergenceInfos[etD1]; ok {
		t.Error("db converged: want convergence info removed")
	}
}

// TestUpdateChangeTimes checks that only changed or new objects get a new
// ChangeTime.
func TestUpdateChangeTimes(t *testing.T) {
	cd := etClusterData(etAsyncSpec(), etD1, []*cluster.DB{
		etMaster(etD1, etK1, 0, etD2),
		etStandby(etD2, etK2, etD1, 0),
	})
	newcd := cd.DeepCopy()
	newcd.DBs[etD1].Spec.Followers = []string{}
	newcd.DBs[etD3] = etStandby(etD3, etK3, etD1, 0)
	newcd.Keepers[etK3] = etKeeper(etK3)
	newcd.Keepers[etK2].Status.Healthy = false

	s := etSentinel(cd)
	s.updateChangeTimes(cd, newcd)

	if newcd.ChangeTime.IsZero() {
		t.Error("cluster data ChangeTime not set")
	}
	if !newcd.Cluster.ChangeTime.IsZero() {
		t.Error("unchanged cluster got a ChangeTime")
	}
	if !newcd.Proxy.ChangeTime.IsZero() {
		t.Error("unchanged proxy got a ChangeTime")
	}
	for uid, changed := range map[string]bool{etD1: true, etD2: false, etD3: true} {
		if got := !newcd.DBs[uid].ChangeTime.IsZero(); got != changed {
			t.Errorf("db %s: ChangeTime set %t, want %t", uid, got, changed)
		}
	}
	for uid, changed := range map[string]bool{etK1: false, etK2: true, etK3: true} {
		if got := !newcd.Keepers[uid].ChangeTime.IsZero(); got != changed {
			t.Errorf("keeper %s: ChangeTime set %t, want %t", uid, got, changed)
		}
	}

	newcd2 := newcd.DeepCopy()
	newcd2.Cluster.Status.Master = etD2
	newcd2.Proxy.Spec.MasterDBUID = ""
	s.updateChangeTimes(newcd, newcd2)
	if newcd2.Cluster.ChangeTime.IsZero() || newcd2.Proxy.ChangeTime.IsZero() {
		t.Error("changed cluster and proxy: want ChangeTime set")
	}
}
