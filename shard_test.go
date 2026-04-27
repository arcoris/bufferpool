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
	"sync/atomic"
	"testing"

	"arcoris.dev/bufferpool/internal/testutil"
)

// TestNewShard verifies construction of an enabled shard.
//
// A newly constructed shard owns enabled bucket storage but starts with disabled
// retention credit. This separates physical storage capacity from retention
// admission credit.
func TestNewShard(t *testing.T) {
	t.Parallel()

	shard := newShard(4)

	state := shard.state()

	if state.Bucket.SlotLimit != 4 {
		t.Fatalf("Bucket.SlotLimit = %d, want 4", state.Bucket.SlotLimit)
	}

	if state.Bucket.RetainedBuffers != 0 {
		t.Fatalf("Bucket.RetainedBuffers = %d, want 0", state.Bucket.RetainedBuffers)
	}

	if state.Bucket.RetainedBytes != 0 {
		t.Fatalf("Bucket.RetainedBytes = %d, want 0", state.Bucket.RetainedBytes)
	}

	if state.Bucket.AvailableSlots != 4 {
		t.Fatalf("Bucket.AvailableSlots = %d, want 4", state.Bucket.AvailableSlots)
	}

	if !state.Counters.IsZero() {
		t.Fatalf("Counters = %+v, want zero snapshot", state.Counters)
	}

	if !state.Credit.IsZero() {
		t.Fatalf("Credit = %+v, want disabled zero credit", state.Credit)
	}

	if state.Credit.IsEnabled() {
		t.Fatal("Credit.IsEnabled() = true, want false")
	}

	assertShardStateRetainedConsistency(t, state)
}

// TestNewShardPanicsForInvalidBucketSlotLimit verifies that shard construction
// preserves bucket construction invariants.
func TestNewShardPanicsForInvalidBucketSlotLimit(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errBucketInvalidSlotLimit, func() {
		_ = newShard(0)
	})
}

// TestShardTryGetMiss verifies miss accounting.
//
// shard.tryGet should record a routed get and a miss when bucket storage contains
// no retained buffer. The shard must not allocate on miss.
func TestShardTryGetMiss(t *testing.T) {
	t.Parallel()

	shard := newShard(1)

	result := shard.tryGet(512)

	if result.Hit {
		t.Fatal("tryGet().Hit = true, want false")
	}

	if !result.Miss() {
		t.Fatal("tryGet().Miss() = false, want true")
	}

	if result.Buffer != nil {
		t.Fatalf("tryGet().Buffer = %v, want nil", result.Buffer)
	}

	if result.Capacity != 0 {
		t.Fatalf("tryGet().Capacity = %d, want 0", result.Capacity)
	}

	counters := shard.countersSnapshot()

	if counters.Gets != 1 {
		t.Fatalf("Gets = %d, want 1", counters.Gets)
	}

	if counters.RequestedBytes != 512 {
		t.Fatalf("RequestedBytes = %d, want 512", counters.RequestedBytes)
	}

	if counters.Misses != 1 {
		t.Fatalf("Misses = %d, want 1", counters.Misses)
	}

	if counters.Hits != 0 {
		t.Fatalf("Hits = %d, want 0", counters.Hits)
	}

	if counters.Allocations != 0 {
		t.Fatalf("Allocations = %d, want 0", counters.Allocations)
	}
}

// TestShardTryRetainRejectsWhenCreditDisabled verifies default credit behavior.
//
// A shard starts with disabled credit. Physical bucket capacity exists, but
// return-path retention must be rejected until credit is explicitly published.
func TestShardTryRetainRejectsWhenCreditDisabled(t *testing.T) {
	t.Parallel()

	shard := newShard(1)

	result := shard.tryRetain(make([]byte, 0, 1024))

	if result.Retained {
		t.Fatal("tryRetain().Retained = true, want false")
	}

	if !result.Dropped() {
		t.Fatal("tryRetain().Dropped() = false, want true")
	}

	if !result.RejectedByCredit() {
		t.Fatal("tryRetain().RejectedByCredit() = false, want true")
	}

	if result.RejectedByBucket() {
		t.Fatal("tryRetain().RejectedByBucket() = true, want false")
	}

	if result.CreditDecision != shardCreditRejectNoCredit {
		t.Fatalf("CreditDecision = %s, want %s", result.CreditDecision, shardCreditRejectNoCredit)
	}

	if result.Capacity != 1024 {
		t.Fatalf("Capacity = %d, want 1024", result.Capacity)
	}

	state := shard.state()

	if state.Bucket.RetainedBuffers != 0 {
		t.Fatalf("Bucket.RetainedBuffers = %d, want 0", state.Bucket.RetainedBuffers)
	}

	if state.Counters.Puts != 1 {
		t.Fatalf("Puts = %d, want 1", state.Counters.Puts)
	}

	if state.Counters.ReturnedBytes != 1024 {
		t.Fatalf("ReturnedBytes = %d, want 1024", state.Counters.ReturnedBytes)
	}

	if state.Counters.Drops != 1 {
		t.Fatalf("Drops = %d, want 1", state.Counters.Drops)
	}

	if state.Counters.DroppedBytes != 1024 {
		t.Fatalf("DroppedBytes = %d, want 1024", state.Counters.DroppedBytes)
	}

	assertShardStateRetainedConsistency(t, state)
}

// TestShardTryRetainRejectsZeroCapacity verifies candidate-capacity validation.
//
// Zero-capacity buffers cannot provide reusable storage. The rejection is made by
// the shard credit gate before bucket storage is attempted.
func TestShardTryRetainRejectsZeroCapacity(t *testing.T) {
	t.Parallel()

	shard := newShard(1)
	shard.updateCredit(newShardCreditLimit(1, 1024))

	result := shard.tryRetain(make([]byte, 0))

	if result.Retained {
		t.Fatal("tryRetain zero-capacity buffer Retained = true, want false")
	}

	if result.CreditDecision != shardCreditRejectInvalidCapacity {
		t.Fatalf("CreditDecision = %s, want %s", result.CreditDecision, shardCreditRejectInvalidCapacity)
	}

	if result.Capacity != 0 {
		t.Fatalf("Capacity = %d, want 0", result.Capacity)
	}

	state := shard.state()

	if state.Bucket.RetainedBuffers != 0 {
		t.Fatalf("Bucket.RetainedBuffers = %d, want 0", state.Bucket.RetainedBuffers)
	}

	if state.Counters.Puts != 1 {
		t.Fatalf("Puts = %d, want 1", state.Counters.Puts)
	}

	if state.Counters.Drops != 1 {
		t.Fatalf("Drops = %d, want 1", state.Counters.Drops)
	}

	assertShardStateRetainedConsistency(t, state)
}

// TestShardTryRetainStoresBuffer verifies successful shard-local retention.
//
// The shard should record the routed put, evaluate credit, physically push the
// buffer into the bucket, and update retained counters.
func TestShardTryRetainStoresBuffer(t *testing.T) {
	t.Parallel()

	shard := newShard(2)
	shard.updateCredit(newShardCreditLimit(2, 2048))

	result := shard.tryRetain(make([]byte, 7, 1024))

	if !result.Retained {
		t.Fatal("tryRetain().Retained = false, want true")
	}

	if result.Dropped() {
		t.Fatal("tryRetain().Dropped() = true, want false")
	}

	if result.RejectedByCredit() {
		t.Fatal("tryRetain().RejectedByCredit() = true, want false")
	}

	if result.RejectedByBucket() {
		t.Fatal("tryRetain().RejectedByBucket() = true, want false")
	}

	if result.CreditDecision != shardCreditAccept {
		t.Fatalf("CreditDecision = %s, want %s", result.CreditDecision, shardCreditAccept)
	}

	if result.Capacity != 1024 {
		t.Fatalf("Capacity = %d, want 1024", result.Capacity)
	}

	state := shard.state()

	if state.Bucket.RetainedBuffers != 1 {
		t.Fatalf("Bucket.RetainedBuffers = %d, want 1", state.Bucket.RetainedBuffers)
	}

	if state.Bucket.RetainedBytes != 1024 {
		t.Fatalf("Bucket.RetainedBytes = %d, want 1024", state.Bucket.RetainedBytes)
	}

	if state.Counters.Puts != 1 {
		t.Fatalf("Puts = %d, want 1", state.Counters.Puts)
	}

	if state.Counters.Retains != 1 {
		t.Fatalf("Retains = %d, want 1", state.Counters.Retains)
	}

	if state.Counters.RetainedBytes != 1024 {
		t.Fatalf("RetainedBytes = %d, want 1024", state.Counters.RetainedBytes)
	}

	if state.Counters.CurrentRetainedBuffers != 1 {
		t.Fatalf("CurrentRetainedBuffers = %d, want 1", state.Counters.CurrentRetainedBuffers)
	}

	if state.Counters.CurrentRetainedBytes != 1024 {
		t.Fatalf("CurrentRetainedBytes = %d, want 1024", state.Counters.CurrentRetainedBytes)
	}

	assertShardStateRetainedConsistency(t, state)
}

// TestShardTryRetainZeroesBeforePublication verifies retained-buffer clearing
// happens before a buffer enters bucket storage.
func TestShardTryRetainZeroesBeforePublication(t *testing.T) {
	t.Parallel()

	shard := newShard(1)
	shard.updateCredit(newShardCreditLimit(1, 1024))

	buffer := make([]byte, 1024)
	buffer[0] = 1
	buffer[1023] = 2

	result := shard.tryRetainWithOptions(buffer[:1], shardRetainOptions{
		ZeroBeforeRetain: true,
	})
	if !result.Retained {
		t.Fatalf("tryRetainWithOptions Retained = false, decision=%s", result.CreditDecision)
	}

	reused := shard.tryGet(SizeFromInt(512))
	if !reused.Hit {
		t.Fatal("tryGet() missed retained buffer")
	}

	full := reused.Buffer[:cap(reused.Buffer)]
	if full[0] != 0 || full[1023] != 0 {
		t.Fatalf("retained buffer was not zeroed before publication: first=%d last=%d", full[0], full[1023])
	}
}

// TestShardTryRetainRejectsWhenBucketIsFull verifies physical storage rejection.
//
// Credit can allow retention while bucket storage rejects the candidate because
// local slot storage is full. This is intentionally distinct from credit
// rejection.
func TestShardTryRetainRejectsWhenBucketIsFull(t *testing.T) {
	t.Parallel()

	shard := newShard(1)
	shard.updateCredit(newShardCreditLimit(2, 2048))

	first := shard.tryRetain(make([]byte, 0, 512))
	if !first.Retained {
		t.Fatalf("first tryRetain Retained = false, want true; decision=%s", first.CreditDecision)
	}

	second := shard.tryRetain(make([]byte, 0, 512))

	if second.Retained {
		t.Fatal("second tryRetain Retained = true, want false")
	}

	if second.RejectedByCredit() {
		t.Fatal("second RejectedByCredit() = true, want false")
	}

	if !second.RejectedByBucket() {
		t.Fatal("second RejectedByBucket() = false, want true")
	}

	if second.CreditDecision != shardCreditAccept {
		t.Fatalf("second CreditDecision = %s, want %s", second.CreditDecision, shardCreditAccept)
	}

	state := shard.state()

	if state.Bucket.RetainedBuffers != 1 {
		t.Fatalf("Bucket.RetainedBuffers = %d, want 1", state.Bucket.RetainedBuffers)
	}

	if state.Counters.Retains != 1 {
		t.Fatalf("Retains = %d, want 1", state.Counters.Retains)
	}

	if state.Counters.Drops != 1 {
		t.Fatalf("Drops = %d, want 1", state.Counters.Drops)
	}

	if state.Counters.CurrentRetainedBuffers != 1 {
		t.Fatalf("CurrentRetainedBuffers = %d, want 1", state.Counters.CurrentRetainedBuffers)
	}

	assertShardStateRetainedConsistency(t, state)
}

// TestShardTryRetainRejectsWhenCreditExhausted verifies credit rejection.
//
// Credit exhaustion should reject retention before bucket storage is attempted.
func TestShardTryRetainRejectsWhenCreditExhausted(t *testing.T) {
	t.Parallel()

	shard := newShard(4)
	shard.updateCredit(newShardCreditLimit(1, 2048))

	first := shard.tryRetain(make([]byte, 0, 512))
	if !first.Retained {
		t.Fatalf("first tryRetain Retained = false, want true; decision=%s", first.CreditDecision)
	}

	second := shard.tryRetain(make([]byte, 0, 512))

	if second.Retained {
		t.Fatal("second tryRetain Retained = true, want false")
	}

	if !second.RejectedByCredit() {
		t.Fatal("second RejectedByCredit() = false, want true")
	}

	if second.RejectedByBucket() {
		t.Fatal("second RejectedByBucket() = true, want false")
	}

	if second.CreditDecision != shardCreditRejectBufferCreditExhausted {
		t.Fatalf("second CreditDecision = %s, want %s", second.CreditDecision, shardCreditRejectBufferCreditExhausted)
	}

	state := shard.state()

	if state.Bucket.RetainedBuffers != 1 {
		t.Fatalf("Bucket.RetainedBuffers = %d, want 1", state.Bucket.RetainedBuffers)
	}

	if state.Counters.Retains != 1 {
		t.Fatalf("Retains = %d, want 1", state.Counters.Retains)
	}

	if state.Counters.Drops != 1 {
		t.Fatalf("Drops = %d, want 1", state.Counters.Drops)
	}

	assertShardStateRetainedConsistency(t, state)
}

// TestShardTryGetHit verifies successful reuse from retained storage.
//
// A hit should remove a retained buffer from the bucket and decrement current
// retained gauges through shard counters.
func TestShardTryGetHit(t *testing.T) {
	t.Parallel()

	shard := newShard(2)
	shard.updateCredit(newShardCreditLimit(2, 2048))

	retain := shard.tryRetain(make([]byte, 5, 1024))
	if !retain.Retained {
		t.Fatalf("tryRetain Retained = false, want true; decision=%s", retain.CreditDecision)
	}

	result := shard.tryGet(256)

	if !result.Hit {
		t.Fatal("tryGet().Hit = false, want true")
	}

	if result.Miss() {
		t.Fatal("tryGet().Miss() = true, want false")
	}

	if result.Buffer == nil {
		t.Fatal("tryGet().Buffer = nil, want buffer")
	}

	if len(result.Buffer) != 0 {
		t.Fatalf("len(Buffer) = %d, want 0", len(result.Buffer))
	}

	if cap(result.Buffer) != 1024 {
		t.Fatalf("cap(Buffer) = %d, want 1024", cap(result.Buffer))
	}

	if result.Capacity != 1024 {
		t.Fatalf("Capacity = %d, want 1024", result.Capacity)
	}

	state := shard.state()

	if state.Bucket.RetainedBuffers != 0 {
		t.Fatalf("Bucket.RetainedBuffers = %d, want 0", state.Bucket.RetainedBuffers)
	}

	if state.Counters.Gets != 1 {
		t.Fatalf("Gets = %d, want 1", state.Counters.Gets)
	}

	if state.Counters.RequestedBytes != 256 {
		t.Fatalf("RequestedBytes = %d, want 256", state.Counters.RequestedBytes)
	}

	if state.Counters.Hits != 1 {
		t.Fatalf("Hits = %d, want 1", state.Counters.Hits)
	}

	if state.Counters.HitBytes != 1024 {
		t.Fatalf("HitBytes = %d, want 1024", state.Counters.HitBytes)
	}

	if state.Counters.CurrentRetainedBuffers != 0 {
		t.Fatalf("CurrentRetainedBuffers = %d, want 0", state.Counters.CurrentRetainedBuffers)
	}

	if state.Counters.CurrentRetainedBytes != 0 {
		t.Fatalf("CurrentRetainedBytes = %d, want 0", state.Counters.CurrentRetainedBytes)
	}

	assertShardStateRetainedConsistency(t, state)
}

// TestShardRecordAllocation verifies allocation accounting.
//
// The shard does not allocate buffers itself. Its owner records allocation after
// a miss or bypass path allocates a new backing buffer.
func TestShardRecordAllocation(t *testing.T) {
	t.Parallel()

	shard := newShard(1)

	shard.recordAllocation(1024)
	shard.recordAllocatedBuffer(make([]byte, 0, 2048))

	counters := shard.countersSnapshot()

	if counters.Allocations != 2 {
		t.Fatalf("Allocations = %d, want 2", counters.Allocations)
	}

	if counters.AllocatedBytes != 3072 {
		t.Fatalf("AllocatedBytes = %d, want 3072", counters.AllocatedBytes)
	}
}

func TestShardRecordAllocationPanicsForZeroCapacity(t *testing.T) {
	t.Parallel()

	shard := newShard(1)

	testutil.MustPanicWithMessage(t, errShardInvalidAllocationCapacity, func() {
		shard.recordAllocation(0)
	})

	state := shard.state()
	if !state.Counters.IsZero() {
		t.Fatalf("Counters after panic = %+v, want zero", state.Counters)
	}

	assertShardStateRetainedConsistency(t, state)
}

func TestShardRecordAllocatedBufferPanicsForZeroCapacity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		buffer []byte
	}{
		{
			name:   "nil",
			buffer: nil,
		},
		{
			name:   "zero capacity",
			buffer: make([]byte, 0),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			shard := newShard(1)

			testutil.MustPanicWithMessage(t, errShardInvalidAllocationCapacity, func() {
				shard.recordAllocatedBuffer(tt.buffer)
			})

			state := shard.state()
			if !state.Counters.IsZero() {
				t.Fatalf("Counters after panic = %+v, want zero", state.Counters)
			}

			assertShardStateRetainedConsistency(t, state)
		})
	}
}

// TestShardTrim verifies trim accounting and physical removal.
//
// shard.trim delegates physical removal to bucket.trim and records trim counters
// afterward.
func TestShardTrim(t *testing.T) {
	t.Parallel()

	shard := newShard(4)
	shard.updateCredit(newShardCreditLimit(4, 4096))

	for _, capacity := range []int{512, 1024, 2048} {
		result := shard.tryRetain(make([]byte, 0, capacity))
		if !result.Retained {
			t.Fatalf("tryRetain(%d) Retained = false, decision=%s", capacity, result.CreditDecision)
		}
	}

	result := shard.trim(2)

	if result.RemovedBuffers != 2 {
		t.Fatalf("RemovedBuffers = %d, want 2", result.RemovedBuffers)
	}

	if result.RemovedBytes != 3072 {
		t.Fatalf("RemovedBytes = %d, want 3072", result.RemovedBytes)
	}

	state := shard.state()

	if state.Bucket.RetainedBuffers != 1 {
		t.Fatalf("Bucket.RetainedBuffers = %d, want 1", state.Bucket.RetainedBuffers)
	}

	if state.Bucket.RetainedBytes != 512 {
		t.Fatalf("Bucket.RetainedBytes = %d, want 512", state.Bucket.RetainedBytes)
	}

	if state.Counters.TrimOperations != 1 {
		t.Fatalf("TrimOperations = %d, want 1", state.Counters.TrimOperations)
	}

	if state.Counters.TrimmedBuffers != 2 {
		t.Fatalf("TrimmedBuffers = %d, want 2", state.Counters.TrimmedBuffers)
	}

	if state.Counters.TrimmedBytes != 3072 {
		t.Fatalf("TrimmedBytes = %d, want 3072", state.Counters.TrimmedBytes)
	}

	if state.Counters.CurrentRetainedBuffers != 1 {
		t.Fatalf("CurrentRetainedBuffers = %d, want 1", state.Counters.CurrentRetainedBuffers)
	}

	if state.Counters.CurrentRetainedBytes != 512 {
		t.Fatalf("CurrentRetainedBytes = %d, want 512", state.Counters.CurrentRetainedBytes)
	}

	assertShardStateRetainedConsistency(t, state)
}

// TestShardTrimZeroLimit verifies zero-limit trim accounting.
//
// A zero trim limit should remove nothing but still record a trim operation at
// shard-counter level.
func TestShardTrimZeroLimit(t *testing.T) {
	t.Parallel()

	shard := newShard(1)
	shard.updateCredit(newShardCreditLimit(1, 1024))

	retain := shard.tryRetain(make([]byte, 0, 1024))
	if !retain.Retained {
		t.Fatalf("tryRetain Retained = false, decision=%s", retain.CreditDecision)
	}

	result := shard.trim(0)

	if !result.IsZero() {
		t.Fatalf("trim(0) result = %+v, want zero removal", result)
	}

	state := shard.state()

	if state.Bucket.RetainedBuffers != 1 {
		t.Fatalf("Bucket.RetainedBuffers = %d, want 1", state.Bucket.RetainedBuffers)
	}

	if state.Counters.TrimOperations != 1 {
		t.Fatalf("TrimOperations = %d, want 1", state.Counters.TrimOperations)
	}

	if state.Counters.TrimmedBuffers != 0 {
		t.Fatalf("TrimmedBuffers = %d, want 0", state.Counters.TrimmedBuffers)
	}

	if state.Counters.CurrentRetainedBuffers != 1 {
		t.Fatalf("CurrentRetainedBuffers = %d, want 1", state.Counters.CurrentRetainedBuffers)
	}

	assertShardStateRetainedConsistency(t, state)
}

// TestShardClear verifies clear accounting and physical removal.
//
// clear removes all retained buffers, records clear counters, and leaves credit
// unchanged.
func TestShardClear(t *testing.T) {
	t.Parallel()

	shard := newShard(4)
	shard.updateCredit(newShardCreditLimit(4, 4096))
	beforeCredit := shard.creditSnapshot()

	for _, capacity := range []int{512, 1024, 2048} {
		result := shard.tryRetain(make([]byte, 0, capacity))
		if !result.Retained {
			t.Fatalf("tryRetain(%d) Retained = false, decision=%s", capacity, result.CreditDecision)
		}
	}

	result := shard.clear()

	if result.RemovedBuffers != 3 {
		t.Fatalf("RemovedBuffers = %d, want 3", result.RemovedBuffers)
	}

	if result.RemovedBytes != 3584 {
		t.Fatalf("RemovedBytes = %d, want 3584", result.RemovedBytes)
	}

	state := shard.state()

	if state.Bucket.RetainedBuffers != 0 {
		t.Fatalf("Bucket.RetainedBuffers = %d, want 0", state.Bucket.RetainedBuffers)
	}

	if state.Bucket.RetainedBytes != 0 {
		t.Fatalf("Bucket.RetainedBytes = %d, want 0", state.Bucket.RetainedBytes)
	}

	if state.Counters.ClearOperations != 1 {
		t.Fatalf("ClearOperations = %d, want 1", state.Counters.ClearOperations)
	}

	if state.Counters.ClearedBuffers != 3 {
		t.Fatalf("ClearedBuffers = %d, want 3", state.Counters.ClearedBuffers)
	}

	if state.Counters.ClearedBytes != 3584 {
		t.Fatalf("ClearedBytes = %d, want 3584", state.Counters.ClearedBytes)
	}

	if state.Counters.CurrentRetainedBuffers != 0 {
		t.Fatalf("CurrentRetainedBuffers = %d, want 0", state.Counters.CurrentRetainedBuffers)
	}

	if state.Counters.CurrentRetainedBytes != 0 {
		t.Fatalf("CurrentRetainedBytes = %d, want 0", state.Counters.CurrentRetainedBytes)
	}

	afterCredit := state.Credit

	if afterCredit.TargetBuffers != beforeCredit.TargetBuffers {
		t.Fatalf("Credit.TargetBuffers after clear = %d, want %d", afterCredit.TargetBuffers, beforeCredit.TargetBuffers)
	}

	if afterCredit.TargetBytes != beforeCredit.TargetBytes {
		t.Fatalf("Credit.TargetBytes after clear = %d, want %d", afterCredit.TargetBytes, beforeCredit.TargetBytes)
	}

	if afterCredit.Generation != beforeCredit.Generation {
		t.Fatalf("Credit.Generation changed after clear: got %v, want %v", afterCredit.Generation, beforeCredit.Generation)
	}

	assertShardStateRetainedConsistency(t, state)
}

// TestShardUpdateAndDisableCredit verifies credit publication helpers.
func TestShardUpdateAndDisableCredit(t *testing.T) {
	t.Parallel()

	shard := newShard(1)

	initial := shard.creditSnapshot()

	generation := shard.updateCredit(newShardCreditLimit(2, 2048))
	enabled := shard.creditSnapshot()

	if enabled.Generation != generation {
		t.Fatalf("enabled Generation = %v, want %v", enabled.Generation, generation)
	}

	if enabled.Generation == initial.Generation {
		t.Fatalf("Generation did not advance: initial=%v enabled=%v", initial.Generation, enabled.Generation)
	}

	if enabled.TargetBuffers != 2 {
		t.Fatalf("TargetBuffers = %d, want 2", enabled.TargetBuffers)
	}

	if enabled.TargetBytes != 2048 {
		t.Fatalf("TargetBytes = %d, want 2048", enabled.TargetBytes)
	}

	if !enabled.IsEnabled() {
		t.Fatal("enabled IsEnabled() = false, want true")
	}

	disabledGeneration := shard.disableCredit()
	disabled := shard.creditSnapshot()

	if disabled.Generation != disabledGeneration {
		t.Fatalf("disabled Generation = %v, want %v", disabled.Generation, disabledGeneration)
	}

	if disabled.Generation == enabled.Generation {
		t.Fatalf("disable did not advance generation: enabled=%v disabled=%v", enabled.Generation, disabled.Generation)
	}

	if !disabled.IsZero() {
		t.Fatalf("disabled IsZero() = false, snapshot=%+v", disabled)
	}
}

// TestShardStateAccessors verifies direct state helper methods.
func TestShardStateAccessors(t *testing.T) {
	t.Parallel()

	shard := newShard(2)
	shard.updateCredit(newShardCreditLimit(2, 2048))

	retain := shard.tryRetain(make([]byte, 0, 1024))
	if !retain.Retained {
		t.Fatalf("tryRetain Retained = false, decision=%s", retain.CreditDecision)
	}

	bucketState := shard.bucketState()
	if bucketState.RetainedBuffers != 1 {
		t.Fatalf("bucketState.RetainedBuffers = %d, want 1", bucketState.RetainedBuffers)
	}

	counters := shard.countersSnapshot()
	if counters.Retains != 1 {
		t.Fatalf("countersSnapshot.Retains = %d, want 1", counters.Retains)
	}

	credit := shard.creditSnapshot()
	if credit.TargetBuffers != 2 {
		t.Fatalf("creditSnapshot.TargetBuffers = %d, want 2", credit.TargetBuffers)
	}

	usage := shard.creditUsage()
	if usage.RetainedBuffers != 1 {
		t.Fatalf("creditUsage.RetainedBuffers = %d, want 1", usage.RetainedBuffers)
	}

	if usage.RetainedBytes != 1024 {
		t.Fatalf("creditUsage.RetainedBytes = %d, want 1024", usage.RetainedBytes)
	}
}

// TestShardStateRetainedUsage verifies retained usage derivation from counters.
func TestShardStateRetainedUsage(t *testing.T) {
	t.Parallel()

	state := shardState{
		Counters: shardCountersSnapshot{
			CurrentRetainedBuffers: 3,
			CurrentRetainedBytes:   4096,
		},
	}

	usage := state.RetainedUsage()

	if usage.RetainedBuffers != 3 {
		t.Fatalf("RetainedUsage().RetainedBuffers = %d, want 3", usage.RetainedBuffers)
	}

	if usage.RetainedBytes != 4096 {
		t.Fatalf("RetainedUsage().RetainedBytes = %d, want 4096", usage.RetainedBytes)
	}
}

// TestShardStateIsOverCredit verifies over-credit detection.
func TestShardStateIsOverCredit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		state shardState

		want bool
	}{
		{
			name: "within enabled credit",
			state: shardState{
				Counters: shardCountersSnapshot{
					CurrentRetainedBuffers: 1,
					CurrentRetainedBytes:   1024,
				},
				Credit: shardCreditSnapshot{
					TargetBuffers: 2,
					TargetBytes:   2048,
				},
			},
			want: false,
		},
		{
			name: "over buffer credit",
			state: shardState{
				Counters: shardCountersSnapshot{
					CurrentRetainedBuffers: 3,
					CurrentRetainedBytes:   1024,
				},
				Credit: shardCreditSnapshot{
					TargetBuffers: 2,
					TargetBytes:   2048,
				},
			},
			want: true,
		},
		{
			name: "over byte credit",
			state: shardState{
				Counters: shardCountersSnapshot{
					CurrentRetainedBuffers: 1,
					CurrentRetainedBytes:   4096,
				},
				Credit: shardCreditSnapshot{
					TargetBuffers: 2,
					TargetBytes:   2048,
				},
			},
			want: true,
		},
		{
			name: "disabled credit with retained usage",
			state: shardState{
				Counters: shardCountersSnapshot{
					CurrentRetainedBuffers: 1,
					CurrentRetainedBytes:   1024,
				},
				Credit: shardCreditSnapshot{},
			},
			want: true,
		},
		{
			name: "disabled credit without retained usage",
			state: shardState{
				Counters: shardCountersSnapshot{},
				Credit:   shardCreditSnapshot{},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.state.IsOverCredit(); got != tt.want {
				t.Fatalf("IsOverCredit() = %t, want %t", got, tt.want)
			}
		})
	}
}

// TestShardBufferCapacity verifies local capacity extraction.
func TestShardBufferCapacity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		buffer []byte
		want   uint64
	}{
		{
			name:   "nil",
			buffer: nil,
			want:   0,
		},
		{
			name:   "zero capacity",
			buffer: make([]byte, 0),
			want:   0,
		},
		{
			name:   "non-zero capacity",
			buffer: make([]byte, 3, 128),
			want:   128,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := shardBufferCapacity(tt.buffer)
			if got != tt.want {
				t.Fatalf("shardBufferCapacity() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestShardConcurrentGetAndRetain verifies shard component synchronization under
// concurrent use.
//
// The credit target is intentionally large enough to avoid credit contention.
// This test checks that bucket locking and atomic counters do not lose updates.
func TestShardConcurrentGetAndRetain(t *testing.T) {
	t.Parallel()

	const workers = 8
	const iterations = 128
	const total = workers * iterations

	shard := newShard(total)
	shard.updateCredit(newShardCreditLimit(uint64(total), uint64(total*64)))

	var retainWG sync.WaitGroup
	retainWG.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer retainWG.Done()

			for j := 0; j < iterations; j++ {
				result := shard.tryRetain(make([]byte, 0, 64))
				if !result.Retained {
					t.Errorf("tryRetain Retained = false, decision=%s", result.CreditDecision)
				}
			}
		}()
	}

	retainWG.Wait()

	afterRetain := shard.state()

	if afterRetain.Counters.Retains != uint64(total) {
		t.Fatalf("Retains after concurrent retain = %d, want %d", afterRetain.Counters.Retains, total)
	}

	if afterRetain.Counters.CurrentRetainedBuffers != uint64(total) {
		t.Fatalf("CurrentRetainedBuffers after concurrent retain = %d, want %d", afterRetain.Counters.CurrentRetainedBuffers, total)
	}

	assertShardStateRetainedConsistency(t, afterRetain)

	var getWG sync.WaitGroup
	getWG.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer getWG.Done()

			for j := 0; j < iterations; j++ {
				result := shard.tryGet(32)
				if !result.Hit {
					t.Error("tryGet Hit = false, want true")
				}
			}
		}()
	}

	getWG.Wait()

	afterGet := shard.state()

	if afterGet.Counters.Gets != uint64(total) {
		t.Fatalf("Gets after concurrent get = %d, want %d", afterGet.Counters.Gets, total)
	}

	if afterGet.Counters.Hits != uint64(total) {
		t.Fatalf("Hits after concurrent get = %d, want %d", afterGet.Counters.Hits, total)
	}

	if afterGet.Counters.CurrentRetainedBuffers != 0 {
		t.Fatalf("CurrentRetainedBuffers after concurrent get = %d, want 0", afterGet.Counters.CurrentRetainedBuffers)
	}

	if afterGet.Bucket.RetainedBuffers != 0 {
		t.Fatalf("Bucket.RetainedBuffers after concurrent get = %d, want 0", afterGet.Bucket.RetainedBuffers)
	}

	assertShardStateRetainedConsistency(t, afterGet)
}

// TestShardConcurrentRetainDoesNotExceedCredit verifies that credit evaluation
// and bucket push are strict for concurrent retain calls.
func TestShardConcurrentRetainDoesNotExceedCredit(t *testing.T) {
	t.Parallel()

	const attempts = 64

	shard := newShard(attempts)
	shard.updateCredit(newShardCreditLimit(1, 1024))

	start := make(chan struct{})
	var wg sync.WaitGroup
	var retained atomic.Uint64

	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			defer wg.Done()

			<-start

			result := shard.tryRetain(make([]byte, 0, 1024))
			if result.Retained {
				retained.Add(1)
			}
		}()
	}

	close(start)
	wg.Wait()

	state := shard.state()
	retainedCount := retained.Load()

	if retainedCount > 1 {
		t.Fatalf("retained attempts = %d, want <= 1", retainedCount)
	}

	if state.Counters.CurrentRetainedBuffers > 1 {
		t.Fatalf("CurrentRetainedBuffers = %d, want <= 1", state.Counters.CurrentRetainedBuffers)
	}

	if state.Counters.CurrentRetainedBytes > 1024 {
		t.Fatalf("CurrentRetainedBytes = %d, want <= 1024", state.Counters.CurrentRetainedBytes)
	}

	if state.Bucket.RetainedBuffers > 1 {
		t.Fatalf("Bucket.RetainedBuffers = %d, want <= 1", state.Bucket.RetainedBuffers)
	}

	if state.Bucket.RetainedBytes > 1024 {
		t.Fatalf("Bucket.RetainedBytes = %d, want <= 1024", state.Bucket.RetainedBytes)
	}

	if state.Counters.Puts != attempts {
		t.Fatalf("Puts = %d, want %d", state.Counters.Puts, attempts)
	}

	if state.Counters.Retains != retainedCount {
		t.Fatalf("Retains = %d, want %d", state.Counters.Retains, retainedCount)
	}

	wantDrops := uint64(attempts) - retainedCount
	if state.Counters.Drops != wantDrops {
		t.Fatalf("Drops = %d, want %d", state.Counters.Drops, wantDrops)
	}

	assertShardStateRetainedConsistency(t, state)
}

func assertShardStateRetainedConsistency(t *testing.T, state shardState) {
	t.Helper()

	if state.Counters.CurrentRetainedBuffers != uint64(state.Bucket.RetainedBuffers) {
		t.Fatalf("CurrentRetainedBuffers = %d, Bucket.RetainedBuffers = %d",
			state.Counters.CurrentRetainedBuffers,
			state.Bucket.RetainedBuffers,
		)
	}

	if state.Counters.CurrentRetainedBytes != state.Bucket.RetainedBytes {
		t.Fatalf("CurrentRetainedBytes = %d, Bucket.RetainedBytes = %d",
			state.Counters.CurrentRetainedBytes,
			state.Bucket.RetainedBytes,
		)
	}
}
