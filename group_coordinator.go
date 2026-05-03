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
// A group skipped status means the coordinator found no group-level budget
// target work to publish. An unpublished budget cycle still reports the attempt
// Generation and commits coordinator observation state, but PublishedGeneration
// remains NoGeneration unless every partition accepts the target batch. That
// separation lets callers distinguish "observed and reported" from "runtime
// budget state changed".
func (g *PoolGroup) TickInto(dst *PoolGroupCoordinatorReport) error {
	g.mustBeInitialized()
	g.runtimeMu.RLock()
	defer g.runtimeMu.RUnlock()

	if !g.lifecycle.AllowsWork() {
		g.finishClosedGroupCoordinatorCycle(dst)
		return newError(ErrClosed, errGroupClosed)
	}

	if dst == nil {
		return nil
	}

	if !g.coordinator.cycleGate.begin() {
		g.finishAlreadyRunningGroupCoordinatorCycle(dst)
		return nil
	}
	defer g.coordinator.cycleGate.end()

	g.coordinator.mu.Lock()
	defer g.coordinator.mu.Unlock()

	cycle := g.evaluateGroupCoordinatorCycle(dst)
	budgetPublication, err := g.publishGroupCoordinatorBudgets(&cycle, dst.SkippedPartitions[:0])
	if err != nil {
		g.finishFailedGroupCoordinatorCycle(dst, &cycle, err)
		return err
	}

	decision := selectGroupCoordinatorStatus(&cycle, budgetPublication)
	coordinatorGeneration := g.commitGroupCoordinatorCycle(&cycle)
	g.finishGroupCoordinatorCycle(dst, &cycle, budgetPublication, decision, coordinatorGeneration)
	return nil
}

// ControllerStatus returns the lightweight status for the last manual group
// coordinator cycle.
//
// The accessor is safe after Close and returns a copy. It does not sample
// partitions, publish targets, run child ticks, or retain the heavy report
// returned by TickInto.
func (g *PoolGroup) ControllerStatus() PoolGroupControllerStatus {
	g.mustBeInitialized()
	return g.coordinator.status.load()
}
