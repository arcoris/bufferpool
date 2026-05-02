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

// publishPartitionControllerBudgets applies the budget plan from evaluation.
//
// The helper only handles partition-to-Pool budget publication and report
// merging. It does not commit controller EWMA/sample state, mark dirty indexes
// processed, or execute trim. A returned error is a failed cycle; a nil error
// with Published=false remains the existing non-error unpublished path.
func (p *PoolPartition) publishPartitionControllerBudgets(cycle *partitionControllerCycleEvaluation) error {
	if len(cycle.poolBudgetTargets) == 0 || !cycle.budgetPublication.CanPublish() {
		return nil
	}

	appliedPublication, err := p.applyPoolBudgetTargetsLocked(cycle.poolBudgetTargets)
	cycle.budgetPublication.Published = appliedPublication.Published
	markControllerClassReportsPublished(&cycle.budgetPublication, appliedPublication)

	if err != nil {
		cycle.budgetPublication.FailureReason = controllerCycleBudgetPublicationFailureReasonForError(err, controllerCycleReasonFailed)
		return err
	}

	if !appliedPublication.Published {
		cycle.budgetPublication.FailureReason = appliedPublication.FailureReason
	}

	return nil
}
