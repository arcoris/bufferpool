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

// PoolGroupCoordinatorReport describes one explicit foreground group observation.
//
// The report is an observational projection. It is not an applied decision and
// must not be interpreted as permission to mutate partition policies, apply
// budgets, execute trim, or start a controller loop.
type PoolGroupCoordinatorReport struct {
	// Generation is the group event generation for this tick.
	Generation Generation

	// PolicyGeneration is the group runtime-policy generation used by the tick.
	PolicyGeneration Generation

	// Lifecycle is the observed group lifecycle state.
	Lifecycle LifecycleState

	// Sample is the detailed aggregate group sample captured by the tick.
	Sample PoolGroupSample

	// Metrics is the lifetime-derived metrics projection from Sample.
	Metrics PoolGroupMetrics

	// Budget is the group budget projection from Sample.
	Budget PartitionBudgetSnapshot

	// Pressure is the group pressure interpretation from Sample.
	Pressure PartitionPressureSnapshot

	// Scores contains advisory scalar score values. Foreground Tick has no
	// previous sample, so workload-window components remain zero until callers use
	// PoolGroupControllerEvaluation with explicit previous/current samples.
	Scores PoolGroupScoreValues
}

// PoolGroupControllerEvaluation is a pure group controller projection.
//
// Evaluation turns two group samples into an aggregate window, rates, and
// advisory score values. It does not mutate PoolGroup state, publish runtime
// policy, execute trim, redistribute budgets, or start background work.
type PoolGroupControllerEvaluation struct {
	// Window contains the aggregate counter movement.
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
	budget PartitionBudgetSnapshot,
	pressure PartitionPressureSnapshot,
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
