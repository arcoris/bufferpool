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

// partitionControllerCycleEvaluation is the transient state for one TickInto
// attempt. It is passed between the evaluation, publication, commit, and report
// phases so TickInto can stay readable without retaining full reports or
// controller diagnostics after the call returns.
type partitionControllerCycleEvaluation struct {
	generation Generation
	runtime    *partitionRuntimeSnapshot
	now        time.Time

	indexes []int
	sample  PoolPartitionSample

	previous PoolPartitionSample
	elapsed  time.Duration
	window   PoolPartitionWindow
	rates    PoolPartitionWindowRates

	ewma          PoolPartitionEWMAState
	nextClassEWMA map[poolClassKey]PoolClassEWMAState

	metrics  PoolPartitionMetrics
	budget   PartitionBudgetSnapshot
	pressure PartitionPressureSnapshot
	scores   PoolPartitionScores

	trimPlan PartitionTrimPlan

	dirtyIndexes []int
	activeDeltas []bool

	budgetPublication PoolPartitionBudgetPublicationReport
	poolBudgetTargets []PoolBudgetTarget
}
