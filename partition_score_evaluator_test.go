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
	"reflect"
	"testing"
)

func TestPoolPartitionScoreEvaluatorDefaultMatchesNewPoolPartitionScores(t *testing.T) {
	rates := PoolPartitionWindowRates{
		HitRatio:                        0.8,
		RetainRatio:                     0.7,
		AllocationRatio:                 0.2,
		DropRatio:                       0.1,
		GetsPerSecond:                   100_000,
		PutsPerSecond:                   80_000,
		PoolReturnFailureRatio:          0.1,
		PoolReturnAdmissionFailureRatio: 0.1,
		LeaseOwnershipViolationRatio:    0.1,
		LeaseInvalidReleaseRatio:        0.1,
		LeaseDoubleReleaseRatio:         0.1,
		PoolReturnClosedFailureRatio:    0.1,
	}
	budget := PartitionBudgetSnapshot{
		MaxOwnedBytes:     100,
		CurrentOwnedBytes: 50,
	}
	pressure := PartitionPressureSnapshot{
		Enabled: true,
		Level:   PressureLevelHigh,
	}
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{})
	if got, want := evaluator.Scores(rates, PoolPartitionEWMAState{}, budget, pressure), NewPoolPartitionScores(rates, PoolPartitionEWMAState{}, budget, pressure); !reflect.DeepEqual(got, want) {
		t.Fatalf("PoolPartitionScoreEvaluator.Scores() = %+v, want %+v", got, want)
	}
}

func TestPoolPartitionScoreEvaluatorCustomWeights(t *testing.T) {
	rates := PoolPartitionWindowRates{
		HitRatio:      1,
		DropRatio:     1,
		GetsPerSecond: defaultPartitionHighGetsPerSecond,
	}
	defaultScores := NewPoolPartitionScores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{
		UsefulnessWeights: PoolPartitionUsefulnessScoreWeights{HitRatio: 1},
		WasteWeights:      PoolPartitionWasteScoreWeights{Drop: 1},
	})
	customScores := evaluator.Scores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	if customScores.Usefulness.Value <= defaultScores.Usefulness.Value {
		t.Fatalf("custom usefulness = %v, want greater than default %v", customScores.Usefulness.Value, defaultScores.Usefulness.Value)
	}
	if customScores.Waste.Value <= defaultScores.Waste.Value {
		t.Fatalf("custom waste = %v, want greater than default %v", customScores.Waste.Value, defaultScores.Waste.Value)
	}
}

func TestPoolPartitionScoreEvaluatorZeroValue(t *testing.T) {
	var evaluator PoolPartitionScoreEvaluator
	rates := PoolPartitionWindowRates{HitRatio: 1, RetainRatio: 1, DropRatio: 1}
	scores := evaluator.Scores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	if scores.Usefulness.Value != 0 || len(scores.Usefulness.Components) != 0 {
		t.Fatalf("zero evaluator usefulness = %+v, want zero", scores.Usefulness)
	}
	if scores.Waste.Value != 0 || len(scores.Waste.Components) != 0 {
		t.Fatalf("zero evaluator waste = %+v, want zero", scores.Waste)
	}
	values := evaluator.ScoreValues(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	if values.Usefulness != 0 || values.Waste != 0 {
		t.Fatalf("zero evaluator values = %+v, want zero usefulness and waste", values)
	}
}

func TestPoolPartitionScoresUseCurrentDropPenalty(t *testing.T) {
	ewma := PoolPartitionEWMAState{
		Initialized:          true,
		HitRatio:             1,
		RetainRatio:          1,
		AllocationRatio:      0,
		GetsPerSecond:        defaultPartitionHighGetsPerSecond,
		PutsPerSecond:        defaultPartitionHighPutsPerSecond,
		AllocationsPerSecond: 0,
		DropRatio:            0,
	}
	currentDrop := PoolPartitionWindowRates{DropRatio: 1}
	scores := NewPoolPartitionScores(currentDrop, ewma, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	if math.Abs(scores.Usefulness.Value-0.65) > 1e-12 {
		t.Fatalf("usefulness with current drop = %v, want 0.65", scores.Usefulness.Value)
	}
	if math.Abs(scores.Waste.Value-0.15) > 1e-12 {
		t.Fatalf("waste with current drop = %v, want 0.15", scores.Waste.Value)
	}

	staleDropEWMA := ewma
	staleDropEWMA.DropRatio = 1
	lowCurrentDrop := PoolPartitionWindowRates{}
	scores = NewPoolPartitionScores(lowCurrentDrop, staleDropEWMA, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	if scores.Usefulness.Value < 0.99 {
		t.Fatalf("stale EWMA drop should not suppress usefulness: %+v", scores.Usefulness)
	}
	if scores.Waste.Value != 0 {
		t.Fatalf("stale EWMA drop should not raise waste: %+v", scores.Waste)
	}
}

func BenchmarkPoolPartitionScoreEvaluatorScores(b *testing.B) {
	rates := PoolPartitionWindowRates{
		HitRatio:        0.8,
		RetainRatio:     0.7,
		AllocationRatio: 0.2,
		DropRatio:       0.1,
		GetsPerSecond:   100_000,
		PutsPerSecond:   80_000,
	}
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = evaluator.Scores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	}
}

func BenchmarkPoolPartitionScoreEvaluatorScoreValues(b *testing.B) {
	rates := PoolPartitionWindowRates{
		HitRatio:        0.8,
		RetainRatio:     0.7,
		AllocationRatio: 0.2,
		DropRatio:       0.1,
		GetsPerSecond:   100_000,
		PutsPerSecond:   80_000,
	}
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = evaluator.ScoreValues(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	}
}
