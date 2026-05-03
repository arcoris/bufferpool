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

// publishGroupCoordinatorBudgets publishes the evaluated partition target batch.
//
// Publication crosses only the group-to-partition ownership boundary. It does
// not run partition TickInto, enable partition schedulers, inspect Pool shards,
// or execute Pool trim. A returned error is a failed coordinator cycle; a nil
// error with Published=false remains the existing no-work, infeasible, or
// skipped-child path handled by selectGroupCoordinatorStatus.
func (g *PoolGroup) publishGroupCoordinatorBudgets(
	cycle *groupCoordinatorCycleEvaluation,
	skippedPartitions []PoolGroupSkippedPartition,
) (PoolGroupBudgetPublicationReport, error) {
	cycle.skippedPartitions = skippedPartitions

	budgetPublication := PoolGroupBudgetPublicationReport{
		Generation: cycle.generation,
		Allocation: newBudgetAllocationDiagnostics(cycle.partitionBudgetAllocation.Allocation),
		Targets:    cycle.partitionBudgetTargets,
		Published:  false,
	}

	if len(cycle.partitionBudgetTargets) == 0 {
		return budgetPublication, nil
	}

	if !cycle.partitionBudgetAllocation.Allocation.Feasible {
		budgetPublication.FailureReason = cycle.partitionBudgetAllocation.Allocation.Reason
		return budgetPublication, nil
	}

	skippedPartitions, err := g.publishPartitionBudgetTargets(cycle.partitionBudgetTargets, skippedPartitions)
	if err != nil {
		return budgetPublication, err
	}

	cycle.skippedPartitions = skippedPartitions
	budgetPublication.SkippedPartitions = skippedPartitions
	budgetPublication.Published = len(skippedPartitions) == 0
	if !budgetPublication.Published {
		budgetPublication.FailureReason = policyUpdateFailureSkippedChild
	}

	return budgetPublication, nil
}

// selectGroupCoordinatorStatus maps publication diagnostics to retained status.
//
// No-target cycles are Skipped because no group-level budget target work exists.
// Skipped-child or infeasible publication cycles are Unpublished. They still
// commit coordinator observation state later so the next group window advances
// exactly as it did before this refactor.
func selectGroupCoordinatorStatus(
	cycle *groupCoordinatorCycleEvaluation,
	budgetPublication PoolGroupBudgetPublicationReport,
) groupCoordinatorCycleStatusDecision {
	if len(cycle.partitionBudgetTargets) == 0 {
		return groupCoordinatorCycleStatusDecision{
			statusKind:          ControllerCycleStatusSkipped,
			appliedGeneration:   NoGeneration,
			publishedGeneration: NoGeneration,
			failureReason:       controllerCycleReasonNoWork,
		}
	}

	if !budgetPublication.Published {
		return groupCoordinatorCycleStatusDecision{
			statusKind:          ControllerCycleStatusUnpublished,
			appliedGeneration:   NoGeneration,
			publishedGeneration: NoGeneration,
			failureReason:       controllerCycleUnpublishedFailureReason(budgetPublication.FailureReason),
		}
	}

	return groupCoordinatorCycleStatusDecision{
		statusKind:          ControllerCycleStatusApplied,
		appliedGeneration:   cycle.generation,
		publishedGeneration: cycle.generation,
		failureReason:       "",
	}
}
