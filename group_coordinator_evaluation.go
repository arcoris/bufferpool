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

// evaluateGroupCoordinatorCycle computes one non-committing group projection.
//
// Evaluation samples the group, builds the group window, computes aggregate and
// partition-level scores, and plans group-to-partition budget targets. It does
// not publish partition targets, commit coordinator previous-sample state, start
// partition schedulers, execute Pool trim, or inspect Pool shards/classes. The
// generation advance stays here because existing TickInto semantics allocate an
// attempt generation before publication or status selection is known.
func (g *PoolGroup) evaluateGroupCoordinatorCycle(dst *PoolGroupCoordinatorReport) groupCoordinatorCycleEvaluation {
	generation := g.generation.Advance()
	runtime := g.currentRuntimeSnapshot()
	now := clockNow(g.coordinator.clock)

	sample := dst.Sample
	g.sampleWithRuntimeAndGeneration(&sample, runtime, generation)

	previous := sample
	elapsed := time.Duration(0)
	if g.coordinator.hasPreviousSample {
		previous = g.coordinator.previousSample
		elapsed = clockElapsed(g.coordinator.previousSampleTime, now)
	}

	window := dst.Window
	window.Reset(previous, sample)
	rates := NewPoolGroupTimedWindowRates(window, elapsed)

	metrics := newPoolGroupMetrics(g.name, sample)
	budget := newGroupBudgetSnapshot(runtime.Policy.Budget, sample)
	pressure := newGroupPressureSnapshot(runtime.Policy.Pressure, sample)
	scoreEvaluator := NewPoolGroupScoreEvaluator(runtime.Policy.Score)
	scores := scoreEvaluator.ScoreValues(rates, budget, pressure)
	partitionScores := g.groupPartitionScores(window, elapsed, scoreEvaluator)

	partitionBudgetAllocation := g.coordinatorPartitionBudgetReport(generation, runtime, window, partitionScores)

	return groupCoordinatorCycleEvaluation{
		generation:                generation,
		runtime:                   runtime,
		now:                       now,
		sample:                    sample,
		previous:                  previous,
		elapsed:                   elapsed,
		window:                    window,
		rates:                     rates,
		metrics:                   metrics,
		budget:                    budget,
		pressure:                  pressure,
		scores:                    scores,
		partitionScores:           partitionScores,
		partitionBudgetAllocation: partitionBudgetAllocation,
		partitionBudgetTargets:    partitionBudgetAllocation.Targets,
	}
}
