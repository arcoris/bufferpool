/*
  Copyright 2026 The ARCORIS Authors

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
*/

package bufferpool

import (
	"errors"
	"math"
	"testing"
)

// TestPoolPartitionEWMAConfig verifies alpha normalization and validation.
func TestPoolPartitionEWMAConfig(t *testing.T) {
	if got := (PoolPartitionEWMAConfig{}).Normalize().Alpha; got != 0.2 {
		t.Fatalf("Normalize().Alpha = %v, want 0.2", got)
	}
	if err := (PoolPartitionEWMAConfig{Alpha: 1}).Validate(); err != nil {
		t.Fatalf("Validate() returned error: %v", err)
	}
	for _, alpha := range []float64{-1, 0, 2, math.NaN(), math.Inf(1)} {
		err := (PoolPartitionEWMAConfig{Alpha: alpha}).Validate()
		if !errors.Is(err, ErrInvalidOptions) {
			t.Fatalf("Validate(%v) error = %v, want ErrInvalidOptions", alpha, err)
		}
	}
}

// TestPoolPartitionEWMAStateWithUpdate verifies first and subsequent updates.
func TestPoolPartitionEWMAStateWithUpdate(t *testing.T) {
	rates := PoolPartitionWindowRates{
		HitRatio:                     1,
		MissRatio:                    0,
		AllocationRatio:              0.25,
		RetainRatio:                  0.75,
		DropRatio:                    0.25,
		PoolReturnFailureRatio:       0.1,
		LeaseInvalidReleaseRatio:     0.2,
		LeaseDoubleReleaseRatio:      0.3,
		LeaseOwnershipViolationRatio: 0.4,
		GetsPerSecond:                10,
		PutsPerSecond:                8,
		AllocationsPerSecond:         2,
		LeaseOpsPerSecond:            6,
	}

	state := (PoolPartitionEWMAState{}).WithUpdate(PoolPartitionEWMAConfig{Alpha: 0.5}, rates)
	if !state.Initialized || state.HitRatio != 1 || state.GetsPerSecond != 10 || state.LeaseOpsPerSecond != 6 {
		t.Fatalf("first update = %+v", state)
	}

	next := state.WithUpdate(PoolPartitionEWMAConfig{Alpha: 0.5}, PoolPartitionWindowRates{HitRatio: 0, GetsPerSecond: 20, LeaseOpsPerSecond: 10})
	if next.HitRatio != 0.5 || next.GetsPerSecond != 15 || next.LeaseOpsPerSecond != 8 {
		t.Fatalf("second update = %+v", next)
	}

	alphaOne := state.WithUpdate(PoolPartitionEWMAConfig{Alpha: 1}, PoolPartitionWindowRates{HitRatio: 0.25})
	if alphaOne.HitRatio != 0.25 {
		t.Fatalf("alpha one update = %+v", alphaOne)
	}

	nonFinite := (PoolPartitionEWMAState{}).WithUpdate(PoolPartitionEWMAConfig{Alpha: 0.5}, PoolPartitionWindowRates{HitRatio: math.NaN()})
	if nonFinite.HitRatio != 0 {
		t.Fatalf("non-finite update = %+v", nonFinite)
	}
}
