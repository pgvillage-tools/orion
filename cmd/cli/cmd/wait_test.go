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

package cmd

import (
	"testing"

	apiv1 "github.com/pgvillage-tools/orion/api/v1"
)

func TestConverged(t *testing.T) {
	status := func(available bool, primary string, keepers ...apiv1.InfoKeeper) *apiv1.InfoCluster {
		return &apiv1.InfoCluster{
			Status:  apiv1.InfoGenericStatus{"available": available, "primaryKeeper": primary},
			Keepers: keepers,
		}
	}
	done := apiv1.InfoKeeper{UID: "k1", Healthy: true, PgWantedGeneration: 3, PgCurrentGeneration: 3}
	busy := apiv1.InfoKeeper{UID: "k2", Healthy: true, PgWantedGeneration: 4, PgCurrentGeneration: 3}
	down := apiv1.InfoKeeper{UID: "k3", Healthy: false, PgWantedGeneration: 4, PgCurrentGeneration: 1}
	for name, tc := range map[string]struct {
		status     *apiv1.InfoCluster
		notPrimary string
		expected   bool
	}{
		"available and converged":                  {status(true, "k1", done), "", true},
		"not available":                            {status(false, "", done), "", false},
		"a healthy keeper hasn't applied it":       {status(true, "k1", done, busy), "", false},
		"an unhealthy keeper doesn't count":        {status(true, "k1", done, down), "", true},
		"the failed keeper still runs the primary": {status(true, "k1", done), "k1", false},
		"another keeper runs the primary":          {status(true, "k1", done), "k2", true},
	} {
		t.Run(name, func(t *testing.T) {
			if got, reason := converged(tc.status, tc.notPrimary); got != tc.expected {
				t.Errorf("expected %v, got %v (%s)", tc.expected, got, reason)
			}
		})
	}
}
