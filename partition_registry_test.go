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

// TestPoolPartitionRegistryLookupAndNamesAreDeterministic verifies registry order and lookup.
func TestPoolPartitionRegistryLookupAndNamesAreDeterministic(t *testing.T) {
	partition := testNewPoolPartition(t, "alpha", "beta", "gamma")

	names := partition.PoolNames()
	want := []string{"alpha", "beta", "gamma"}
	if len(names) != len(want) {
		t.Fatalf("PoolNames length = %d, want %d", len(names), len(want))
	}
	for index := range want {
		if names[index] != want[index] {
			t.Fatalf("PoolNames[%d] = %q, want %q", index, names[index], want[index])
		}
	}

	names[0] = "mutated"
	fresh := partition.PoolNames()
	if fresh[0] != "alpha" {
		t.Fatalf("PoolNames returned mutable registry storage")
	}

	for _, name := range want {
		snapshot, ok := partition.PoolSnapshot(name)
		if !ok {
			t.Fatalf("PoolSnapshot(%q) ok = false", name)
		}
		if snapshot.Name == "" {
			t.Fatalf("PoolSnapshot(%q) returned empty pool name", name)
		}
	}
	if _, ok := partition.PoolSnapshot("missing"); ok {
		t.Fatalf("PoolSnapshot(missing) ok = true, want false")
	}
	if _, ok := partition.PoolMetrics("missing"); ok {
		t.Fatalf("PoolMetrics(missing) ok = true, want false")
	}
}

// TestPartitionRegistryRejectsEmptyNamesDuringConstruction verifies defensive name validation.
func TestPartitionRegistryRejectsEmptyNamesDuringConstruction(t *testing.T) {
	_, err := newPartitionRegistry([]PartitionPoolConfig{{Name: "   ", Config: PoolConfig{Name: "unused"}}})
	requirePartitionErrorIs(t, err, ErrInvalidOptions)
}

// TestPartitionRegistryAcceptsNoPoolsDuringConstruction verifies empty partitions.
func TestPartitionRegistryAcceptsNoPoolsDuringConstruction(t *testing.T) {
	registry, err := newPartitionRegistry(nil)
	requirePartitionNoError(t, err)
	if registry.len() != 0 {
		t.Fatalf("registry len = %d, want 0", registry.len())
	}
	if names := registry.namesCopy(); len(names) != 0 {
		t.Fatalf("registry names = %#v, want empty", names)
	}
}

// TestPartitionRegistryRejectsDuplicateNamesDuringConstruction verifies duplicate rejection.
func TestPartitionRegistryRejectsDuplicateNamesDuringConstruction(t *testing.T) {
	_, err := newPartitionRegistry([]PartitionPoolConfig{
		testPartitionPoolConfig("alpha"),
		testPartitionPoolConfig("alpha"),
	})
	requirePartitionErrorIs(t, err, ErrInvalidOptions)
}

// TestPartitionRegistryCloseAllIsIdempotent verifies repeated close behavior.
func TestPartitionRegistryCloseAllIsIdempotent(t *testing.T) {
	registry, err := newPartitionRegistry([]PartitionPoolConfig{
		testPartitionPoolConfig("alpha"),
		testPartitionPoolConfig("beta"),
	})
	requirePartitionNoError(t, err)

	requirePartitionNoError(t, registry.closeAll())
	requirePartitionNoError(t, registry.closeAll())

	for _, name := range []string{"alpha", "beta"} {
		pool, ok := registry.pool(name)
		if !ok {
			t.Fatalf("registry.pool(%q) ok = false", name)
		}
		if !pool.IsClosed() {
			t.Fatalf("pool %q is not closed after registry.closeAll", name)
		}
	}
}
