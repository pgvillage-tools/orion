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

package v1

import "github.com/pgvillage-tools/orion/internal/util"

// KeepersInfo stores all KeeperInfo resources belonging to this cluster
type KeepersInfo map[string]*KeeperInfo

// DeepCopy returns a copy of the KeepersInfo resource
func (k *KeepersInfo) DeepCopy() (dc *KeepersInfo) {
	return util.DeepCopy(k)
}

// KeeperInfo can store all info belonging to a Keeper
type KeeperInfo struct {
	// An unique id for this info, used to know when this the keeper info
	// has been updated
	InfoUID string `json:"infoUID,omitempty"`

	UID        string `json:"uid,omitempty"`
	ClusterUID string `json:"clusterUID,omitempty"`
	BootUUID   string `json:"bootUUID,omitempty"`

	PostgresBinaryVersion PostgresBinaryVersion `json:"postgresBinaryVersion,omitempty"`

	PostgresState *PostgresState `json:"postgresState,omitempty"`

	CanBeMaster             *bool `json:"canBeMaster,omitempty"`
	CanBeSynchronousReplica *bool `json:"canBeSynchronousReplica,omitempty"`
}

// DeepCopy returns a copy of the KeeperInfo resource
func (k *KeeperInfo) DeepCopy() (dc *KeeperInfo) {
	return util.DeepCopy(k)
}
