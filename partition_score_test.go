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
	"math"
	"testing"
)

// TestPoolPartitionScores verifies finite score projections for common signals.
func TestPoolPartitionScores(t *testing.T) {
	tests := []struct {
		name     string
		rates    PoolPartitionWindowRates
		ewma     PoolPartitionEWMAState
		budget   PartitionBudgetSnapshot
		pressure PartitionPressureSnapshot
		check    func(t *testing.T, scores PoolPartitionScores)
	}{
		{
			name: "zero",
			check: func(t *testing.T, scores PoolPartitionScores) {
				if scores.Risk.Value != 0 || scores.Budget.Value != 0 || scores.Pressure.Value != 0 {
					t.Fatalf("zero scores = %+v", scores)
				}
			},
		},
		{
			name: "high usefulness",
			rates: PoolPartitionWindowRates{
				HitRatio:          1,
				RetainRatio:       1,
				GetsPerSecond:     defaultPartitionHighGetsPerSecond,
				PutsPerSecond:     defaultPartitionHighPutsPerSecond,
				LeaseOpsPerSecond: defaultPartitionHighLeaseOpsPerSecond,
			},
			check: func(t *testing.T, scores PoolPartitionScores) {
				if scores.Usefulness.Value < 0.8 || len(scores.Usefulness.Components) == 0 {
					t.Fatalf("usefulness score = %+v", scores.Usefulness)
				}
			},
		},
		{
			name: "high pressure",
			pressure: PartitionPressureSnapshot{
				Enabled: true,
				Level:   PressureLevelCritical,
			},
			check: func(t *testing.T, scores PoolPartitionScores) {
				if scores.Pressure.Value != 1 {
					t.Fatalf("pressure score = %+v", scores.Pressure)
				}
			},
		},
		{
			name: "high risk",
			rates: PoolPartitionWindowRates{
				PoolReturnFailureRatio:          1,
				PoolReturnAdmissionFailureRatio: 1,
				LeaseOwnershipViolationRatio:    1,
			},
			check: func(t *testing.T, scores PoolPartitionScores) {
				if scores.Risk.Value == 0 || scores.Risk.OwnershipComponent == 0 {
					t.Fatalf("risk score = %+v", scores.Risk)
				}
			},
		},
		{
			name: "budget",
			budget: PartitionBudgetSnapshot{
				MaxOwnedBytes:     100,
				CurrentOwnedBytes: 150,
				OwnedOverBudget:   true,
			},
			check: func(t *testing.T, scores PoolPartitionScores) {
				if scores.Budget.Value != 1 || !scores.Budget.OverBudget {
					t.Fatalf("budget score = %+v", scores.Budget)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scores := NewPoolPartitionScores(tt.rates, tt.ewma, tt.budget, tt.pressure)
			requirePoolPartitionScoresFinite(t, scores)
			tt.check(t, scores)
		})
	}
}

// TestPoolPartitionScoresKeepWindowAndLifetimeMetricsSeparate verifies window semantics.
func TestPoolPartitionScoresKeepWindowAndLifetimeMetricsSeparate(t *testing.T) {
	previous := PoolPartitionSample{
		PoolCounters: PoolCountersSnapshot{Gets: 100, Hits: 90, Misses: 10, Allocations: 10},
	}
	current := PoolPartitionSample{
		PoolCounters: PoolCountersSnapshot{Gets: 200, Hits: 100, Misses: 100, Allocations: 100},
	}
	windowRates := NewPoolPartitionWindowRates(NewPoolPartitionWindow(previous, current))
	lifetimeMetrics := newPoolPartitionMetrics("", current)

	if windowRates.HitRatio == float64(lifetimeMetrics.HitRatio) {
		t.Fatalf("window hit ratio unexpectedly equals lifetime hit ratio: window=%v lifetime=%v", windowRates.HitRatio, lifetimeMetrics.HitRatio)
	}
}

func requirePoolPartitionScoresFinite(t *testing.T, scores PoolPartitionScores) {
	t.Helper()
	values := []float64{
		scores.Usefulness.Value,
		scores.Waste.Value,
		scores.Budget.Value,
		scores.Pressure.Value,
		scores.Activity.Value,
		scores.Risk.Value,
	}
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
			t.Fatalf("score value is not finite [0,1]: %+v", scores)
		}
	}
}
