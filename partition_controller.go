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

	// Budget is the partition budget projection from Sample.
	Budget PartitionBudgetSnapshot

	// Pressure is the pressure interpretation from Sample.
	Pressure PartitionPressureSnapshot

	// TrimPlan is the planning-only trim decision for this tick.
	TrimPlan PartitionTrimPlan

	// TrimResult remains zero until physical trim execution is implemented.
	TrimResult PartitionTrimResult
}

// Tick performs one explicit partition controller observation/planning pass.
//
// The current implementation samples state and computes metrics, budget usage,
// pressure, and a non-mutating trim plan. It does not publish Pool runtime
// snapshots and does not execute physical trim. Controller.Enabled controls
// future automatic scheduling; manual Tick remains available when the partition
// is active.
func (p *PoolPartition) Tick() (PartitionControllerReport, error) {
	p.mustBeInitialized()
	if !p.lifecycle.AllowsWork() {
		return PartitionControllerReport{}, newError(ErrClosed, errPartitionClosed)
	}
	generation := p.generation.Advance()
	runtime := p.currentRuntimeSnapshot()
	var sample PoolPartitionSample
	p.sampleWithRuntimeAndGeneration(&sample, runtime, generation, true)
	metrics := newPoolPartitionMetrics(p.name, sample)
	budget := newPartitionBudgetSnapshot(runtime.Policy.Budget, sample)
	pressure := newPartitionPressureSnapshot(runtime.Policy.Pressure, sample)
	trimPlan := newPartitionTrimPlan(runtime.Policy.Trim, pressure, sample)
	return PartitionControllerReport{
		Generation:       generation,
		PolicyGeneration: sample.PolicyGeneration,
		Lifecycle:        p.lifecycle.Load(),
		Sample:           sample,
		Metrics:          metrics,
		Budget:           budget,
		Pressure:         pressure,
		TrimPlan:         trimPlan,
	}, nil
}
