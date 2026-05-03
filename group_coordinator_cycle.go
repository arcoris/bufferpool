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

// groupCoordinatorCycleEvaluation is the transient state for one TickInto call.
//
// The value is passed between evaluation, publication, commit, and report
// helpers so PoolGroup.TickInto can expose the orchestration order directly.
// It is not retained by the coordinator, scheduler, or status store; full
// diagnostics remain caller-owned through PoolGroupCoordinatorReport.
type groupCoordinatorCycleEvaluation struct {
	generation Generation
	runtime    *groupRuntimeSnapshot
	now        time.Time

	sample PoolGroupSample

	previous PoolGroupSample
	elapsed  time.Duration
	window   PoolGroupWindow
	rates    PoolGroupWindowRates

	metrics  PoolGroupMetrics
	budget   PoolGroupBudgetSnapshot
	pressure PoolGroupPressureSnapshot
	scores   PoolGroupScoreValues

	partitionScores           []PoolGroupPartitionScore
	partitionBudgetAllocation partitionBudgetAllocationReport
	partitionBudgetTargets    []PartitionBudgetTarget
	skippedPartitions         []PoolGroupSkippedPartition
}

// groupCoordinatorCycleStatusDecision describes the retained status that should
// be published after a non-error coordinator cycle.
//
// The group coordinator commits observation state for skipped and unpublished
// cycles, so this status decision is intentionally separate from the commit
// decision. AppliedGeneration and PublishedGeneration remain NoGeneration unless
// every partition accepted the group-to-partition budget target batch.
type groupCoordinatorCycleStatusDecision struct {
	statusKind          ControllerCycleStatus
	appliedGeneration   Generation
	publishedGeneration Generation
	failureReason       string
}
