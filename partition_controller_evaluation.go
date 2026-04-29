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
// publish runtime policy, execute trim, or start background work. Future
// controller decisions must wrap this projection with hysteresis, cooldown,
// rollback, and verification gates before applying policy.
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
	updatedEWMA := ewma.WithUpdate(ewmaConfig, rates)
	scores := NewPoolPartitionScores(rates, updatedEWMA, budget, pressure)
	return PoolPartitionControllerEvaluation{
		Window:         window,
		Rates:          rates,
		EWMA:           updatedEWMA,
		Scores:         scores,
		Recommendation: NewPoolPartitionRecommendation(scores),
	}
}
