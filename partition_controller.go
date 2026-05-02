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

// Tick performs one explicit partition controller cycle.
//
// The current implementation samples active Pools, computes window deltas,
// updates partition-owned EWMA state, scores active Pool classes, publishes class
// budgets into owned Pools, and executes bounded physical trim when the trim
// plan is enabled. It does not start background scheduling. Controller.Enabled
// controls future automatic scheduling; manual Tick remains available when the
// partition is active.
//
// Tick returns a detailed diagnostic/report value and may allocate per-Pool
// sample/report storage. Controller sampling uses the active/dirty registry and
// periodic full scans, so ordinary ticks can narrow work after clean idle Pools
// age out while still revalidating inactive Pools deterministically.
func (p *PoolPartition) Tick() (PartitionControllerReport, error) {
	p.mustBeInitialized()
	var report PartitionControllerReport
	err := p.TickInto(&report)
	return report, err
}

// TickInto writes one explicit partition controller cycle into dst.
//
// TickInto has the same control semantics as Tick but reuses dst.Sample.Pools
// capacity. A nil dst is a no-op after receiver and lifecycle validation. Like
// Tick, this is a manual foreground call: it does not start background
// goroutines, coordinate with PoolGroup, or redistribute group-level budgets.
//
// If budget publication returns an error, TickInto reports Failed, returns the
// original error, and commits no controller state. If publication is a non-error
// diagnostic non-publication, TickInto reports Unpublished and also commits no
// controller state. That keeps observed-but-unapplied ticks from half-advancing
// EWMA, previous sample/time, dirty-marker processing, trim execution, or
// controller generation.
//
// Applied is owner-specific: a partition tick can be Applied because it
// committed observation, EWMA, active/dirty, and trim controller state even when
// no Pool budget targets were needed. Group ticks use Skipped for their own
// no-target case because the group has no lower-level observation state to
// apply in that path.
func (p *PoolPartition) TickInto(dst *PartitionControllerReport) error {
	p.mustBeInitialized()
	if err := p.beginForegroundOperation(); err != nil {
		p.finishClosedPartitionControllerCycle(dst)
		return err
	}
	defer p.endForegroundOperation()

	if dst == nil {
		return nil
	}

	if !p.controller.cycleGate.begin() {
		p.finishAlreadyRunningPartitionControllerCycle(dst)
		return nil
	}
	defer p.controller.cycleGate.end()

	p.controller.mu.Lock()
	defer p.controller.mu.Unlock()

	cycle := p.evaluatePartitionControllerCycle(dst)
	if err := p.publishPartitionControllerBudgets(&cycle); err != nil {
		p.finishFailedPartitionControllerCycle(dst, &cycle, err)
		return err
	}

	if len(cycle.poolBudgetTargets) > 0 && !cycle.budgetPublication.Published {
		p.finishUnpublishedPartitionControllerCycle(dst, &cycle)
		return nil
	}

	trimResult, err := p.commitAppliedPartitionControllerCycle(&cycle)
	if err != nil {
		p.finishFailedPartitionControllerCommit(dst, &cycle, err)
		return err
	}

	p.finishAppliedPartitionControllerCycle(dst, &cycle, trimResult)
	return nil
}

// ControllerStatus returns the lightweight status for the last manual
// partition controller cycle.
//
// The accessor is safe after Close and returns a copy. It does not sample Pools,
// publish budgets, execute trim, or retain the heavy report returned by TickInto.
func (p *PoolPartition) ControllerStatus() PoolPartitionControllerStatus {
	p.mustBeInitialized()
	return p.controller.status.load()
}
