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

// group_coordinator_report.go owns report models and report assembly helpers
// for explicit PoolGroup coordinator cycles. The helpers may project samples
// into report diagnostics, but coordinator state mutation, child publication,
// and retained status publication remain in TickInto.

// PoolGroupCoordinatorReport describes one explicit foreground group coordinator
// cycle.
//
// The report records the observation, window projection, partition scores, and
// retained-budget targets published by one manual TickInto call. It still does
// not execute trim, scan shards directly, compute class EWMA, propagate
// pressure, or start a controller loop.
type PoolGroupCoordinatorReport struct {
	// Status is the lightweight retained coordinator-cycle status published for
	// this manual TickInto attempt. Full samples, scores, and budget diagnostics
	// remain in the report fields below and are not retained by ControllerStatus.
	Status PoolGroupControllerStatus

	// Generation is the group event generation for this tick attempt. It means
	// group state was sampled and a report was produced; it does not by itself
	// mean budget targets were published.
	Generation Generation

	// CoordinatorGeneration is the group coordinator generation committed after
	// observation state is saved. It may advance for an unpublished budget tick
	// because the observation window was still accepted.
	CoordinatorGeneration Generation

	// PolicyGeneration is the group runtime-policy generation used by the tick.
	PolicyGeneration Generation

	// PublishedGeneration is the budget target generation accepted by every
	// partition. It is NoGeneration when allocation is infeasible or a child
	// partition skipped publication.
	PublishedGeneration Generation

	// Lifecycle is the observed group lifecycle state.
	Lifecycle LifecycleState

	// Sample is the detailed aggregate group sample captured by the tick.
	Sample PoolGroupSample

	// Metrics is the lifetime-derived metrics projection from Sample.
	Metrics PoolGroupMetrics

	// Window contains aggregate movement from the previous coordinator sample to
	// Sample.
	Window PoolGroupWindow

	// Rates contains aggregate ratio and throughput projections derived from Window.
	Rates PoolGroupWindowRates

	// Budget is the group budget projection from Sample.
	Budget PoolGroupBudgetSnapshot

	// Pressure is the group pressure interpretation from Sample.
	Pressure PoolGroupPressureSnapshot

	// Scores contains aggregate scalar score values for the group window.
	Scores PoolGroupScoreValues

	// PartitionScores contains one score projection per sampled partition.
	PartitionScores []PoolGroupPartitionScore

	// PartitionBudgetTargets are retained-budget targets considered by this
	// cycle. BudgetPublication.Published says whether every target was accepted.
	PartitionBudgetTargets []PartitionBudgetTarget

	// BudgetPublication reports allocation feasibility and publication status for
	// group-to-partition budget propagation.
	BudgetPublication PoolGroupBudgetPublicationReport

	// SkippedPartitions records partitions that did not accept budget publication.
	SkippedPartitions []PoolGroupSkippedPartition
}

// PoolGroupPartitionScore describes one partition score used for redistribution.
type PoolGroupPartitionScore struct {
	// PartitionName is the group-local partition name.
	PartitionName string

	// Scores contains scalar partition score values computed from the group window.
	Scores PoolPartitionScoreValues

	// Score is the positive allocation weight derived from Scores.
	Score float64
}

// PoolGroupSkippedPartition records a partition skipped during budget publication.
type PoolGroupSkippedPartition struct {
	// PartitionName is the group-local partition name.
	PartitionName string

	// Reason is a diagnostic skip reason.
	Reason string
}

// PoolGroupControllerEvaluation is a pure group controller projection.
//
// Evaluation turns two group samples into an aggregate window, rates, and
// advisory score values. It does not mutate PoolGroup state, publish runtime
// policy, execute trim, redistribute budgets, or start background work.
type PoolGroupControllerEvaluation struct {
	// Window contains the aggregate countermovement.
	Window PoolGroupWindow

	// Rates contains aggregate window-derived ratios and throughput.
	Rates PoolGroupWindowRates

	// Scores contains pure scalar score projections.
	Scores PoolGroupScoreValues
}

// NewPoolGroupControllerEvaluation returns a pure group control evaluation.
func NewPoolGroupControllerEvaluation(
	previous PoolGroupSample,
	current PoolGroupSample,
	elapsed time.Duration,
	budget PoolGroupBudgetSnapshot,
	pressure PoolGroupPressureSnapshot,
	evaluator PoolGroupScoreEvaluator,
) PoolGroupControllerEvaluation {
	window := NewPoolGroupWindow(previous, current)
	rates := NewPoolGroupTimedWindowRates(window, elapsed)
	return PoolGroupControllerEvaluation{
		Window: window,
		Rates:  rates,
		Scores: evaluator.ScoreValues(rates, budget, pressure),
	}
}

// newGroupBudgetSnapshot projects group aggregate sample usage against group limits.
func newGroupBudgetSnapshot(policy PartitionBudgetPolicy, sample PoolGroupSample) PoolGroupBudgetSnapshot {
	return PoolGroupBudgetSnapshot(newPartitionBudgetSnapshot(policy, sample.Aggregate))
}

// newGroupPressureSnapshot projects group aggregate sample usage against pressure policy.
func newGroupPressureSnapshot(policy PartitionPressurePolicy, sample PoolGroupSample) PoolGroupPressureSnapshot {
	return PoolGroupPressureSnapshot(newPartitionPressureSnapshot(policy, sample.Aggregate))
}
