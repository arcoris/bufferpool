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

// SetPressure publishes a group pressure signal and propagates it to partitions.
//
// SetPressure is a foreground control-plane operation. It serializes with group
// hard Close, validates the level, publishes an immutable group pressure
// snapshot, and asks each partition to publish the signal to its owned Pools. It
// does not scan Pool shards and does not execute physical trim.
func (g *PoolGroup) SetPressure(level PressureLevel) error {
	g.mustBeInitialized()
	if err := (PressureSignal{Level: level}).validate(); err != nil {
		return err
	}

	g.runtimeMu.Lock()
	defer g.runtimeMu.Unlock()

	if !g.lifecycle.AllowsWork() {
		return newError(ErrClosed, errGroupClosed)
	}

	generation := g.generation.Advance()
	signal := PressureSignal{Level: level, Source: PressureSourceGroup, Generation: generation}

	runtime := g.currentRuntimeSnapshot()
	g.publishRuntimeSnapshot(newGroupRuntimeSnapshotWithPressure(generation, runtime.Policy, signal))

	for _, entry := range g.registry.entries {
		if err := entry.partition.applyPressure(signal); err != nil {
			return err
		}
	}
	return nil
}
