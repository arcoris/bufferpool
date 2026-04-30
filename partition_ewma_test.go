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
	"time"
)

// TestPoolPartitionEWMAConfig verifies half-life normalization and validation.
func TestPoolPartitionEWMAConfig(t *testing.T) {
	normalized := (PoolPartitionEWMAConfig{}).Normalize()
	if normalized.HalfLife != defaultPartitionEWMAHalfLife {
		t.Fatalf("Normalize().HalfLife = %s, want %s", normalized.HalfLife, defaultPartitionEWMAHalfLife)
	}
	if normalized.MinAlpha != 0 || normalized.MaxAlpha != 1 {
		t.Fatalf("Normalize() alpha bounds = %v/%v, want 0/1", normalized.MinAlpha, normalized.MaxAlpha)
	}
	if err := (PoolPartitionEWMAConfig{HalfLife: time.Second, MinAlpha: 0.1, MaxAlpha: 0.9}).Validate(); err != nil {
		t.Fatalf("Validate() returned error: %v", err)
	}
	for _, config := range []PoolPartitionEWMAConfig{
		{HalfLife: -time.Second},
		{HalfLife: time.Second, MinAlpha: -0.1},
		{HalfLife: time.Second, MinAlpha: 1.1},
		{HalfLife: time.Second, MaxAlpha: 1.1},
		{HalfLife: time.Second, MinAlpha: 0.9, MaxAlpha: 0.1},
		{HalfLife: time.Second, MinAlpha: math.NaN()},
		{HalfLife: time.Second, MaxAlpha: math.Inf(1)},
	} {
		err := config.Validate()
		if !errors.Is(err, ErrInvalidOptions) {
			t.Fatalf("Validate(%+v) error = %v, want ErrInvalidOptions", config, err)
		}
	}
}

// TestPoolPartitionEWMAStateWithUpdate verifies half-life weighted updates.
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

	config := PoolPartitionEWMAConfig{HalfLife: time.Second}
	state := (PoolPartitionEWMAState{}).WithUpdate(config, time.Second, rates)
	if !state.Initialized || state.HitRatio != 1 || state.GetsPerSecond != 10 || state.LeaseOpsPerSecond != 6 {
		t.Fatalf("first update = %+v", state)
	}

	next := state.WithUpdate(config, time.Second, PoolPartitionWindowRates{HitRatio: 0, GetsPerSecond: 20, LeaseOpsPerSecond: 10})
	if next.HitRatio != 0.5 || next.GetsPerSecond != 15 || next.LeaseOpsPerSecond != 8 {
		t.Fatalf("second update = %+v", next)
	}

	zeroElapsed := state.WithUpdate(config, 0, PoolPartitionWindowRates{HitRatio: 0.25})
	if zeroElapsed.HitRatio != state.HitRatio {
		t.Fatalf("zero elapsed update = %+v, want previous %+v", zeroElapsed, state)
	}

	nonFinite := (PoolPartitionEWMAState{}).WithUpdate(config, time.Second, PoolPartitionWindowRates{HitRatio: math.NaN()})
	if nonFinite.HitRatio != 0 {
		t.Fatalf("non-finite update = %+v", nonFinite)
	}
}

func TestPoolPartitionEWMAHalfLifeAlpha(t *testing.T) {
	config := PoolPartitionEWMAConfig{HalfLife: time.Second}

	if got := config.DecayAlpha(0); got != 1 {
		t.Fatalf("DecayAlpha(0) = %v, want 1", got)
	}
	if got := config.DecayAlpha(time.Second); got != 0.5 {
		t.Fatalf("DecayAlpha(half-life) = %v, want 0.5", got)
	}
	if got := config.DecayAlpha(2 * time.Second); got != 0.25 {
		t.Fatalf("DecayAlpha(2 half-lives) = %v, want 0.25", got)
	}
	clamped := (PoolPartitionEWMAConfig{HalfLife: time.Second, MinAlpha: 0.4, MaxAlpha: 0.8}).DecayAlpha(10 * time.Second)
	if clamped != 0.4 {
		t.Fatalf("clamped DecayAlpha = %v, want 0.4", clamped)
	}
}
