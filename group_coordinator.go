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

// Tick performs one explicit foreground group coordinator cycle.
//
// Tick samples group state, computes a bounded window against the previous group
// coordinator sample, scores partitions, publishes retained-budget targets into
// owned PoolPartitions when the group has a retained budget, and returns a
// report. It does not execute physical trim, scan shards directly, compute class
// EWMA, call Pool.Get or Pool.Put, propagate pressure, or start background work.
func (g *PoolGroup) Tick() (PoolGroupCoordinatorReport, error) {
	g.mustBeInitialized()
	var report PoolGroupCoordinatorReport
	err := g.TickInto(&report)
	return report, err
}

// TickInto writes one explicit foreground group coordinator cycle into dst.
//
// TickInto is serialized with hard Close through runtimeMu because it advances
// the group generation and publishes partition budget targets through the group
// ownership boundary. It reuses dst.Sample.Partitions and report slice capacity.
// A nil dst is a no-op after receiver and lifecycle validation. This is a
// manual foreground call, not a scheduler tick from a background goroutine. dst
// must not be shared by concurrent callers without external synchronization.
//
// An unpublished budget cycle still reports the attempt Generation and commits
// coordinator observation state. PublishedGeneration remains NoGeneration unless
// every partition accepts the budget target batch. That separation lets callers
// distinguish "observed and reported" from "runtime budget state changed".
func (g *PoolGroup) TickInto(dst *PoolGroupCoordinatorReport) error {
	g.mustBeInitialized()
	g.runtimeMu.RLock()
	defer g.runtimeMu.RUnlock()

	if !g.lifecycle.AllowsWork() {
		return newError(ErrClosed, errGroupClosed)
	}
	if dst == nil {
		return nil
	}
	g.coordinator.mu.Lock()
	defer g.coordinator.mu.Unlock()

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
	partitionBudgetTargets := partitionBudgetAllocation.Targets
	skippedPartitions := dst.SkippedPartitions[:0]
	budgetPublication := PoolGroupBudgetPublicationReport{
		Generation: generation,
		Allocation: newBudgetAllocationDiagnostics(partitionBudgetAllocation.Allocation),
		Targets:    partitionBudgetTargets,
		Published:  false,
	}
	var err error
	if len(partitionBudgetTargets) > 0 && !partitionBudgetAllocation.Allocation.Feasible {
		budgetPublication.FailureReason = partitionBudgetAllocation.Allocation.Reason
	} else if len(partitionBudgetTargets) > 0 {
		skippedPartitions, err = g.publishPartitionBudgetTargets(partitionBudgetTargets, skippedPartitions)
		if err != nil {
			return err
		}
		budgetPublication.SkippedPartitions = skippedPartitions
		budgetPublication.Published = len(skippedPartitions) == 0
		if !budgetPublication.Published {
			budgetPublication.FailureReason = errGroupClosed
		}
	}
	publishedGeneration := NoGeneration
	if budgetPublication.Published {
		publishedGeneration = generation
	}
	g.coordinator.previousSample = copyPoolGroupSampleInto(g.coordinator.previousSample, sample)
	g.coordinator.previousSampleTime = now
	g.coordinator.hasPreviousSample = true
	coordinatorGeneration := g.coordinator.generation.Advance()
	*dst = PoolGroupCoordinatorReport{
		Generation:             generation,
		CoordinatorGeneration:  coordinatorGeneration,
		PolicyGeneration:       sample.PolicyGeneration,
		PublishedGeneration:    publishedGeneration,
		Lifecycle:              g.lifecycle.Load(),
		Sample:                 sample,
		Metrics:                metrics,
		Window:                 window,
		Rates:                  rates,
		Budget:                 budget,
		Pressure:               pressure,
		Scores:                 scores,
		PartitionScores:        partitionScores,
		PartitionBudgetTargets: partitionBudgetTargets,
		BudgetPublication:      budgetPublication,
		SkippedPartitions:      skippedPartitions,
	}
	return nil
}

// newGroupBudgetSnapshot projects group aggregate sample usage against group limits.
func newGroupBudgetSnapshot(policy PartitionBudgetPolicy, sample PoolGroupSample) PoolGroupBudgetSnapshot {
	return PoolGroupBudgetSnapshot(newPartitionBudgetSnapshot(policy, sample.Aggregate))
}

// newGroupPressureSnapshot projects group aggregate sample usage against pressure policy.
func newGroupPressureSnapshot(policy PartitionPressurePolicy, sample PoolGroupSample) PoolGroupPressureSnapshot {
	return PoolGroupPressureSnapshot(newPartitionPressureSnapshot(policy, sample.Aggregate))
}
