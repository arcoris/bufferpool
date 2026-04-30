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

// PartitionControllerReport describes one explicit controller tick.
type PartitionControllerReport struct {
	// Generation is the partition event generation for this tick.
	Generation Generation

	// PolicyGeneration is the partition runtime-policy generation used by the tick.
	PolicyGeneration Generation

	// Lifecycle is the observed partition lifecycle state.
	Lifecycle LifecycleState

	// Sample is the detailed aggregate sample captured by the tick.
	Sample PoolPartitionSample

	// Metrics is the lifetime-derived metrics projection from Sample.
	Metrics PoolPartitionMetrics

	// Window contains counter movement from the previous controller sample to
	// Sample.
	Window PoolPartitionWindow

	// Rates contains ratio and throughput projections derived from Window.
	Rates PoolPartitionWindowRates

	// EWMA contains the updated partition-level smoothed signals.
	EWMA PoolPartitionEWMAState

	// Scores contains partition-level score projections derived from window and
	// smoothed signals.
	Scores PoolPartitionScores

	// Budget is the partition budget projection from Sample.
	Budget PartitionBudgetSnapshot

	// Pressure is the pressure interpretation from Sample.
	Pressure PartitionPressureSnapshot

	// TrimPlan is the planning-only trim decision for this tick.
	TrimPlan PartitionTrimPlan

	// TrimResult remains zero until physical trim execution is implemented.
	TrimResult PartitionTrimResult

	// PoolBudgetTargets are the Pool/class budget targets applied by this tick.
	PoolBudgetTargets []PoolBudgetTarget
}

// Tick performs one explicit partition controller cycle.
//
// The current implementation samples active Pools, computes window deltas,
// updates partition-owned EWMA state, scores active Pool classes, publishes class
// budgets into owned Pools, and computes a non-mutating trim plan. It does not
// execute physical trim or start background scheduling. Controller.Enabled
// controls future automatic scheduling; manual Tick remains available when the
// partition is active.
//
// Tick returns a detailed diagnostic/report value and may allocate per-Pool
// sample/report storage. Controller sampling uses the active/dirty registry and
// periodic full scans, so ordinary ticks can narrow work after clean idle Pools
// age out while still revalidating inactive Pools deterministically.
func (p *PoolPartition) Tick() (PartitionControllerReport, error) {
	p.mustBeInitialized()
	var report PartitionControllerReport
	err := p.TickInto(&report)
	return report, err
}

// TickInto writes one explicit partition controller cycle into dst.
//
// TickInto has the same control semantics as Tick but reuses dst.Sample.Pools
// capacity. A nil dst is a no-op after receiver and lifecycle validation. Like
// Tick, this is a manual foreground call: it does not start background
// goroutines, execute physical trim, coordinate with PoolGroup, or redistribute
// group-level budgets.
func (p *PoolPartition) TickInto(dst *PartitionControllerReport) error {
	p.mustBeInitialized()
	if !p.lifecycle.AllowsWork() {
		return newError(ErrClosed, errPartitionClosed)
	}
	if dst == nil {
		return nil
	}
	p.controller.mu.Lock()
	defer p.controller.mu.Unlock()

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

	metrics := newPoolPartitionMetrics(p.name, sample)
	budget := newPartitionBudgetSnapshot(runtime.Policy.Budget, sample)
	pressure := newPartitionPressureSnapshot(runtime.Policy.Pressure, sample)
	scores := NewPoolPartitionScores(rates, ewma, budget, pressure)
	trimPlan := newPartitionTrimPlan(runtime.Policy.Trim, pressure, sample)
	dirtyIndexes := p.activeRegistry.dirtyIndexes(nil)
	activeDeltas := controllerPoolActivityDeltas(indexes, window, dirtyIndexes)
	if err := p.activeRegistry.observeControllerActivity(indexes, activeDeltas); err != nil {
		return err
	}
	poolBudgetTargets := p.controllerPoolBudgetTargets(generation, runtime, window, elapsed)
	if err := p.applyPoolBudgetTargets(poolBudgetTargets); err != nil {
		return err
	}
	p.markDirtyProcessed()
	p.controller.previousSample = copyPoolPartitionSampleInto(p.controller.previousSample, sample)
	p.controller.previousSampleTime = now
	p.controller.hasPreviousSample = true
	p.controller.ewma = ewma
	p.controller.cycles++
	_ = p.controller.generation.Advance()

	*dst = PartitionControllerReport{
		Generation:        generation,
		PolicyGeneration:  sample.PolicyGeneration,
		Lifecycle:         p.lifecycle.Load(),
		Sample:            sample,
		Metrics:           metrics,
		Window:            window,
		Rates:             rates,
		EWMA:              ewma,
		Scores:            scores,
		Budget:            budget,
		Pressure:          pressure,
		TrimPlan:          trimPlan,
		PoolBudgetTargets: poolBudgetTargets,
	}
	return nil
}
