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

// TestPoolPartitionSnapshotContainsPoolsLeasesAndMetrics verifies diagnostic snapshot contents.
func TestPoolPartitionSnapshotContainsPoolsLeasesAndMetrics(t *testing.T) {
	partition := testNewPoolPartition(t, "primary", "secondary")

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	defer func() { requirePartitionNoError(t, partition.Release(lease, lease.Buffer())) }()

	snapshot := partition.Snapshot()

	if snapshot.Name != "test-partition" {
		t.Fatalf("snapshot name = %q, want test-partition", snapshot.Name)
	}
	if snapshot.Lifecycle != LifecycleActive {
		t.Fatalf("snapshot lifecycle = %s, want active", snapshot.Lifecycle)
	}
	if snapshot.PolicyGeneration != InitialGeneration {
		t.Fatalf("snapshot policy generation = %s, want %s", snapshot.PolicyGeneration, InitialGeneration)
	}
	if snapshot.Metrics.Generation != snapshot.Generation {
		t.Fatalf("snapshot metrics generation = %s, want snapshot generation %s", snapshot.Metrics.Generation, snapshot.Generation)
	}
	if snapshot.Metrics.PolicyGeneration != snapshot.PolicyGeneration {
		t.Fatalf("snapshot metrics policy generation = %s, want %s", snapshot.Metrics.PolicyGeneration, snapshot.PolicyGeneration)
	}
	if snapshot.PoolCount() != 2 {
		t.Fatalf("PoolCount() = %d, want 2", snapshot.PoolCount())
	}
	if snapshot.Leases.ActiveCount() != 1 {
		t.Fatalf("active leases in snapshot = %d, want 1", snapshot.Leases.ActiveCount())
	}
	if snapshot.Metrics.ActiveLeases != 1 {
		t.Fatalf("snapshot metrics active leases = %d, want 1", snapshot.Metrics.ActiveLeases)
	}
	if snapshot.IsEmpty() {
		t.Fatalf("snapshot with active lease must not be empty")
	}
}

// TestPoolPartitionSnapshotIsDefensive verifies snapshot copy boundaries.
func TestPoolPartitionSnapshotIsDefensive(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")

	snapshot := partition.Snapshot()
	if len(snapshot.Pools) != 1 {
		t.Fatalf("len(snapshot.Pools) = %d, want 1", len(snapshot.Pools))
	}

	snapshot.Pools[0].Name = "mutated"
	snapshot.Config.Pools[0].Name = "mutated-config"
	if len(snapshot.Config.Pools[0].Config.Policy.Classes.Sizes) == 0 {
		t.Fatalf("test requires class sizes")
	}
	snapshot.Config.Pools[0].Config.Policy.Classes.Sizes[0] = ClassSize(12345)

	fresh := partition.Snapshot()
	if fresh.Pools[0].Name != "primary" {
		t.Fatalf("snapshot shared Pool slice with caller")
	}
	if fresh.Config.Pools[0].Name != "primary" {
		t.Fatalf("snapshot shared Config pool slice with caller")
	}
	if fresh.Config.Pools[0].Config.Policy.Classes.Sizes[0] == ClassSize(12345) {
		t.Fatalf("snapshot shared Config policy class slice with caller")
	}
}

// TestPoolPartitionSnapshotEmptyForFreshPartition verifies empty diagnostic state.
func TestPoolPartitionSnapshotEmptyForFreshPartition(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")

	snapshot := partition.Snapshot()
	if !snapshot.IsEmpty() {
		t.Fatalf("fresh partition snapshot should be empty from activity perspective: %+v", snapshot.Metrics)
	}
}

// TestPoolPartitionSnapshotUsesCoherentPolicyGeneration verifies generation reporting.
func TestPoolPartitionSnapshotUsesCoherentPolicyGeneration(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")

	policy := PartitionPolicy{Budget: PartitionBudgetPolicy{MaxOwnedBytes: 64 * KiB}}
	partition.publishRuntimeSnapshot(newPartitionRuntimeSnapshot(Generation(9), policy))

	snapshot := partition.Snapshot()
	if snapshot.PolicyGeneration != Generation(9) {
		t.Fatalf("snapshot policy generation = %s, want 9", snapshot.PolicyGeneration)
	}
	if snapshot.Metrics.PolicyGeneration != snapshot.PolicyGeneration {
		t.Fatalf("metrics policy generation = %s, want snapshot policy generation %s", snapshot.Metrics.PolicyGeneration, snapshot.PolicyGeneration)
	}
}

// TestPoolPartitionSnapshotMatchesQuiescentSample verifies simple quiescent consistency.
func TestPoolPartitionSnapshotMatchesQuiescentSample(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	requirePartitionNoError(t, partition.Release(lease, lease.Buffer()))

	sample := partition.Sample()
	snapshot := partition.Snapshot()

	if snapshot.Metrics.CurrentRetainedBytes != sample.CurrentRetainedBytes {
		t.Fatalf("snapshot retained bytes = %d, sample retained bytes = %d", snapshot.Metrics.CurrentRetainedBytes, sample.CurrentRetainedBytes)
	}
	if snapshot.Metrics.ActiveLeases != 0 || snapshot.Leases.ActiveCount() != 0 {
		t.Fatalf("snapshot active leases = metrics %d leases %d, want zero", snapshot.Metrics.ActiveLeases, snapshot.Leases.ActiveCount())
	}
	if snapshot.Metrics.PolicyGeneration != snapshot.PolicyGeneration {
		t.Fatalf("snapshot metrics policy generation = %s, want %s", snapshot.Metrics.PolicyGeneration, snapshot.PolicyGeneration)
	}
}
