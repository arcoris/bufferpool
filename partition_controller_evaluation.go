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

import "time"

// PoolPartitionControllerEvaluation is a pure controller projection.
//
// Evaluation turns two samples into window deltas, rates, updated EWMA, scores,
// and an advisory recommendation. It does not mutate PoolPartition state,
// publish runtime policy, execute trim, or start background work. Callers that
// apply decisions from this projection must do so through the owner-controlled
// publication and lifecycle gates.
type PoolPartitionControllerEvaluation struct {
	// Window contains the current counter movement.
	Window PoolPartitionWindow

	// Rates contains window-derived ratios and optional throughput.
	Rates PoolPartitionWindowRates

	// EWMA is the updated smoothing state.
	EWMA PoolPartitionEWMAState

	// Scores contains pure score projections.
	Scores PoolPartitionScores

	// Recommendation is advisory output derived from Scores, not an applied decision.
	Recommendation PoolPartitionRecommendation
}

// NewPoolPartitionControllerEvaluation returns a pure partition control evaluation.
func NewPoolPartitionControllerEvaluation(
	previous PoolPartitionSample,
	current PoolPartitionSample,
	elapsed time.Duration,
	ewma PoolPartitionEWMAState,
	ewmaConfig PoolPartitionEWMAConfig,
	budget PartitionBudgetSnapshot,
	pressure PartitionPressureSnapshot,
) PoolPartitionControllerEvaluation {
	window := NewPoolPartitionWindow(previous, current)
	rates := NewPoolPartitionTimedWindowRates(window, elapsed)
	updatedEWMA := ewma.WithUpdate(ewmaConfig, elapsed, rates)
	scores := NewPoolPartitionScores(rates, updatedEWMA, budget, pressure)

	return PoolPartitionControllerEvaluation{
		Window:         window,
		Rates:          rates,
		EWMA:           updatedEWMA,
		Scores:         scores,
		Recommendation: NewPoolPartitionRecommendation(scores),
	}
}

// evaluatePartitionControllerCycle computes one controller tick projection.
//
// The phase is non-committing: it samples, builds windows, updates tick-local
// EWMA values, scores, and plans budget publication, but it does not publish
// budgets, execute trim, mark dirty indexes processed, or commit controller
// state. The generation advance stays here because the previous implementation
// allocated the attempt generation before any publication branch was known.
func (p *PoolPartition) evaluatePartitionControllerCycle(dst *PartitionControllerReport) partitionControllerCycleEvaluation {
	generation := p.generation.Advance()
	runtime := p.currentRuntimeSnapshot()
	now := clockNow(p.controller.clock)

	indexes := p.controllerSampleIndexes(nil)
	sample := dst.Sample
	p.sampleIndexesWithRuntimeAndGeneration(&sample, runtime, generation, indexes, true)

	previous := sample
	elapsed := time.Duration(0)
	if p.controller.hasPreviousSample {
		previous = p.controller.previousSample
		elapsed = clockElapsed(p.controller.previousSampleTime, now)
	}

	window := dst.Window
	window.Reset(previous, sample)
	rates := NewPoolPartitionTimedWindowRates(window, elapsed)
	ewma := p.controller.ewma.WithUpdate(PoolPartitionEWMAConfig{}, elapsed, rates)
	nextClassEWMA := p.controllerUpdatedClassEWMA(window, elapsed)

	metrics := newPoolPartitionMetrics(p.name, sample)
	budget := newPartitionBudgetSnapshot(runtime.Policy.Budget, sample)
	pressure := newEffectivePartitionPressureSnapshot(runtime.Policy.Pressure, runtime.Pressure, sample)
	scores := NewPoolPartitionScores(rates, ewma, budget, pressure)
	trimPlan := newPartitionTrimPlan(runtime.Policy.Trim, pressure, sample)

	dirtyIndexes := p.activeRegistry.dirtyIndexes(nil)
	activeDeltas := controllerPoolActivityDeltas(indexes, window, dirtyIndexes)

	budgetPublication := p.controllerPoolBudgetReport(generation, runtime, window, elapsed, nextClassEWMA)

	return partitionControllerCycleEvaluation{
		generation:        generation,
		runtime:           runtime,
		now:               now,
		indexes:           indexes,
		sample:            sample,
		previous:          previous,
		elapsed:           elapsed,
		window:            window,
		rates:             rates,
		ewma:              ewma,
		nextClassEWMA:     nextClassEWMA,
		metrics:           metrics,
		budget:            budget,
		pressure:          pressure,
		scores:            scores,
		trimPlan:          trimPlan,
		dirtyIndexes:      dirtyIndexes,
		activeDeltas:      activeDeltas,
		budgetPublication: budgetPublication,
		poolBudgetTargets: budgetPublication.Targets,
	}
}
