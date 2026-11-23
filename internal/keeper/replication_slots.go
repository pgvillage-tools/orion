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

package keeper

import (
	"context"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/util"
)

func (p *PostgresKeeper) updateReplSlots(
	ctx context.Context,
	curReplSlots []string,
	uid string,
	followersUIDs,
	additionalReplSlots []string) error {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	internalReplSlots := map[string]struct{}{}

	// Create a list of the wanted internal replication slots
	for _, followerUID := range followersUIDs {
		if followerUID == uid {
			continue
		}
		internalReplSlots[common.OrionName(followerUID)] = struct{}{}
	}

	// Add AdditionalReplicationSlots
	for _, slot := range additionalReplSlots {
		internalReplSlots[common.OrionName(slot)] = struct{}{}
	}

	// Drop internal replication slots
	for _, slot := range curReplSlots {
		if !common.IsOrionName(slot) {
			continue
		}
		if _, ok := internalReplSlots[slot]; !ok {
			logger.Info().Str("slot", slot).Msg("dropping replication slot")
			if err := p.pgm.DropReplicationSlot(ctx, slot); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Str("slot", slot).Msg("failed to drop replication slot")
				// don't return the error but continue also if drop failed (standby still connected)
			}
		}
	}

	// Create internal replication slots
	for slot := range internalReplSlots {
		if !util.StringInSlice(curReplSlots, slot) {
			logger.Info().Str("slot", slot).Msg("creating replication slot")
			if err := p.pgm.CreateReplicationSlot(ctx, slot); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Str("slot", slot).Msg("failed to create replication slot")
				return err
			}
		}
	}
	return nil
}

func (p *PostgresKeeper) refreshReplicationSlots(ctx context.Context, _ *cluster.Data, db *cluster.DB) error {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	var currentReplicationSlots []string
	currentReplicationSlots, err := p.pgm.GetReplicationSlots(ctx)
	if err != nil {
		logger.Debug().AnErr(logging.KeyErr, err).Msg("failed to get replication slots")
		return err
	}

	followersUIDs := db.Spec.Followers

	if err = p.updateReplSlots(
		ctx,
		currentReplicationSlots,
		db.UID,
		followersUIDs,
		db.Spec.AdditionalReplicationSlots); err != nil {
		logger.Debug().AnErr(logging.KeyErr, err).Msg("error updating replication slots")
		return err
	}

	return nil
}
