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

// Tick performs one explicit group coordinator observation pass.
//
// The current implementation samples group state and computes metrics, budget
// usage, pressure, and scalar score values. It does not publish partition
// runtime policies, redistribute budgets, execute physical trim, or start
// background work.
func (g *PoolGroup) Tick() (PoolGroupCoordinatorReport, error) {
	g.mustBeInitialized()
	var report PoolGroupCoordinatorReport
	err := g.TickInto(&report)
	return report, err
}

// TickInto writes one explicit group coordinator observation pass into dst.
//
// TickInto reuses dst.Sample.Partitions capacity. A nil dst is a no-op after
// receiver and lifecycle validation. This is a manual foreground call, not a
// scheduler tick from a background goroutine.
func (g *PoolGroup) TickInto(dst *PoolGroupCoordinatorReport) error {
	g.mustBeInitialized()
	if !g.lifecycle.AllowsWork() {
		return newError(ErrClosed, errGroupClosed)
	}
	if dst == nil {
		return nil
	}
	generation := g.generation.Advance()
	runtime := g.currentRuntimeSnapshot()
	sample := dst.Sample
	g.sampleWithRuntimeAndGeneration(&sample, runtime, generation)
	metrics := newPoolGroupMetrics(g.name, sample)
	budget := newGroupBudgetSnapshot(runtime.Policy.Budget, sample)
	pressure := newGroupPressureSnapshot(runtime.Policy.Pressure, sample)
	scores := g.scoreEvaluator.ScoreValues(PoolGroupWindowRates{}, budget, pressure)
	*dst = PoolGroupCoordinatorReport{
		Generation:       generation,
		PolicyGeneration: sample.PolicyGeneration,
		Lifecycle:        g.lifecycle.Load(),
		Sample:           sample,
		Metrics:          metrics,
		Budget:           budget,
		Pressure:         pressure,
		Scores:           scores,
	}
	return nil
}

// newGroupBudgetSnapshot projects group aggregate sample usage against group limits.
func newGroupBudgetSnapshot(policy PartitionBudgetPolicy, sample PoolGroupSample) PartitionBudgetSnapshot {
	return newPartitionBudgetSnapshot(policy, sample.Aggregate)
}

// newGroupPressureSnapshot projects group aggregate sample usage against pressure policy.
func newGroupPressureSnapshot(policy PartitionPressurePolicy, sample PoolGroupSample) PartitionPressureSnapshot {
	return newPartitionPressureSnapshot(policy, sample.Aggregate)
}
