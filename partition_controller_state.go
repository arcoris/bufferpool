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

import (
	"sync"
	"time"

	"arcoris.dev/bufferpool/internal/clock"
)

const (
	// partitionControllerFullScanInterval bounds how long inactive Pools can avoid
	// controller sampling.
	//
	// The controller has no background goroutine. Full scans happen only on manual
	// foreground TickInto calls and exist to revalidate inactive Pools without
	// making every tick scan every Pool.
	partitionControllerFullScanInterval uint64 = 16
)

// partitionController owns adaptive partition-local state.
//
// TickInto is the only writer. Pool.Get, Pool.Put, LeaseRegistry, PoolGroup, and
// background goroutines do not read or mutate this state. The mutex serializes
// previous-window storage, EWMA maps, cycle counters, and controller generation.
type partitionController struct {
	// mu serializes all controller state mutation.
	mu sync.Mutex

	// previousSample is the last sample retained for window deltas.
	previousSample PoolPartitionSample

	// hasPreviousSample reports whether previousSample is initialized.
	hasPreviousSample bool

	// previousSampleTime is the clock timestamp associated with previousSample.
	previousSampleTime time.Time

	// ewma stores partition-level smoothed signals.
	ewma PoolPartitionEWMAState

	// ewmaByPoolClass stores class-level smoothed signals by partition Pool and
	// class id.
	ewmaByPoolClass map[poolClassKey]PoolClassEWMAState

	// generation is the controller cycle generation stream.
	generation AtomicGeneration

	// clock supplies deterministic elapsed time for foreground ticks.
	clock clock.Clock

	// policy is the normalized partition policy observed at construction. Runtime
	// policy snapshots remain authoritative for each tick; this copy exists so the
	// controller has a stable initialized configuration boundary.
	policy PartitionPolicy

	// cycles counts foreground controller cycles.
	cycles uint64
}

// init prepares partition controller state in place.
//
// partitionController contains sync.Mutex, so it must not be returned by value
// from a constructor after initialization. PoolPartition allocates the parent
// struct first and initializes this field in place to avoid copying lock state.
func (c *partitionController) init(clk clock.Clock, policy PartitionPolicy) {
	if clk == nil {
		clk = clockDefault()
	}
	c.clock = clk
	c.policy = policy.Normalize()
	c.ewmaByPoolClass = make(map[poolClassKey]PoolClassEWMAState)
	c.generation.Store(InitialGeneration)
}

// clockDefault isolates the concrete production clock constructor for tests that
// replace controller.clock directly.
func clockDefault() clock.Clock {
	return clock.Default()
}

// clockNow returns the current controller timestamp from c.
func clockNow(c clock.Clock) time.Time {
	return clock.Now(c)
}

// clockElapsed returns elapsed controller time between two timestamps.
func clockElapsed(start, end time.Time) time.Duration {
	return clock.Elapsed(start, end)
}
