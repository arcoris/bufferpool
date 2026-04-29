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

var (
	partitionScoreEvaluatorScoresSink PoolPartitionScores
	partitionScoreEvaluatorValuesSink PoolPartitionScoreValues
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

func TestPoolPartitionScoreEvaluatorCustomUsefulnessWeights(t *testing.T) {
	rates := PoolPartitionWindowRates{
		HitRatio:      1,
		DropRatio:     1,
		GetsPerSecond: defaultPartitionHighGetsPerSecond,
	}
	defaultScores := NewPoolPartitionScores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{
		UsefulnessWeights: PoolPartitionUsefulnessScoreWeights{HitRatio: 1},
	})
	customScores := evaluator.Scores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	if customScores.Usefulness.Value <= defaultScores.Usefulness.Value {
		t.Fatalf("custom usefulness = %v, want greater than default %v", customScores.Usefulness.Value, defaultScores.Usefulness.Value)
	}
}

func TestPoolPartitionScoreEvaluatorCustomWasteWeights(t *testing.T) {
	rates := PoolPartitionWindowRates{
		DropRatio: 1,
	}
	defaultScores := NewPoolPartitionScores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{
		WasteWeights: PoolPartitionWasteScoreWeights{Drop: 1},
	})
	customScores := evaluator.Scores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	if customScores.Waste.Value <= defaultScores.Waste.Value {
		t.Fatalf("custom waste = %v, want greater than default %v", customScores.Waste.Value, defaultScores.Waste.Value)
	}
}

func TestPoolPartitionScoreEvaluatorCustomActivityConfig(t *testing.T) {
	rates := PoolPartitionWindowRates{GetsPerSecond: 1}
	defaultScores := NewPoolPartitionScores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{
		ActivityConfig: PoolPartitionActivityScoreConfig{
			HighGetsPerSecond: 1,
			GetsWeight:        1,
		},
	})

	customScores := evaluator.Scores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	if customScores.Activity.Value <= defaultScores.Activity.Value {
		t.Fatalf("custom activity = %v, want greater than default %v", customScores.Activity.Value, defaultScores.Activity.Value)
	}
}

func TestPoolPartitionScoreEvaluatorCustomRiskConfig(t *testing.T) {
	rates := PoolPartitionWindowRates{LeaseInvalidReleaseRatio: 1}
	defaultScores := NewPoolPartitionScores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{
		RiskConfig: PoolPartitionRiskScoreConfig{
			Weights:       PoolPartitionRiskScoreWeights{Misuse: 1},
			MisuseWeights: PoolPartitionMisuseRiskWeights{InvalidRelease: 1},
		},
	})

	customScores := evaluator.Scores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	if customScores.Risk.Value <= defaultScores.Risk.Value {
		t.Fatalf("custom risk = %v, want greater than default %v", customScores.Risk.Value, defaultScores.Risk.Value)
	}
}

func TestPoolPartitionScoreEvaluatorPartialUsefulnessWeightsAreExplicit(t *testing.T) {
	rates := PoolPartitionWindowRates{
		HitRatio:        1,
		RetainRatio:     1,
		AllocationRatio: 1,
	}
	defaultScores := NewPoolPartitionScores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{
		UsefulnessWeights: PoolPartitionUsefulnessScoreWeights{HitRatio: 1},
	})

	customScores := evaluator.Scores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	if customScores.Usefulness.Value != 1 {
		t.Fatalf("partial usefulness weights = %v, want explicit hit-only score 1", customScores.Usefulness.Value)
	}
	if customScores.Usefulness.Value == defaultScores.Usefulness.Value {
		t.Fatalf("partial usefulness weights unexpectedly matched defaults: %v", customScores.Usefulness.Value)
	}
	if !isFiniteUnit(customScores.Usefulness.Value) {
		t.Fatalf("partial usefulness value = %v, want finite unit value", customScores.Usefulness.Value)
	}
}

func TestPoolPartitionScoreEvaluatorPartialWasteWeightsAreExplicit(t *testing.T) {
	rates := PoolPartitionWindowRates{
		HitRatio:  1,
		DropRatio: 1,
	}
	defaultScores := NewPoolPartitionScores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{
		WasteWeights: PoolPartitionWasteScoreWeights{Drop: 1},
	})

	customScores := evaluator.Scores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	if customScores.Waste.Value != 1 {
		t.Fatalf("partial waste weights = %v, want explicit drop-only score 1", customScores.Waste.Value)
	}
	if customScores.Waste.Value == defaultScores.Waste.Value {
		t.Fatalf("partial waste weights unexpectedly matched defaults: %v", customScores.Waste.Value)
	}
	if !isFiniteUnit(customScores.Waste.Value) {
		t.Fatalf("partial waste value = %v, want finite unit value", customScores.Waste.Value)
	}
}

func TestPoolPartitionScoreEvaluatorPartialActivityConfigIsExplicit(t *testing.T) {
	rates := PoolPartitionWindowRates{GetsPerSecond: 1}
	defaultScores := NewPoolPartitionScores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{
		ActivityConfig: PoolPartitionActivityScoreConfig{HighGetsPerSecond: 1},
	})

	customScores := evaluator.Scores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	if customScores.Activity.Value != 1 {
		t.Fatalf("partial activity config = %v, want explicit get-only score 1", customScores.Activity.Value)
	}
	if customScores.Activity.Value == defaultScores.Activity.Value {
		t.Fatalf("partial activity config unexpectedly matched defaults: %v", customScores.Activity.Value)
	}
	if !isFiniteUnit(customScores.Activity.Value) {
		t.Fatalf("partial activity value = %v, want finite unit value", customScores.Activity.Value)
	}
}

func TestPoolPartitionScoreEvaluatorPartialRiskConfigIsExplicit(t *testing.T) {
	rates := PoolPartitionWindowRates{LeaseInvalidReleaseRatio: 1}
	defaultScores := NewPoolPartitionScores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{
		RiskConfig: PoolPartitionRiskScoreConfig{
			Weights: PoolPartitionRiskScoreWeights{Misuse: 1},
		},
	})

	customScores := evaluator.Scores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	if customScores.Risk.Value != 0 || customScores.Risk.MisuseComponent != 0 {
		t.Fatalf("partial risk config = %+v, want explicit zero omitted misuse weights", customScores.Risk)
	}
	if defaultScores.Risk.Value == 0 {
		t.Fatalf("default risk = %+v, want default misuse weights to contribute", defaultScores.Risk)
	}
	if !isFiniteUnit(customScores.Risk.Value) {
		t.Fatalf("partial risk value = %v, want finite unit value", customScores.Risk.Value)
	}
}

func TestPoolPartitionScoreEvaluatorZeroValue(t *testing.T) {
	var evaluator PoolPartitionScoreEvaluator
	rates := PoolPartitionWindowRates{
		HitRatio:                     1,
		RetainRatio:                  1,
		DropRatio:                    1,
		GetsPerSecond:                defaultPartitionHighGetsPerSecond,
		LeaseInvalidReleaseRatio:     1,
		LeaseOwnershipViolationRatio: 1,
	}
	budget := PartitionBudgetSnapshot{
		MaxOwnedBytes:     100,
		CurrentOwnedBytes: 150,
		OwnedOverBudget:   true,
	}
	pressure := PartitionPressureSnapshot{
		Enabled: true,
		Level:   PressureLevelCritical,
	}
	scores := evaluator.Scores(rates, PoolPartitionEWMAState{}, budget, pressure)
	if scores.Usefulness.Value != 0 || len(scores.Usefulness.Components) != 0 {
		t.Fatalf("zero evaluator usefulness = %+v, want zero", scores.Usefulness)
	}
	if scores.Waste.Value != 0 || len(scores.Waste.Components) != 0 {
		t.Fatalf("zero evaluator waste = %+v, want zero", scores.Waste)
	}
	if scores.Activity.Value != 0 || scores.Risk.Value != 0 {
		t.Fatalf("zero evaluator activity/risk = %+v/%+v, want zero", scores.Activity, scores.Risk)
	}
	if scores.Budget.Value == 0 || scores.Pressure.Value == 0 {
		t.Fatalf("zero evaluator budget/pressure = %+v/%+v, want pure projections preserved", scores.Budget, scores.Pressure)
	}
	values := evaluator.ScoreValues(rates, PoolPartitionEWMAState{}, budget, pressure)
	if values.Usefulness != 0 || values.Waste != 0 || values.Activity != 0 || values.Risk != 0 {
		t.Fatalf("zero evaluator values = %+v, want zero evaluator-owned scores", values)
	}
	if values.Budget == 0 || values.Pressure == 0 {
		t.Fatalf("zero evaluator values = %+v, want budget/pressure projections preserved", values)
	}
}

func TestPoolPartitionScoreEvaluatorScoreValuesMatchesScoresValues(t *testing.T) {
	rates := PoolPartitionWindowRates{
		HitRatio:                        0.8,
		RetainRatio:                     0.7,
		AllocationRatio:                 0.2,
		DropRatio:                       0.1,
		GetsPerSecond:                   defaultPartitionHighGetsPerSecond,
		PutsPerSecond:                   defaultPartitionHighPutsPerSecond,
		LeaseOpsPerSecond:               defaultPartitionHighLeaseOpsPerSecond,
		PoolReturnFailureRatio:          0.1,
		PoolReturnAdmissionFailureRatio: 0.1,
		LeaseInvalidReleaseRatio:        0.1,
	}
	budget := PartitionBudgetSnapshot{MaxOwnedBytes: 100, CurrentOwnedBytes: 50}
	pressure := PartitionPressureSnapshot{Enabled: true, Level: PressureLevelHigh}
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{})

	scores := evaluator.Scores(rates, PoolPartitionEWMAState{}, budget, pressure)
	values := evaluator.ScoreValues(rates, PoolPartitionEWMAState{}, budget, pressure)

	if values.Usefulness != scores.Usefulness.Value ||
		values.Waste != scores.Waste.Value ||
		values.Budget != scores.Budget.Value ||
		values.Pressure != scores.Pressure.Value ||
		values.Activity != scores.Activity.Value ||
		values.Risk != scores.Risk.Value {
		t.Fatalf("ScoreValues() = %+v, Scores() = %+v", values, scores)
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
		LeaseOpsPerSecond:    defaultPartitionHighLeaseOpsPerSecond,
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
		partitionScoreEvaluatorScoresSink = evaluator.Scores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
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
		partitionScoreEvaluatorValuesSink = evaluator.ScoreValues(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	}
}

func BenchmarkPoolPartitionScoreEvaluatorScoreValuesCustomConfig(b *testing.B) {
	rates := PoolPartitionWindowRates{
		HitRatio:                        0.8,
		RetainRatio:                     0.7,
		AllocationRatio:                 0.2,
		DropRatio:                       0.1,
		GetsPerSecond:                   100_000,
		PutsPerSecond:                   80_000,
		LeaseOpsPerSecond:               120_000,
		PoolReturnFailureRatio:          0.1,
		PoolReturnAdmissionFailureRatio: 0.1,
		LeaseInvalidReleaseRatio:        0.1,
	}
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{
		ActivityConfig: PoolPartitionActivityScoreConfig{
			HighGetsPerSecond:     defaultPartitionHighGetsPerSecond,
			HighPutsPerSecond:     defaultPartitionHighPutsPerSecond,
			HighLeaseOpsPerSecond: defaultPartitionHighLeaseOpsPerSecond,
		},
		RiskConfig: PoolPartitionRiskScoreConfig{
			Weights:       PoolPartitionRiskScoreWeights{Misuse: 1},
			MisuseWeights: PoolPartitionMisuseRiskWeights{InvalidRelease: 1},
		},
	})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		partitionScoreEvaluatorValuesSink = evaluator.ScoreValues(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	}
}

func isFiniteUnit(value float64) bool {
	return value >= 0 && value <= 1 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
