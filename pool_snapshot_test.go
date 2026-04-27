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

// TestPoolSnapshotInitialState verifies the snapshot projection immediately
// after construction.
func TestPoolSnapshotInitialState(t *testing.T) {
	t.Parallel()

	policy := poolTestSmallSingleShardPolicy()
	pool := MustNew(PoolConfig{
		Name:   "snapshots",
		Policy: policy,
	})
	defer closePoolForTest(t, pool)

	snapshot := pool.Snapshot()

	if snapshot.Name != "snapshots" {
		t.Fatalf("Snapshot().Name = %q, want %q", snapshot.Name, "snapshots")
	}

	if snapshot.Lifecycle != LifecycleActive {
		t.Fatalf("Snapshot().Lifecycle = %s, want %s", snapshot.Lifecycle, LifecycleActive)
	}

	if snapshot.ClassCount() != len(policy.Classes.Sizes) {
		t.Fatalf("Snapshot().ClassCount() = %d, want %d", snapshot.ClassCount(), len(policy.Classes.Sizes))
	}

	if snapshot.ShardCount() != len(policy.Classes.Sizes)*policy.Shards.ShardsPerClass {
		t.Fatalf("Snapshot().ShardCount() = %d, want %d",
			snapshot.ShardCount(),
			len(policy.Classes.Sizes)*policy.Shards.ShardsPerClass,
		)
	}

	if snapshot.CurrentRetainedBuffers != 0 || snapshot.CurrentRetainedBytes != 0 {
		t.Fatalf("initial retained usage = %d/%d, want zero",
			snapshot.CurrentRetainedBuffers,
			snapshot.CurrentRetainedBytes,
		)
	}

	if !snapshot.Counters.IsZero() {
		t.Fatalf("initial counters are not zero: %#v", snapshot.Counters)
	}

	if snapshot.IsEmpty() != true {
		t.Fatal("initial snapshot should be empty")
	}

	for classIndex, class := range snapshot.Classes {
		if class.Class.Size() != policy.Classes.Sizes[classIndex] {
			t.Fatalf("class %d size = %s, want %s",
				classIndex,
				class.Class.Size(),
				policy.Classes.Sizes[classIndex],
			)
		}

		if class.ShardCount() != policy.Shards.ShardsPerClass {
			t.Fatalf("class %d shard count = %d, want %d",
				classIndex,
				class.ShardCount(),
				policy.Shards.ShardsPerClass,
			)
		}

		if class.Budget.IsZero() {
			t.Fatalf("class %d budget is zero", classIndex)
		}
	}
}

// TestPoolSnapshotAfterGetPut verifies aggregate counters and retained gauges.
func TestPoolSnapshotAfterGetPut(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	buffer, err := pool.Get(512)
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}

	if err := pool.Put(buffer); err != nil {
		t.Fatalf("Put() returned error: %v", err)
	}

	snapshot := pool.Snapshot()

	if snapshot.IsEmpty() {
		t.Fatal("snapshot should not be empty after Get/Put")
	}

	if snapshot.Counters.Gets != 1 {
		t.Fatalf("snapshot gets = %d, want 1", snapshot.Counters.Gets)
	}

	if snapshot.Counters.Misses != 1 {
		t.Fatalf("snapshot misses = %d, want 1", snapshot.Counters.Misses)
	}

	if snapshot.Counters.Allocations != 1 {
		t.Fatalf("snapshot allocations = %d, want 1", snapshot.Counters.Allocations)
	}

	if snapshot.Counters.Puts != 1 {
		t.Fatalf("snapshot puts = %d, want 1", snapshot.Counters.Puts)
	}

	if snapshot.Counters.Retains != 1 {
		t.Fatalf("snapshot retains = %d, want 1", snapshot.Counters.Retains)
	}

	if snapshot.CurrentRetainedBuffers != 1 {
		t.Fatalf("snapshot retained buffers = %d, want 1", snapshot.CurrentRetainedBuffers)
	}

	if snapshot.CurrentRetainedBytes == 0 {
		t.Fatal("snapshot retained bytes = 0, want positive")
	}
}

// TestPoolSnapshotAfterReuse verifies hit accounting after retained reuse.
func TestPoolSnapshotAfterReuse(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	if err := pool.Put(make([]byte, 0, KiB.Int())); err != nil {
		t.Fatalf("Put() returned error: %v", err)
	}

	if _, err := pool.Get(700); err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}

	snapshot := pool.Snapshot()

	if snapshot.Counters.Hits != 1 {
		t.Fatalf("snapshot hits = %d, want 1", snapshot.Counters.Hits)
	}

	if snapshot.Counters.Misses != 0 {
		t.Fatalf("snapshot misses = %d, want 0", snapshot.Counters.Misses)
	}

	if snapshot.CurrentRetainedBuffers != 0 || snapshot.CurrentRetainedBytes != 0 {
		t.Fatalf("retained usage after reuse = %d/%d, want zero",
			snapshot.CurrentRetainedBuffers,
			snapshot.CurrentRetainedBytes,
		)
	}
}

// TestPoolSnapshotDefensivePolicyCopy verifies that Snapshot does not expose the
// live Pool policy slice.
func TestPoolSnapshotDefensivePolicyCopy(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	snapshot := pool.Snapshot()
	if len(snapshot.Policy.Classes.Sizes) == 0 {
		t.Fatal("snapshot policy class sizes are empty")
	}

	original := pool.Policy().Classes.Sizes[0]
	snapshot.Policy.Classes.Sizes[0] = ClassSizeFromSize(8 * MiB)

	if got := pool.Policy().Classes.Sizes[0]; got != original {
		t.Fatalf("mutating snapshot policy changed Pool policy: got %s, want %s", got, original)
	}
}

// TestPoolSnapshotHelpers verifies small helper methods on snapshot DTOs.
func TestPoolSnapshotHelpers(t *testing.T) {
	t.Parallel()

	counters := PoolCountersSnapshot{
		Hits:                   2,
		Misses:                 3,
		Retains:                4,
		Drops:                  5,
		TrimOperations:         6,
		ClearOperations:        7,
		TrimmedBuffers:         8,
		ClearedBuffers:         9,
		TrimmedBytes:           10,
		ClearedBytes:           11,
		CurrentRetainedBuffers: 1,
	}

	if counters.IsZero() {
		t.Fatal("counters with fields set reported zero")
	}

	if counters.ReuseAttempts() != 5 {
		t.Fatalf("ReuseAttempts() = %d, want 5", counters.ReuseAttempts())
	}

	if counters.PutOutcomes() != 9 {
		t.Fatalf("PutOutcomes() = %d, want 9", counters.PutOutcomes())
	}

	if counters.RemovalOperations() != 13 {
		t.Fatalf("RemovalOperations() = %d, want 13", counters.RemovalOperations())
	}

	if counters.RemovedBuffers() != 17 {
		t.Fatalf("RemovedBuffers() = %d, want 17", counters.RemovedBuffers())
	}

	if counters.RemovedBytes() != 21 {
		t.Fatalf("RemovedBytes() = %d, want 21", counters.RemovedBytes())
	}

	bucket := PoolBucketSnapshot{
		RetainedBuffers: 1,
		AvailableSlots:  0,
	}
	if bucket.IsEmpty() {
		t.Fatal("non-empty bucket reported empty")
	}
	if !bucket.IsFull() {
		t.Fatal("bucket with zero available slots did not report full")
	}

	credit := PoolShardCreditSnapshot{
		ClassSize:     ClassSizeFromBytes(512),
		TargetBuffers: 1,
		TargetBytes:   512,
	}
	if !credit.IsEnabled() {
		t.Fatal("enabled credit reported disabled")
	}
	if !credit.IsConsistent() {
		t.Fatal("valid credit reported inconsistent")
	}

	partial := PoolShardCreditSnapshot{TargetBuffers: 1}
	if !partial.IsPartial() {
		t.Fatal("partial credit did not report partial")
	}
	if partial.IsConsistent() {
		t.Fatal("partial credit reported consistent")
	}

	inconsistent := PoolShardCreditSnapshot{
		ClassSize:     ClassSizeFromBytes(512),
		TargetBuffers: 2,
		TargetBytes:   512,
	}
	if inconsistent.IsConsistent() {
		t.Fatal("credit with fewer target bytes than class-sized target buffers reported consistent")
	}

	missingClassSize := PoolShardCreditSnapshot{
		TargetBuffers: 1,
		TargetBytes:   512,
	}
	if missingClassSize.IsConsistent() {
		t.Fatal("enabled credit without class size reported consistent")
	}
}
