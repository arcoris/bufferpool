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

import "testing"

// TestPartitionActiveRegistryInitialState verifies deterministic active state.
func TestPartitionActiveRegistryInitialState(t *testing.T) {
	registry := newPartitionActiveRegistry([]string{"alpha", "beta", "gamma"})

	active := registry.activeIndexes(nil)
	want := []int{0, 1, 2}
	if len(active) != len(want) {
		t.Fatalf("active len = %d, want %d", len(active), len(want))
	}
	for index := range want {
		if active[index] != want[index] {
			t.Fatalf("active[%d] = %d, want %d", index, active[index], want[index])
		}
	}

	dirty := registry.dirtyIndexes(nil)
	if len(dirty) != 0 {
		t.Fatalf("dirty indexes = %v, want empty", dirty)
	}
	if registry.generationSnapshot() != InitialGeneration {
		t.Fatalf("generation = %s, want %s", registry.generationSnapshot(), InitialGeneration)
	}
}

// TestPartitionActiveRegistryReturnedIndexesAreDefensive verifies copy boundaries.
func TestPartitionActiveRegistryReturnedIndexesAreDefensive(t *testing.T) {
	registry := newPartitionActiveRegistry([]string{"alpha", "beta"})

	active := registry.activeIndexes(nil)
	active[0] = 99

	fresh := registry.activeIndexes(nil)
	if fresh[0] != 0 {
		t.Fatalf("activeIndexes exposed internal storage: %v", fresh)
	}
}

// TestPartitionActiveRegistryDirtySet verifies mark and reset behavior.
func TestPartitionActiveRegistryDirtySet(t *testing.T) {
	registry := newPartitionActiveRegistry([]string{"alpha", "beta", "gamma"})

	requirePartitionNoError(t, registry.markDirty("beta"))
	requirePartitionNoError(t, registry.markDirtyIndex(2))

	dirty := registry.dirtyIndexes(nil)
	want := []int{1, 2}
	if len(dirty) != len(want) {
		t.Fatalf("dirty len = %d, want %d", len(dirty), len(want))
	}
	for index := range want {
		if dirty[index] != want[index] {
			t.Fatalf("dirty[%d] = %d, want %d", index, dirty[index], want[index])
		}
	}

	registry.resetDirty()
	dirty = registry.dirtyIndexes(dirty)
	if len(dirty) != 0 {
		t.Fatalf("dirty after reset = %v, want empty", dirty)
	}
}

// TestPartitionActiveRegistryRejectsUnknownPool verifies unknown mark errors.
func TestPartitionActiveRegistryRejectsUnknownPool(t *testing.T) {
	registry := newPartitionActiveRegistry([]string{"alpha"})

	requirePartitionErrorIs(t, registry.markActive("missing"), ErrInvalidOptions)
	requirePartitionErrorIs(t, registry.markDirty("missing"), ErrInvalidOptions)
	requirePartitionErrorIs(t, registry.markActiveIndex(9), ErrInvalidOptions)
	requirePartitionErrorIs(t, registry.markDirtyIndex(9), ErrInvalidOptions)
}

// TestPoolPartitionMarksActivityFromAcquireReleaseAndPolicyPublish verifies integration markers.
func TestPoolPartitionMarksActivityFromAcquireReleaseAndPolicyPublish(t *testing.T) {
	partition := testNewPoolPartition(t, "alpha", "beta")
	partition.activeRegistry.resetDirty()

	lease, err := partition.Acquire("beta", 300)
	requirePartitionNoError(t, err)

	dirty := partition.activeRegistry.dirtyIndexes(nil)
	if len(dirty) != 1 || dirty[0] != 1 {
		t.Fatalf("dirty after Acquire(beta) = %v, want [1]", dirty)
	}

	partition.activeRegistry.resetDirty()
	requirePartitionNoError(t, partition.Release(lease, lease.Buffer()))
	dirty = partition.activeRegistry.dirtyIndexes(dirty)
	if len(dirty) != 1 || dirty[0] != 1 {
		t.Fatalf("dirty after Release(beta lease) = %v, want [1]", dirty)
	}

	partition.activeRegistry.resetDirty()
	policy := partition.Policy()
	partition.publishRuntimeSnapshot(newPartitionRuntimeSnapshot(Generation(9), policy))
	dirty = partition.activeRegistry.dirtyIndexes(dirty)
	if len(dirty) != 2 || dirty[0] != 0 || dirty[1] != 1 {
		t.Fatalf("dirty after partition policy publish = %v, want [0 1]", dirty)
	}

	partition.activeRegistry.resetDirty()
	poolPolicy := partition.config.Pools[0].Config.Policy
	requirePartitionNoError(t, partition.publishPoolRuntimeSnapshot("alpha", Generation(3), poolPolicy))
	dirty = partition.activeRegistry.dirtyIndexes(dirty)
	if len(dirty) != 1 || dirty[0] != 0 {
		t.Fatalf("dirty after pool policy publish = %v, want [0]", dirty)
	}
}
