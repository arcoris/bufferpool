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

// TestStaticPoolRuntimeIntegration exercises the implemented static data-plane
// path as one coherent runtime stack.
//
// The test intentionally crosses several internal boundaries because there is
// no public PoolGroup/PoolPartition/Lease API yet. Public Get/Put calls verify
// the owner-level path, and direct classState calls verify class-table routing
// invariants that Pool normally enforces before delegating to classes.
func TestStaticPoolRuntimeIntegration(t *testing.T) {
	t.Parallel()

	policy := poolTestIntegrationPolicy()
	table := newClassTable(policy.Classes.Sizes)

	capacityClass, ok := table.classForCapacity(SizeFromInt(700))
	if !ok {
		t.Fatal("classForCapacity(700) returned no class")
	}
	if capacityClass.Size() != ClassSizeFromBytes(512) {
		t.Fatalf("classForCapacity(700) = %s, want 512 B", capacityClass.Size())
	}

	requestClass, ok := table.classForRequest(SizeFromInt(700))
	if !ok {
		t.Fatal("classForRequest(700) returned no class")
	}
	if requestClass.Size() != ClassSizeFromSize(KiB) {
		t.Fatalf("classForRequest(700) = %s, want 1 KiB", requestClass.Size())
	}

	pool := MustNew(PoolConfig{Policy: policy})
	defer closePoolForTest(t, pool)

	if err := pool.Put(make([]byte, 0, 700)); err != nil {
		t.Fatalf("Put(700-capacity) returned error: %v", err)
	}

	afterCapacityRetain := pool.Snapshot()
	assertPoolSnapshotRetainedConsistency(t, afterCapacityRetain)

	class512 := mustPoolClassSnapshot(t, afterCapacityRetain, ClassSizeFromBytes(512))
	if class512.CurrentRetainedBuffers != 1 || class512.CurrentRetainedBytes != 700 {
		t.Fatalf("512 B class retained usage = %d/%d, want 1/700",
			class512.CurrentRetainedBuffers,
			class512.CurrentRetainedBytes,
		)
	}

	oneKiBState := pool.mustClassStateFor(requestClass)
	beforeRejectedClassRetain := oneKiBState.state()
	rejected := oneKiBState.tryRetain(0, make([]byte, 0, 700))
	if !rejected.RejectedByClass() {
		t.Fatalf("1 KiB class retained 700-capacity buffer: %#v", rejected)
	}

	afterRejectedClassRetain := oneKiBState.state()
	assertClassStateRetainedConsistency(t, afterRejectedClassRetain)
	if afterRejectedClassRetain.Shards[0].Counters.Puts != beforeRejectedClassRetain.Shards[0].Counters.Puts {
		t.Fatal("class-level rejection touched shard put counters")
	}
	if afterRejectedClassRetain.Shards[0].Bucket.RetainedBuffers != beforeRejectedClassRetain.Shards[0].Bucket.RetainedBuffers {
		t.Fatal("class-level rejection mutated shard bucket state")
	}

	if err := pool.Put(make([]byte, 0, KiB.Int())); err != nil {
		t.Fatalf("Put(1 KiB capacity) returned error: %v", err)
	}

	reused, err := pool.Get(700)
	if err != nil {
		t.Fatalf("Get(700) returned error: %v", err)
	}
	if len(reused) != 700 {
		t.Fatalf("len(Get(700)) = %d, want 700", len(reused))
	}
	if cap(reused) < KiB.Int() {
		t.Fatalf("cap(Get(700)) = %d, want at least 1 KiB", cap(reused))
	}

	missed, err := pool.Get(700)
	if err != nil {
		t.Fatalf("miss Get(700) returned error: %v", err)
	}
	if cap(missed) != KiB.Int() {
		t.Fatalf("cap(miss Get(700)) = %d, want 1 KiB", cap(missed))
	}

	afterAllocation := pool.Snapshot()
	assertPoolSnapshotRetainedConsistency(t, afterAllocation)
	class1KiB := mustPoolClassSnapshot(t, afterAllocation, ClassSizeFromSize(KiB))
	if class1KiB.Counters.Allocations != 1 || class1KiB.Counters.AllocatedBytes != KiB.Bytes() {
		t.Fatalf("1 KiB allocation counters = %d/%d, want 1/%d",
			class1KiB.Counters.Allocations,
			class1KiB.Counters.AllocatedBytes,
			KiB.Bytes(),
		)
	}

	assertPoolPanic(t, errClassStateAllocationBelowClassSize, func() {
		oneKiBState.recordAllocation(0, 512)
	})
	assertPoolPanic(t, errClassStateInvalidAllocationCapacity, func() {
		oneKiBState.recordAllocation(0, 0)
	})

	creditPool := MustNew(PoolConfig{Policy: poolTestCreditExhaustionPolicy()})
	defer closePoolForTest(t, creditPool)

	if err := creditPool.Put(make([]byte, 0, 512)); err != nil {
		t.Fatalf("credit test first Put() returned error: %v", err)
	}
	if err := creditPool.Put(make([]byte, 0, 512)); err != nil {
		t.Fatalf("credit test second Put() returned error: %v", err)
	}

	creditSnapshot := creditPool.Snapshot()
	assertPoolSnapshotRetainedConsistency(t, creditSnapshot)
	if creditSnapshot.Counters.Drops != 1 {
		t.Fatalf("credit exhausted drops = %d, want 1", creditSnapshot.Counters.Drops)
	}

	bucketPool := MustNew(PoolConfig{
		Policy:           poolTestBucketFullPolicy(),
		PolicyValidation: PoolPolicyValidationModeDisabled,
	})
	defer closePoolForTest(t, bucketPool)

	if err := bucketPool.Put(make([]byte, 0, 512)); err != nil {
		t.Fatalf("bucket test first Put() returned error: %v", err)
	}
	if err := bucketPool.Put(make([]byte, 0, 512)); err != nil {
		t.Fatalf("bucket test second Put() returned error: %v", err)
	}

	bucketSnapshot := bucketPool.Snapshot()
	assertPoolSnapshotRetainedConsistency(t, bucketSnapshot)
	if bucketSnapshot.Counters.Drops != 1 {
		t.Fatalf("bucket full drops = %d, want 1", bucketSnapshot.Counters.Drops)
	}

	trimClass := pool.mustClassStateFor(capacityClass)
	if err := pool.Put(make([]byte, 0, 700)); err != nil {
		t.Fatalf("Put before trim returned error: %v", err)
	}

	trimmed := trimClass.trimShard(0, 1)
	if trimmed.RemovedBuffers != 1 || trimmed.RemovedBytes == 0 {
		t.Fatalf("trim result = %d/%d, want one removed buffer", trimmed.RemovedBuffers, trimmed.RemovedBytes)
	}
	assertClassStateRetainedConsistency(t, trimClass.state())

	trimClass.clear()
	trimClass.tryRetain(0, make([]byte, 0, 700))
	trimClass.tryRetain(1, make([]byte, 0, 700))
	cleared := trimClass.clear()
	if cleared.RemovedBuffers != 2 {
		t.Fatalf("clear removed buffers = %d, want 2", cleared.RemovedBuffers)
	}
	if trimClass.state().CurrentRetainedBuffers != 0 {
		t.Fatal("class clear left retained buffers without concurrent mutation")
	}

	if err := pool.Put(make([]byte, 0, KiB.Int())); err != nil {
		t.Fatalf("Put before disableBudget returned error: %v", err)
	}
	oneKiBState.disableBudget()

	overBudgetSnapshot := pool.Snapshot()
	class1KiBAfterDisable := mustPoolClassSnapshot(t, overBudgetSnapshot, ClassSizeFromSize(KiB))
	if !class1KiBAfterDisable.IsOverBudget() {
		t.Fatal("disabled budget with retained storage did not report over budget")
	}
	assertPoolSnapshotRetainedConsistency(t, overBudgetSnapshot)
}

// poolTestIntegrationPolicy returns a compact three-class policy for end-to-end
// static runtime tests.
//
// The policy is intentionally larger than the small unit-test policy: it has
// multiple classes and two shards per class so routing, budget publication,
// shard selection, trim, and clear behavior can be exercised together.
func poolTestIntegrationPolicy() Policy {
	policy := poolTestSmallSingleShardPolicy()
	policy.Retention.SoftRetainedBytes = 6 * KiB
	policy.Retention.HardRetainedBytes = 16 * KiB
	policy.Retention.MaxRetainedBuffers = 12
	policy.Retention.MaxRetainedBufferCapacity = 2 * KiB
	policy.Retention.MaxClassRetainedBytes = 8 * KiB
	policy.Retention.MaxClassRetainedBuffers = 4
	policy.Retention.MaxShardRetainedBytes = 4 * KiB
	policy.Retention.MaxShardRetainedBuffers = 2
	policy.Classes.Sizes = []ClassSize{
		ClassSizeFromBytes(512),
		ClassSizeFromSize(KiB),
		ClassSizeFromSize(2 * KiB),
	}
	policy.Shards.ShardsPerClass = 2
	policy.Shards.BucketSlotsPerShard = 2
	policy.Shards.AcquisitionFallbackShards = 1
	policy.Shards.ReturnFallbackShards = 1

	return policy
}

// poolTestCreditExhaustionPolicy returns a policy where credit is smaller than
// physical bucket capacity.
//
// This lets tests prove that shard credit rejects retention before the bucket
// fills, and that the rejection is counted as a lower-layer drop rather than an
// owner-side drop reason.
func poolTestCreditExhaustionPolicy() Policy {
	policy := poolTestSmallSingleShardPolicy()
	policy.Shards.BucketSlotsPerShard = 2
	policy.Retention.MaxClassRetainedBuffers = 1
	policy.Retention.MaxShardRetainedBuffers = 1

	return policy
}

// poolTestBucketFullPolicy returns a policy where bucket slots are smaller than
// shard credit.
//
// Policy validation normally rejects this shape, so tests use it only with
// validation disabled to exercise the physical bucket-full path.
func poolTestBucketFullPolicy() Policy {
	policy := poolTestSmallSingleShardPolicy()
	policy.Shards.BucketSlotsPerShard = 1
	policy.Retention.MaxClassRetainedBuffers = 2
	policy.Retention.MaxShardRetainedBuffers = 2

	return policy
}

// mustPoolClassSnapshot returns the snapshot for size or fails the test.
//
// Tests use this helper when the class table itself is not under test and a
// missing class would mean the test fixture is invalid.
func mustPoolClassSnapshot(t *testing.T, snapshot PoolSnapshot, size ClassSize) PoolClassSnapshot {
	t.Helper()

	for _, class := range snapshot.Classes {
		if class.Class.Size() == size {
			return class
		}
	}

	t.Fatalf("snapshot has no class with size %s", size)
	return PoolClassSnapshot{}
}

// assertPoolSnapshotRetainedConsistency verifies derived retained-state
// invariants in a PoolSnapshot.
//
// The helper checks the whole aggregation chain:
//
//   - every shard bucket matches shard current retained counters;
//   - every class retained gauge equals the sum of its shards;
//   - pool retained gauges and aggregate counters equal the sum of classes.
func assertPoolSnapshotRetainedConsistency(t *testing.T, snapshot PoolSnapshot) {
	t.Helper()

	var classBuffers uint64
	var classBytes uint64
	for _, class := range snapshot.Classes {
		var shardBuffers uint64
		var shardBytes uint64
		for _, shard := range class.Shards {
			if uint64(shard.Bucket.RetainedBuffers) != shard.Counters.CurrentRetainedBuffers {
				t.Fatalf("shard bucket/counter retained buffers = %d/%d",
					shard.Bucket.RetainedBuffers,
					shard.Counters.CurrentRetainedBuffers,
				)
			}
			if shard.Bucket.RetainedBytes != shard.Counters.CurrentRetainedBytes {
				t.Fatalf("shard bucket/counter retained bytes = %d/%d",
					shard.Bucket.RetainedBytes,
					shard.Counters.CurrentRetainedBytes,
				)
			}

			shardBuffers += shard.Counters.CurrentRetainedBuffers
			shardBytes += shard.Counters.CurrentRetainedBytes
		}

		if class.CurrentRetainedBuffers != shardBuffers {
			t.Fatalf("class retained buffers = %d, shard sum = %d", class.CurrentRetainedBuffers, shardBuffers)
		}
		if class.CurrentRetainedBytes != shardBytes {
			t.Fatalf("class retained bytes = %d, shard sum = %d", class.CurrentRetainedBytes, shardBytes)
		}

		classBuffers += class.CurrentRetainedBuffers
		classBytes += class.CurrentRetainedBytes
	}

	if snapshot.CurrentRetainedBuffers != classBuffers {
		t.Fatalf("pool retained buffers = %d, class sum = %d", snapshot.CurrentRetainedBuffers, classBuffers)
	}
	if snapshot.CurrentRetainedBytes != classBytes {
		t.Fatalf("pool retained bytes = %d, class sum = %d", snapshot.CurrentRetainedBytes, classBytes)
	}
	if snapshot.Counters.CurrentRetainedBuffers != classBuffers {
		t.Fatalf("pool counter retained buffers = %d, class sum = %d", snapshot.Counters.CurrentRetainedBuffers, classBuffers)
	}
	if snapshot.Counters.CurrentRetainedBytes != classBytes {
		t.Fatalf("pool counter retained bytes = %d, class sum = %d", snapshot.Counters.CurrentRetainedBytes, classBytes)
	}
}
