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
	"sort"
	"testing"
	"time"
)

func TestPartitionControllerBudgetUsesTypedClassScores(t *testing.T) {
	t.Parallel()

	partition := testPartitionControllerScorePartition(t, "primary")
	window := testPartitionControllerScoreWindow(map[string][]classCountersDelta{
		"primary": {
			{Gets: 64, Hits: 64, Puts: 64, Retains: 64},
			{Gets: 2, Misses: 2, Allocations: 2, Puts: 2, Drops: 2},
		},
	})

	report := partition.controllerPoolBudgetReport(Generation(300), partition.currentRuntimeSnapshot(), window, time.Second, controllerUpdatedEWMAForTest(window, 128))
	if len(report.ClassReports) != 1 || len(report.ClassReports[0].Targets) != 2 {
		t.Fatalf("class reports = %+v, want one two-class report", report.ClassReports)
	}
	targets := report.ClassReports[0].Targets
	if targets[0].TargetBytes <= targets[1].TargetBytes {
		t.Fatalf("class targets = %+v, want high-scored class to receive more budget", targets)
	}
	if report.ClassReports[0].ClassScores[0].Score <= report.ClassReports[0].ClassScores[1].Score {
		t.Fatalf("class score diagnostics = %+v, want first class higher", report.ClassReports[0].ClassScores)
	}
}

func TestPartitionControllerBudgetUsesTypedPoolScores(t *testing.T) {
	t.Parallel()

	partition := testPartitionControllerScorePartition(t, "primary", "secondary")
	window := testPartitionControllerScoreWindow(map[string][]classCountersDelta{
		"primary": {
			{Gets: 80, Hits: 80, Puts: 80, Retains: 80},
			{Gets: 40, Hits: 40, Puts: 40, Retains: 40},
		},
		"secondary": {
			{Gets: 2, Misses: 2, Allocations: 2, Puts: 2, Drops: 2},
			{Gets: 1, Misses: 1, Allocations: 1, Puts: 1, Drops: 1},
		},
	})

	report := partition.controllerPoolBudgetReport(Generation(301), partition.currentRuntimeSnapshot(), window, time.Second, controllerUpdatedEWMAForTest(window, 128))
	primary := testPoolBudgetTargetByName(t, report.Targets, "primary")
	secondary := testPoolBudgetTargetByName(t, report.Targets, "secondary")
	if primary.RetainedBytes <= secondary.RetainedBytes {
		t.Fatalf("pool targets = %+v, want primary score to receive larger target", report.Targets)
	}
	if testPoolBudgetScoreReportByName(t, report.PoolScores, "primary").Score <= testPoolBudgetScoreReportByName(t, report.PoolScores, "secondary").Score {
		t.Fatalf("pool score diagnostics = %+v, want primary score greater", report.PoolScores)
	}
}

// TestAdaptiveScoringBudgetTargetsFollowScoreOrdering verifies the final
// integration point between typed scores and budget allocation. The allocator
// receives only Score.Value, and the higher-scored Pool must receive the larger
// adaptive target.
func TestAdaptiveScoringBudgetTargetsFollowScoreOrdering(t *testing.T) {
	t.Parallel()

	partition := testPartitionControllerScorePartition(t, "primary", "secondary")
	window := testPartitionControllerScoreWindow(map[string][]classCountersDelta{
		"primary": {
			{Gets: 96, Hits: 96, Puts: 96, Retains: 96},
			{Gets: 64, Hits: 64, Puts: 64, Retains: 64},
		},
		"secondary": {
			{Gets: 1, Misses: 1, Allocations: 1, Puts: 1, Drops: 1},
			{Gets: 1, Misses: 1, Allocations: 1, Puts: 1, Drops: 1},
		},
	})

	report := partition.controllerPoolBudgetReport(Generation(401), partition.currentRuntimeSnapshot(), window, time.Second, controllerUpdatedEWMAForTest(window, 128))
	primaryTarget := testPoolBudgetTargetByName(t, report.Targets, "primary")
	secondaryTarget := testPoolBudgetTargetByName(t, report.Targets, "secondary")
	primaryScore := testPoolBudgetScoreReportByName(t, report.PoolScores, "primary")
	secondaryScore := testPoolBudgetScoreReportByName(t, report.PoolScores, "secondary")
	if primaryScore.Score <= secondaryScore.Score || primaryTarget.RetainedBytes <= secondaryTarget.RetainedBytes {
		t.Fatalf("scores/targets = primary %.6f/%s secondary %.6f/%s, want higher score to receive larger budget",
			primaryScore.Score, primaryTarget.RetainedBytes, secondaryScore.Score, secondaryTarget.RetainedBytes)
	}
}

func TestPartitionControllerScoresUseSameUpdatedEWMA(t *testing.T) {
	t.Parallel()

	window := testPartitionControllerScoreWindow(map[string][]classCountersDelta{
		"primary": {
			{Gets: 1, Hits: 1},
		},
	})
	oldEWMA := map[poolClassKey]PoolClassEWMAState{
		{PoolName: "primary", ClassID: ClassID(0)}: {Initialized: true, Activity: 1},
	}
	updatedEWMA := map[poolClassKey]PoolClassEWMAState{
		{PoolName: "primary", ClassID: ClassID(0)}: {Initialized: true, Activity: 256},
	}

	oldClassScores := controllerClassScoreMap(window, time.Second, oldEWMA)
	updatedClassScores := controllerClassScoreMap(window, time.Second, updatedEWMA)
	oldPoolScore := poolWindowScore(window.Pools[0], oldClassScores)
	updatedPoolScore := poolWindowScore(window.Pools[0], updatedClassScores)

	if updatedClassScores[poolClassKey{PoolName: "primary", ClassID: ClassID(0)}].Value <= oldClassScores[poolClassKey{PoolName: "primary", ClassID: ClassID(0)}].Value {
		t.Fatalf("updated class score did not reflect updated EWMA: old=%+v updated=%+v", oldClassScores, updatedClassScores)
	}
	if updatedPoolScore.Value <= oldPoolScore.Value {
		t.Fatalf("updated pool score = %v, want greater than old %v", updatedPoolScore.Value, oldPoolScore.Value)
	}
	if updatedPoolScore.ClassScores[0].Score.Value != updatedClassScores[poolClassKey{PoolName: "primary", ClassID: ClassID(0)}].Value {
		t.Fatalf("pool score class entry = %+v, want updated class score %+v", updatedPoolScore.ClassScores[0], updatedClassScores)
	}
}

// TestAdaptiveScoringUsesOneUpdatedEWMASnapshot verifies the final phase-order
// invariant: class scores and Pool aggregation are both derived from one
// tick-local updated EWMA snapshot, not from mixed old/new state.
func TestAdaptiveScoringUsesOneUpdatedEWMASnapshot(t *testing.T) {
	t.Parallel()

	window := testPartitionControllerScoreWindow(map[string][]classCountersDelta{
		"primary": {
			{Gets: 2, Hits: 2},
		},
	})
	updatedEWMA := map[poolClassKey]PoolClassEWMAState{
		{PoolName: "primary", ClassID: ClassID(0)}: {Initialized: true, Activity: 512},
	}
	classScores := controllerClassScoreMap(window, time.Second, updatedEWMA)
	poolScore := poolWindowScore(window.Pools[0], classScores)
	key := poolClassKey{PoolName: "primary", ClassID: ClassID(0)}

	if poolScore.ClassScores[0].Score.Value != classScores[key].Value {
		t.Fatalf("pool score class entry = %+v, class score = %+v, want same updated EWMA score", poolScore.ClassScores[0], classScores[key])
	}
}

func TestPartitionControllerDoesNotUseRawSumOnlyPoolScore(t *testing.T) {
	t.Parallel()

	window := PoolPartitionPoolWindow{
		Name: "primary",
		Classes: []PoolPartitionClassWindow{
			{ClassID: ClassID(0)},
			{ClassID: ClassID(1)},
		},
	}
	classScores := map[poolClassKey]PoolClassScore{
		{PoolName: "primary", ClassID: ClassID(0)}: testPoolClassScoreEntry(ClassID(0), 0.2).Score,
		{PoolName: "primary", ClassID: ClassID(1)}: testPoolClassScoreEntry(ClassID(1), 0.4).Score,
	}

	score := poolWindowScore(window, classScores)
	if score.Value == 0.6 {
		t.Fatalf("pool score = %v, unexpectedly matched raw sum", score.Value)
	}
	if math.Abs(score.Value-0.3) > 1e-12 {
		t.Fatalf("pool score = %v, want equal-weight typed aggregate 0.3", score.Value)
	}
}

func TestPartitionControllerReportsPoolScoreDiagnostics(t *testing.T) {
	t.Parallel()

	partition := testPartitionControllerScorePartition(t, "primary")
	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))

	if len(report.BudgetPublication.PoolScores) != 1 {
		t.Fatalf("pool score diagnostics = %+v, want one Pool score report", report.BudgetPublication.PoolScores)
	}
	score := report.BudgetPublication.PoolScores[0]
	if score.PoolName != "primary" || score.Score < 0 || score.Score > 1 || len(score.Components) == 0 || len(score.ClassScores) == 0 {
		t.Fatalf("pool score diagnostic = %+v, want compact normalized diagnostics", score)
	}
}

func TestPartitionControllerReportIncludesPoolScoreDiagnostics(t *testing.T) {
	t.Parallel()

	partition := testPartitionControllerScorePartition(t, "primary")
	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))

	if len(report.PoolScores) == 0 {
		t.Fatalf("PoolScores = %+v, want controller-level pool score diagnostics", report.PoolScores)
	}
	if !reflect.DeepEqual(report.PoolScores, report.BudgetPublication.PoolScores) {
		t.Fatalf("PoolScores = %+v, want budget publication scores %+v", report.PoolScores, report.BudgetPublication.PoolScores)
	}
	report.BudgetPublication.PoolScores[0].Components[0].Name = "mutated"
	if report.PoolScores[0].Components[0].Name == "mutated" {
		t.Fatalf("controller PoolScores share component storage with BudgetPublication")
	}
}

func TestPartitionControllerReportsClassScoreDiagnostics(t *testing.T) {
	t.Parallel()

	partition := testPartitionControllerScorePartition(t, "primary")
	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))

	if len(report.BudgetPublication.ClassReports) != 1 {
		t.Fatalf("class reports = %+v, want one report", report.BudgetPublication.ClassReports)
	}
	classReport := report.BudgetPublication.ClassReports[0]
	if len(classReport.ClassScores) == 0 {
		t.Fatalf("class score diagnostics missing from %+v", classReport)
	}
	for _, score := range classReport.ClassScores {
		if score.PoolName != "primary" || score.Score < 0 || score.Score > 1 || len(score.Components) == 0 {
			t.Fatalf("class score diagnostic = %+v, want normalized component diagnostics", score)
		}
	}
}

func TestPartitionControllerReportIncludesClassScoreDiagnostics(t *testing.T) {
	t.Parallel()

	partition := testPartitionControllerScorePartition(t, "primary")
	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))

	if len(report.ClassScores) == 0 {
		t.Fatalf("ClassScores = %+v, want controller-level class score diagnostics", report.ClassScores)
	}
	if len(report.BudgetPublication.ClassReports) == 0 || len(report.BudgetPublication.ClassReports[0].ClassScores) != len(report.ClassScores) {
		t.Fatalf("BudgetPublication class reports = %+v, controller ClassScores = %+v", report.BudgetPublication.ClassReports, report.ClassScores)
	}
	report.BudgetPublication.ClassReports[0].ClassScores[0].Components[0].Name = "mutated"
	if report.ClassScores[0].Components[0].Name == "mutated" {
		t.Fatalf("controller ClassScores share component storage with BudgetPublication")
	}
}

func TestPartitionControllerScoreDiagnosticsMatchBudgetInputs(t *testing.T) {
	t.Parallel()

	partition := testPartitionControllerScorePartition(t, "primary")
	window := testPartitionControllerScoreWindow(map[string][]classCountersDelta{
		"primary": {
			{Gets: 32, Hits: 32, Puts: 32, Retains: 32},
			{Gets: 4, Misses: 4, Allocations: 4, Puts: 4, Drops: 4},
		},
	})
	updatedEWMA := controllerUpdatedEWMAForTest(window, 64)
	classScores := controllerClassScoreMap(window, time.Second, updatedEWMA)
	poolScore := poolWindowScore(window.Pools[0], classScores)

	report := partition.controllerPoolBudgetReport(Generation(302), partition.currentRuntimeSnapshot(), window, time.Second, updatedEWMA)
	gotPoolScore := testPoolBudgetScoreReportByName(t, report.PoolScores, "primary")
	if gotPoolScore.Score != poolScore.Value {
		t.Fatalf("pool score report = %v, want budget input %v", gotPoolScore.Score, poolScore.Value)
	}
	if len(report.ClassReports) != 1 || len(report.ClassReports[0].ClassScores) != len(window.Pools[0].Classes) {
		t.Fatalf("class score reports = %+v, want one score per class", report.ClassReports)
	}
	for _, classWindow := range window.Pools[0].Classes {
		key := poolClassKey{PoolName: "primary", ClassID: classWindow.ClassID}
		got := testPoolClassScoreReportByID(t, report.ClassReports[0].ClassScores, classWindow.ClassID)
		if got.Score != classScores[key].Value {
			t.Fatalf("class %s score report = %v, want budget input %v", classWindow.ClassID, got.Score, classScores[key].Value)
		}
	}
}

func TestBudgetPublicationScoreDiagnosticsMatchAllocationInputs(t *testing.T) {
	t.Parallel()

	partition := testPartitionControllerScorePartition(t, "primary")
	window := testPartitionControllerScoreWindow(map[string][]classCountersDelta{
		"primary": {
			{Gets: 48, Hits: 48, Puts: 48, Retains: 48},
			{Gets: 3, Misses: 3, Allocations: 3, Puts: 3, Drops: 3},
		},
	})
	updatedEWMA := controllerUpdatedEWMAForTest(window, 64)
	classScores := controllerClassScoreMap(window, time.Second, updatedEWMA)
	poolScore := poolWindowScore(window.Pools[0], classScores)

	report := partition.controllerPoolBudgetReport(Generation(303), partition.currentRuntimeSnapshot(), window, time.Second, updatedEWMA)
	gotPoolScore := testPoolBudgetScoreReportByName(t, report.PoolScores, "primary")
	if gotPoolScore.Score != poolScore.Value {
		t.Fatalf("pool score diagnostic = %v, want allocation input %v", gotPoolScore.Score, poolScore.Value)
	}
	for _, classWindow := range window.Pools[0].Classes {
		key := poolClassKey{PoolName: "primary", ClassID: classWindow.ClassID}
		got := testPoolClassScoreReportByID(t, report.ClassReports[0].ClassScores, classWindow.ClassID)
		if got.Score != classScores[key].Value {
			t.Fatalf("class score diagnostic = %v, want allocation input %v", got.Score, classScores[key].Value)
		}
	}
}

func TestPartitionControllerMissingClassScoreUsesNeutralScore(t *testing.T) {
	t.Parallel()

	window := testPartitionControllerScoreWindow(map[string][]classCountersDelta{
		"primary": {
			{Gets: 32, Hits: 32},
			{Gets: 16, Hits: 16},
		},
	})

	poolScore := poolWindowScore(window.Pools[0], nil)
	if poolScore.Value != 0 {
		t.Fatalf("missing class scores produced pool score %v, want conservative zero", poolScore.Value)
	}
	for _, entry := range poolScore.ClassScores {
		if !entry.Score.IsZero() {
			t.Fatalf("missing class score entry = %+v, want zero score", entry)
		}
	}
}

func TestMissingScoreDiagnosticIsNeutral(t *testing.T) {
	t.Parallel()

	window := testPartitionControllerScoreWindow(map[string][]classCountersDelta{
		"primary": {
			{Gets: 32, Hits: 32},
		},
	})
	poolScore := poolWindowScore(window.Pools[0], nil)
	report := newPoolBudgetScoreReport(poolScore)
	if report.Score != 0 || len(report.ClassScores) != 1 {
		t.Fatalf("missing score report = %+v, want one neutral class score", report)
	}
	classScore := report.ClassScores[0]
	if len(classScore.Components) != 1 || classScore.Components[0].Name != poolClassScoreComponentNeutralMissing {
		t.Fatalf("missing class score components = %+v, want stable neutral reason", classScore.Components)
	}
}

func TestScoreReportComponentsAreCopied(t *testing.T) {
	t.Parallel()

	score := PoolClassScore{
		Value: 0.75,
		Components: []PoolClassScoreComponent{
			{Name: "activity", Value: 0.75, Weight: 1},
		},
	}
	report := newPoolClassScoreReport("primary", ClassID(0), score)
	score.Components[0].Name = "mutated"
	if report.Components[0].Name == "mutated" {
		t.Fatalf("score report components share storage with source score")
	}
}

func TestScoreDiagnosticsDoNotExposeMutableInternalSlices(t *testing.T) {
	t.Parallel()

	source := []PoolBudgetScoreReport{
		{
			PoolName: "primary",
			Score:    0.5,
			Components: []PoolBudgetScoreComponent{
				{Name: "class:0", Value: 0.5, Weight: 1},
			},
			ClassScores: []PoolClassScoreReport{
				{
					PoolName: "primary",
					ClassID:  ClassID(0),
					Score:    0.5,
					Components: []PoolClassScoreComponent{
						{Name: "activity", Value: 0.5, Weight: 1},
					},
				},
			},
		},
	}
	copied := copyPoolBudgetScoreReports(source)
	source[0].Components[0].Name = "mutated"
	source[0].ClassScores[0].Components[0].Name = "mutated"

	if copied[0].Components[0].Name == "mutated" || copied[0].ClassScores[0].Components[0].Name == "mutated" {
		t.Fatalf("copied score diagnostics share mutable component storage: %+v", copied)
	}
}

func TestScoreDiagnosticsAreFiniteAndClamped(t *testing.T) {
	t.Parallel()

	partition := testPartitionControllerScorePartition(t, "primary")
	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))

	assertPoolBudgetScoreReportsFiniteAndClamped(t, report.PoolScores)
	assertPoolClassScoreReportsFiniteAndClamped(t, report.ClassScores)
}

func TestScoreDiagnosticsWeightsAreNonNegative(t *testing.T) {
	t.Parallel()

	partition := testPartitionControllerScorePartition(t, "primary")
	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))

	for _, score := range report.PoolScores {
		assertRuntimeScoreComponentsNonNegativeWeights(t, score.Components)
		for _, classScore := range score.ClassScores {
			assertRuntimeScoreComponentsNonNegativeWeights(t, classScore.Components)
		}
	}
	for _, score := range report.ClassScores {
		assertRuntimeScoreComponentsNonNegativeWeights(t, score.Components)
	}
}

func TestPartitionControllerBudgetFeasibilityStillPropagates(t *testing.T) {
	t.Parallel()

	partition := testPartitionControllerScorePartition(t, "primary")
	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))

	if !report.BudgetPublication.Published {
		t.Fatalf("BudgetPublication = %+v, want published", report.BudgetPublication)
	}
	if !report.BudgetPublication.Allocation.Feasible {
		t.Fatalf("Pool allocation = %+v, want feasible", report.BudgetPublication.Allocation)
	}
	for _, classReport := range report.BudgetPublication.ClassReports {
		if !classReport.Allocation.Feasible || !classReport.Published {
			t.Fatalf("class budget report = %+v, want feasible and published", classReport)
		}
	}
}

// TestAdaptiveScoringDoesNotBreakBudgetFeasibilityReports keeps allocation
// diagnostics visible after typed score integration. Scores may explain
// priorities, but the feasibility report must still describe the hard budget
// relationship independently from score components.
func TestAdaptiveScoringDoesNotBreakBudgetFeasibilityReports(t *testing.T) {
	t.Parallel()

	partition := testPartitionControllerScorePartition(t, "primary", "secondary")
	runtime := partition.currentRuntimeSnapshot()
	policy := runtime.Policy
	policy.Budget.MaxRetainedBytes = SizeFromBytes(1)
	partition.publishRuntimeSnapshot(newPartitionRuntimeSnapshotWithPressure(Generation(400), policy, runtime.Pressure))
	window := testPartitionControllerScoreWindow(map[string][]classCountersDelta{
		"primary":   {{Gets: 64, Hits: 64}},
		"secondary": {{Gets: 64, Hits: 64}},
	})

	report := partition.controllerPoolBudgetReport(Generation(402), partition.currentRuntimeSnapshot(), window, time.Second, controllerUpdatedEWMAForTest(window, 64))
	if !report.Allocation.Feasible ||
		report.Allocation.Reason != budgetAllocationReasonFeasible ||
		report.Allocation.TargetCount != 2 ||
		report.Allocation.AssignedBytes > report.Allocation.RequestedBytes ||
		report.FailureReason != "" {
		t.Fatalf("BudgetPublication = %+v, want feasible diagnostic preserved independently from scores", report)
	}
	if len(report.PoolScores) == 0 {
		t.Fatalf("BudgetPublication = %+v, want score diagnostics alongside feasibility diagnostics", report)
	}
}

// TestAdaptiveScoringFailedPublicationDoesNotCommitControllerState mirrors the
// controller non-commit contract with score diagnostics present. A returned
// publication error is a failed cycle and cannot age dirty markers or commit
// EWMA/sample state.
func TestAdaptiveScoringFailedPublicationDoesNotCommitControllerState(t *testing.T) {
	t.Parallel()

	partition := testPartitionControllerScorePartition(t, "primary")
	pool, ok := partition.registry.pool("primary")
	if !ok {
		t.Fatal("primary pool not found")
	}
	requirePartitionNoError(t, pool.Close())

	beforeGeneration := partition.controller.generation.Load()
	beforeEWMA := partition.controller.ewma
	beforeClassEWMA := copyPoolClassEWMAStateMap(partition.controller.ewmaByPoolClass)
	beforeDirty := partition.controllerDirtyIndexes(nil)
	var report PartitionControllerReport
	requirePartitionErrorIs(t, partition.TickInto(&report), ErrClosed)

	if report.Status.Status != ControllerCycleStatusFailed ||
		report.BudgetPublication.Published ||
		report.BudgetPublication.FailureReason != controllerCycleReasonClosed ||
		len(report.PoolScores) == 0 {
		t.Fatalf("TickInto() = %+v, want failed diagnostic tick with score reports", report)
	}
	if partition.controller.generation.Load() != beforeGeneration ||
		partition.controller.ewma != beforeEWMA ||
		!reflect.DeepEqual(partition.controller.ewmaByPoolClass, beforeClassEWMA) ||
		!reflect.DeepEqual(partition.controllerDirtyIndexes(nil), beforeDirty) {
		t.Fatalf("controller state committed after unpublished scored tick")
	}
}

func TestPartitionControllerFailedBudgetPublicationDoesNotCommitScoresOrEWMA(t *testing.T) {
	t.Parallel()

	partition := testPartitionControllerScorePartition(t, "primary")
	pool, ok := partition.registry.pool("primary")
	if !ok {
		t.Fatal("primary pool not found")
	}
	requirePartitionNoError(t, pool.Close())

	beforeGeneration := partition.controller.generation.Load()
	beforeEWMA := partition.controller.ewma
	beforeClassEWMA := copyPoolClassEWMAStateMap(partition.controller.ewmaByPoolClass)
	beforeDirty := partition.controllerDirtyIndexes(nil)

	var report PartitionControllerReport
	requirePartitionErrorIs(t, partition.TickInto(&report), ErrClosed)
	if report.Status.Status != ControllerCycleStatusFailed {
		t.Fatalf("Status = %+v, want failed", report.Status)
	}
	if report.BudgetPublication.Published {
		t.Fatalf("BudgetPublication = %+v, want failed unpublished publication", report.BudgetPublication)
	}
	if len(report.BudgetPublication.PoolScores) == 0 {
		t.Fatalf("failed publication report missing score diagnostics: %+v", report.BudgetPublication)
	}
	if after := partition.controller.generation.Load(); after != beforeGeneration {
		t.Fatalf("controller generation = %s, want unchanged %s", after, beforeGeneration)
	}
	if partition.controller.ewma != beforeEWMA {
		t.Fatalf("controller EWMA changed after unpublished tick")
	}
	if !reflect.DeepEqual(partition.controller.ewmaByPoolClass, beforeClassEWMA) {
		t.Fatalf("class EWMA changed after unpublished tick")
	}
	if afterDirty := partition.controllerDirtyIndexes(nil); !reflect.DeepEqual(afterDirty, beforeDirty) {
		t.Fatalf("dirty indexes = %v, want unchanged %v", afterDirty, beforeDirty)
	}
}

func testPartitionControllerScorePartition(t *testing.T, poolNames ...string) *PoolPartition {
	t.Helper()

	config := PoolPartitionConfig{
		Name: "score-partition",
		Policy: PartitionPolicy{
			Budget: PartitionBudgetPolicy{
				MaxRetainedBytes: 2 * KiB,
			},
		},
		Pools: make([]PartitionPoolConfig, len(poolNames)),
	}
	for index, name := range poolNames {
		config.Pools[index] = PartitionPoolConfig{Name: name, Config: PoolConfig{Policy: poolTestSmallSingleShardPolicy()}}
	}

	partition, err := NewPoolPartition(config)
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })
	return partition
}

func testPartitionControllerScoreWindow(poolDeltas map[string][]classCountersDelta) PoolPartitionWindow {
	poolNames := make([]string, 0, len(poolDeltas))
	for name := range poolDeltas {
		poolNames = append(poolNames, name)
	}
	sort.Strings(poolNames)

	window := PoolPartitionWindow{
		Pools: make([]PoolPartitionPoolWindow, 0, len(poolNames)),
	}
	for _, poolName := range poolNames {
		deltas := poolDeltas[poolName]
		poolWindow := PoolPartitionPoolWindow{
			Name:    poolName,
			Classes: make([]PoolPartitionClassWindow, 0, len(deltas)),
		}
		for index, delta := range deltas {
			classID := ClassID(index)
			classBytes := uint64(512)
			if index > 0 {
				classBytes = KiB.Bytes()
			}
			sample := testPoolClassScoreSample(classID, classBytes, 0, 0)
			poolWindow.Classes = append(poolWindow.Classes, PoolPartitionClassWindow{
				Class:   sample.Class,
				ClassID: classID,
				Current: sample,
				Delta:   delta,
			})
		}
		window.Pools = append(window.Pools, poolWindow)
	}
	return window
}

func controllerUpdatedEWMAForTest(window PoolPartitionWindow, activity float64) map[poolClassKey]PoolClassEWMAState {
	ewma := make(map[poolClassKey]PoolClassEWMAState, partitionWindowClassCount(window))
	for _, poolWindow := range window.Pools {
		for _, classWindow := range poolWindow.Classes {
			ewma[poolClassKey{PoolName: poolWindow.Name, ClassID: classWindow.ClassID}] = PoolClassEWMAState{
				Initialized: true,
				Activity:    activity + float64(classWindow.Delta.Gets),
				DropRatio:   newPoolPartitionClassActivity(classWindow, time.Second).DropRatio,
			}
		}
	}
	return ewma
}

func testPoolBudgetTargetByName(t *testing.T, targets []PoolBudgetTarget, name string) PoolBudgetTarget {
	t.Helper()
	for _, target := range targets {
		if target.PoolName == name {
			return target
		}
	}
	t.Fatalf("target %q not found in %+v", name, targets)
	return PoolBudgetTarget{}
}

func testPoolBudgetScoreReportByName(t *testing.T, scores []PoolBudgetScoreReport, name string) PoolBudgetScoreReport {
	t.Helper()
	for _, score := range scores {
		if score.PoolName == name {
			return score
		}
	}
	t.Fatalf("pool score %q not found in %+v", name, scores)
	return PoolBudgetScoreReport{}
}

func testPoolClassScoreReportByID(t *testing.T, scores []PoolClassScoreReport, classID ClassID) PoolClassScoreReport {
	t.Helper()
	for _, score := range scores {
		if score.ClassID == classID {
			return score
		}
	}
	t.Fatalf("class score %s not found in %+v", classID, scores)
	return PoolClassScoreReport{}
}

func assertPoolBudgetScoreReportsFiniteAndClamped(t *testing.T, reports []PoolBudgetScoreReport) {
	t.Helper()
	for _, report := range reports {
		assertScoreValueFiniteAndClamped(t, report.Score)
		assertRuntimeScoreComponentsFiniteAndClamped(t, report.Components)
		assertPoolClassScoreReportsFiniteAndClamped(t, report.ClassScores)
	}
}

func assertPoolClassScoreReportsFiniteAndClamped(t *testing.T, reports []PoolClassScoreReport) {
	t.Helper()
	for _, report := range reports {
		assertScoreValueFiniteAndClamped(t, report.Score)
		assertRuntimeScoreComponentsFiniteAndClamped(t, report.Components)
	}
}

func assertRuntimeScoreComponentsFiniteAndClamped(t *testing.T, components []RuntimeScoreComponent) {
	t.Helper()
	for _, component := range components {
		assertScoreValueFiniteAndClamped(t, component.Value)
		if math.IsNaN(component.Weight) || math.IsInf(component.Weight, 0) {
			t.Fatalf("component weight is not finite: %+v", component)
		}
	}
}

func assertRuntimeScoreComponentsNonNegativeWeights(t *testing.T, components []RuntimeScoreComponent) {
	t.Helper()
	for _, component := range components {
		if component.Weight < 0 || math.IsNaN(component.Weight) || math.IsInf(component.Weight, 0) {
			t.Fatalf("component weight = %v, want finite non-negative component %+v", component.Weight, component)
		}
	}
}

func assertScoreValueFiniteAndClamped(t *testing.T, value float64) {
	t.Helper()
	if value < 0 || value > 1 || math.IsNaN(value) || math.IsInf(value, 0) {
		t.Fatalf("score value = %v, want finite [0,1]", value)
	}
}
