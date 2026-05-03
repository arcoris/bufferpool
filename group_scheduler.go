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

// startCoordinatorScheduler starts the opt-in group-level coordinator scheduler.
//
// The scheduler is construction-time only in the current integration. It is
// started after the group is fully initialized and active, and it dispatches
// through schedulerTick so scheduled cycles use exactly the same TickInto path
// as manual callers. A disabled coordinator policy is a no-op and keeps the
// default manual group behavior unchanged. PublishPolicy intentionally does not
// use this helper; live scheduler enable, disable, and interval retiming are not
// part of the current owner policy contract.
func (g *PoolGroup) startCoordinatorScheduler(tickerFactory controllerSchedulerTickerFactory) error {
	policy := g.config.Policy.Normalize().Coordinator
	if !policy.Enabled {
		return nil
	}
	return g.coordinatorScheduler.Start(controllerSchedulerStartInput{
		Interval:      policy.TickInterval,
		Tick:          g.schedulerTick,
		IsClosedError: func(err error) bool { return errors.Is(err, ErrClosed) },
		TickerFactory: tickerFactory,
	})
}

// stopCoordinatorScheduler stops the group-level coordinator scheduler.
//
// Close calls this before taking runtimeMu.Lock. That ordering matters because a
// scheduled TickInto may already hold or be waiting for runtimeMu.RLock. Stop
// waits for the scheduler goroutine without holding any group lock that TickInto
// might need, then Close can safely acquire runtimeMu and close child
// partitions.
func (g *PoolGroup) stopCoordinatorScheduler() {
	g.coordinatorScheduler.Stop()
}

// schedulerTick performs one scheduled group coordinator cycle.
//
// The report is intentionally stack-local and discarded immediately. TickInto is
// still responsible for publishing lightweight ControllerStatus, enforcing the
// no-overlap gate, publishing partition budget targets, and returning lifecycle
// errors to the scheduler runtime. This method does not tick partitions
// directly, call Pool.Get/Pool.Put, scan Pool internals, or execute Pool trim.
// ControllerStatus remains the last coordinator-cycle outcome; lifecycle
// queries remain responsible for telling callers whether the group itself is
// closed.
func (g *PoolGroup) schedulerTick() error {
	var report PoolGroupCoordinatorReport
	return g.TickInto(&report)
}
