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

// groupCoordinator owns applied group-local control state.
//
// TickInto is the only writer. PoolGroup Acquire, Release, diagnostics,
// PoolPartition TickInto, Pool.Get, and Pool.Put do not mutate this state. The
// mutex serializes previous-window storage, coordinator generation, and elapsed
// time measurement for manual foreground group coordinator cycles.
type groupCoordinator struct {
	// mu serializes all coordinator state mutation.
	mu sync.Mutex

	// cycleGate rejects overlapping manual TickInto calls before they block on
	// mu. It is not a scheduler and does not replace group lifecycle gates.
	cycleGate controllerCycleGate

	// status stores the lightweight outcome of the last foreground coordinator
	// attempt. Full reports are returned to callers and are not retained here.
	status controllerCycleStatusStore

	// previousSample is the last group sample retained for window deltas.
	previousSample PoolGroupSample

	// hasPreviousSample reports whether previousSample is initialized.
	hasPreviousSample bool

	// previousSampleTime is the clock timestamp associated with previousSample.
	previousSampleTime time.Time

	// generation is the coordinator cycle generation stream.
	generation AtomicGeneration

	// clock supplies deterministic elapsed time for foreground ticks.
	clock clock.Clock

	// policy is the normalized group policy observed at construction. Runtime
	// policy snapshots remain authoritative for each tick; this copy exists so
	// the coordinator has a stable initialized configuration boundary.
	policy PoolGroupPolicy
}

// init prepares group coordinator state in place.
//
// groupCoordinator contains sync.Mutex, so it must not be returned by value from
// a constructor after initialization. PoolGroup allocates the parent first and
// initializes this field in place to avoid copying lock state.
func (c *groupCoordinator) init(policy PoolGroupPolicy) {
	c.clock = clockDefault()
	c.policy = policy.Normalize()
	c.generation.Store(InitialGeneration)
}
