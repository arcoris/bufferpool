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

// TestScoreValuesAreFiniteAcrossZeroInputs pins the scalar no-diagnostics path
// on empty inputs. ScoreValues is intended for allocation-conscious controller
// paths, so it must preserve the same finite [0,1] domain as Scores without
// requiring component allocation.
func TestScoreValuesAreFiniteAcrossZeroInputs(t *testing.T) {
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{})
	values := evaluator.ScoreValues(PoolPartitionWindowRates{}, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})

	assertPoolPartitionScoreValuesFinite(t, values)
}

// TestScoreValuesAreFiniteAcrossExtremeInputs verifies that pathological rate
// and budget inputs cannot leak NaN, infinity, or out-of-range values through
// either scalar values or explainable score reports.
func TestScoreValuesAreFiniteAcrossExtremeInputs(t *testing.T) {
	rates := PoolPartitionWindowRates{
		HitRatio:                        math.Inf(1),
		RetainRatio:                     math.NaN(),
		AllocationRatio:                 math.Inf(-1),
		DropRatio:                       math.Inf(1),
		GetsPerSecond:                   math.Inf(1),
		PutsPerSecond:                   math.NaN(),
		AllocationsPerSecond:            math.Inf(1),
		LeaseOpsPerSecond:               math.Inf(1),
		PoolReturnFailureRatio:          math.Inf(1),
		PoolReturnAdmissionFailureRatio: math.NaN(),
		LeaseOwnershipViolationRatio:    math.Inf(1),
		LeaseInvalidReleaseRatio:        math.NaN(),
		LeaseDoubleReleaseRatio:         math.Inf(1),
	}
	ewma := PoolPartitionEWMAState{
		Initialized:          true,
		HitRatio:             math.Inf(1),
		MissRatio:            math.NaN(),
		AllocationRatio:      math.Inf(1),
		RetainRatio:          math.NaN(),
		GetsPerSecond:        math.Inf(1),
		PutsPerSecond:        math.NaN(),
		AllocationsPerSecond: math.Inf(1),
		LeaseOpsPerSecond:    math.Inf(1),
	}
	budget := PartitionBudgetSnapshot{
		MaxOwnedBytes:     1,
		CurrentOwnedBytes: ^uint64(0),
		OwnedOverBudget:   true,
	}
	pressure := PartitionPressureSnapshot{Enabled: true, Level: PressureLevelCritical}
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{})

	assertPoolPartitionScoreValuesFinite(t, evaluator.ScoreValues(rates, ewma, budget, pressure))
	assertPoolPartitionScoresFinite(t, evaluator.Scores(rates, ewma, budget, pressure))
}

// TestScoreComponentsAreDefensivelyCopied verifies public diagnostic copy
// methods return owned component storage. Callers can retain and mutate returned
// diagnostics without modifying the source score value used by reports.
func TestScoreComponentsAreDefensivelyCopied(t *testing.T) {
	classScore := PoolClassScore{
		Value: 0.5,
		Components: []PoolClassScoreComponent{
			{Name: "activity", Value: 0.5, Weight: 1},
		},
	}
	classComponents := classScore.ComponentsCopy()
	classComponents[0].Name = "mutated"
	if classScore.Components[0].Name == "mutated" {
		t.Fatalf("PoolClassScore.ComponentsCopy shared source storage")
	}

	poolScore := PoolBudgetScore{
		PoolName: "primary",
		Value:    0.5,
		Components: []PoolBudgetScoreComponent{
			{Name: "class:0", Value: 0.5, Weight: 1},
		},
		ClassScores: []PoolClassScoreEntry{
			{ClassID: ClassID(0), Score: classScore},
		},
	}
	poolComponents := poolScore.ComponentsCopy()
	classEntries := poolScore.ClassScoresCopy()
	poolComponents[0].Name = "mutated"
	classEntries[0].Score.Components[0].Name = "mutated"
	if poolScore.Components[0].Name == "mutated" || poolScore.ClassScores[0].Score.Components[0].Name == "mutated" {
		t.Fatalf("PoolBudgetScore copy helpers shared source storage")
	}
}

// TestScoreEvaluatorZeroValueBehavior documents the zero-value evaluator:
// evaluator-owned usefulness, waste, activity, and risk are disabled, while
// pure budget and pressure projections remain available because they do not
// depend on configured scorers.
func TestScoreEvaluatorZeroValueBehavior(t *testing.T) {
	var evaluator PoolPartitionScoreEvaluator
	rates := PoolPartitionWindowRates{
		HitRatio:                 1,
		RetainRatio:              1,
		DropRatio:                1,
		GetsPerSecond:            defaultPartitionHighGetsPerSecond,
		LeaseInvalidReleaseRatio: 1,
	}
	budget := PartitionBudgetSnapshot{MaxOwnedBytes: 100, CurrentOwnedBytes: 150, OwnedOverBudget: true}
	pressure := PartitionPressureSnapshot{Enabled: true, Level: PressureLevelCritical}
	scores := evaluator.Scores(rates, PoolPartitionEWMAState{}, budget, pressure)
	values := evaluator.ScoreValues(rates, PoolPartitionEWMAState{}, budget, pressure)

	if scores.Usefulness.Value != 0 || scores.Waste.Value != 0 || scores.Activity.Value != 0 || scores.Risk.Value != 0 {
		t.Fatalf("zero evaluator scores = %+v, want evaluator-owned scores disabled", scores)
	}
	if values.Usefulness != 0 || values.Waste != 0 || values.Activity != 0 || values.Risk != 0 {
		t.Fatalf("zero evaluator values = %+v, want evaluator-owned values disabled", values)
	}
	if scores.Budget.Value == 0 || scores.Pressure.Value == 0 || values.Budget == 0 || values.Pressure == 0 {
		t.Fatalf("zero evaluator pure projections missing: scores=%+v values=%+v", scores, values)
	}
	assertPoolPartitionScoreValuesFinite(t, values)
	assertPoolPartitionScoresFinite(t, scores)
}

// TestDefaultScoreEvaluatorUsesConservativeDefaults verifies the default
// evaluator produces bounded non-degenerate projections at documented threshold
// inputs without requiring callers to configure public tuning knobs.
func TestDefaultScoreEvaluatorUsesConservativeDefaults(t *testing.T) {
	rates := PoolPartitionWindowRates{
		HitRatio:                 1,
		RetainRatio:              1,
		AllocationRatio:          0,
		DropRatio:                0,
		GetsPerSecond:            defaultPartitionHighGetsPerSecond,
		PutsPerSecond:            defaultPartitionHighPutsPerSecond,
		LeaseOpsPerSecond:        defaultPartitionHighLeaseOpsPerSecond,
		LeaseInvalidReleaseRatio: 0.5,
	}
	budget := PartitionBudgetSnapshot{MaxOwnedBytes: 100, CurrentOwnedBytes: 50}
	pressure := PartitionPressureSnapshot{Enabled: true, Level: PressureLevelMedium}
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{})
	scores := evaluator.Scores(rates, PoolPartitionEWMAState{}, budget, pressure)

	if scores.Usefulness.Value == 0 || scores.Activity.Value == 0 || scores.Budget.Value == 0 || scores.Pressure.Value == 0 {
		t.Fatalf("default evaluator scores = %+v, want conservative non-zero defaults for active signals", scores)
	}
	assertPoolPartitionScoresFinite(t, scores)
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

// assertPoolPartitionScoreValuesFinite checks every scalar score emitted by the
// allocation-conscious ScoreValues path. It is deliberately separate from
// component checks so the test protects the no-diagnostic path from accidental
// allocation-oriented rewrites.
func assertPoolPartitionScoreValuesFinite(t *testing.T, values PoolPartitionScoreValues) {
	t.Helper()
	for name, value := range map[string]float64{
		"usefulness": values.Usefulness,
		"waste":      values.Waste,
		"budget":     values.Budget,
		"pressure":   values.Pressure,
		"activity":   values.Activity,
		"risk":       values.Risk,
	} {
		if !isFiniteUnit(value) {
			t.Fatalf("%s score value = %v, want finite [0,1]", name, value)
		}
	}
}

// assertPoolPartitionScoresFinite checks the explainable Scores path,
// including component values and weights. Components are the public diagnostic
// contract, so this helper keeps clamping and finite protection explicit.
func assertPoolPartitionScoresFinite(t *testing.T, scores PoolPartitionScores) {
	t.Helper()
	if !isFiniteUnit(scores.Usefulness.Value) ||
		!isFiniteUnit(scores.Waste.Value) ||
		!isFiniteUnit(scores.Budget.Value) ||
		!isFiniteUnit(scores.Pressure.Value) ||
		!isFiniteUnit(scores.Activity.Value) ||
		!isFiniteUnit(scores.Risk.Value) {
		t.Fatalf("scores = %+v, want finite [0,1] values", scores)
	}
	assertPoolPartitionScoreComponentsFinite(t, scores.Usefulness.Components)
	assertPoolPartitionScoreComponentsFinite(t, scores.Waste.Components)
}

// assertPoolPartitionScoreComponentsFinite verifies component diagnostics stay
// finite, clamped, and non-negative without depending on a specific formula.
func assertPoolPartitionScoreComponentsFinite(t *testing.T, components []PoolPartitionScoreComponent) {
	t.Helper()
	for _, component := range components {
		if !isFiniteUnit(component.Value) {
			t.Fatalf("component value = %v in %+v, want finite [0,1]", component.Value, component)
		}
		if component.Weight < 0 || math.IsNaN(component.Weight) || math.IsInf(component.Weight, 0) {
			t.Fatalf("component weight = %v in %+v, want finite non-negative", component.Weight, component)
		}
	}
}
