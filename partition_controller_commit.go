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

// commitAppliedPartitionControllerCycle performs the applied-cycle side effects.
//
// Active-registry observation is the last normally fallible bookkeeping step
// before physical trim. It uses sampled registry indexes, so an error indicates
// an internal index invariant failure; running it before trim prevents a Failed
// status from hiding already-executed trim work. Physical trim then runs before
// controller sample/EWMA state is committed, preserving the existing applied
// tick ordering.
func (p *PoolPartition) commitAppliedPartitionControllerCycle(
	cycle *partitionControllerCycleEvaluation,
) (PartitionTrimResult, error) {
	if err := p.activeRegistry.observeControllerActivityWithDirtyIndexes(
		cycle.indexes,
		cycle.activeDeltas,
		cycle.dirtyIndexes,
	); err != nil {
		return PartitionTrimResult{}, err
	}

	trimResult := p.executeTrimPlanWithScoring(cycle.trimPlan, newPartitionTrimScoringContext(cycle.window))

	p.markDirtyProcessed()
	p.controller.previousSample = copyPoolPartitionSampleInto(p.controller.previousSample, cycle.sample)
	p.controller.previousSampleTime = cycle.now
	p.controller.hasPreviousSample = true
	p.controller.ewma = cycle.ewma
	p.controller.ewmaByPoolClass = cycle.nextClassEWMA
	p.controller.cycles++
	_ = p.controller.generation.Advance()

	return trimResult, nil
}
