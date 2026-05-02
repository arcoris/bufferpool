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

import "errors"

// startControllerScheduler starts the opt-in partition-local controller scheduler.
//
// The scheduler is construction-time only in this stage. It is started after the
// partition is fully initialized and active, and it dispatches through
// schedulerTick so scheduled cycles use exactly the same TickInto path as manual
// callers. A disabled controller policy is a no-op and keeps the default manual
// partition behavior unchanged.
func (p *PoolPartition) startControllerScheduler(tickerFactory controllerSchedulerTickerFactory) error {
	policy := p.config.Policy.Normalize().Controller
	if !policy.Enabled {
		return nil
	}
	return p.controllerScheduler.Start(controllerSchedulerStartInput{
		Interval:      policy.TickInterval,
		Tick:          p.schedulerTick,
		IsClosedError: func(err error) bool { return errors.Is(err, ErrClosed) },
		TickerFactory: tickerFactory,
	})
}

// stopControllerScheduler stops the partition-local controller scheduler.
//
// Close calls this before taking the foreground close write lock. That ordering
// matters because a scheduled TickInto may already hold or be waiting for the
// foreground read side and controller.mu. Stop waits for the scheduler goroutine
// without holding any partition lock that TickInto might need, then Close can
// safely acquire foregroundMu and clean up child resources.
func (p *PoolPartition) stopControllerScheduler() {
	p.controllerScheduler.Stop()
}

// schedulerTick performs one scheduled partition controller cycle.
//
// The report is intentionally stack-local and discarded immediately. TickInto is
// still responsible for publishing lightweight ControllerStatus, enforcing the
// no-overlap gate, applying budgets, running bounded trim, and returning any
// lifecycle error to the scheduler runtime.
func (p *PoolPartition) schedulerTick() error {
	var report PartitionControllerReport
	return p.TickInto(&report)
}
