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

import "sync"

// bucket is a synchronized local storage owner for one retained-buffer segment.
//
// bucket is the first concurrency boundary above bucketSegment. bucketSegment
// owns the raw LIFO slot storage and retained-byte accounting, but it is not
// goroutine-safe. bucket wraps that segment with a mutex so shard, class, or
// future pool storage code can safely perform local retain/reuse operations.
//
// Responsibility boundary:
//
//   - bucket owns synchronization;
//   - bucketSegment owns slot storage and local retained-byte accounting;
//   - admission code decides whether a returned buffer should be retained;
//   - class or shard code decides which bucket a buffer belongs to;
//   - bucket trim methods execute local storage reduction operations;
//   - counters and metrics code records operation outcomes;
//   - policy, pressure, trim planning, and controller code decide desired
//     retention behavior.
//
// bucket deliberately does not know about size classes, policies, pressure,
// ownership, leases, generations, clocks, metrics, trim reasons, or global
// budgets. It is a storage execution primitive, not a decision-making component.
//
// Storage behavior:
//
//   - tryPush stores an already admitted buffer if the segment can retain it;
//   - tryPop returns the most recently retained buffer;
//   - buffers are stored and returned with len == 0;
//   - retained memory is accounted by cap(buffer), not len(buffer).
//
// Concurrency:
//
// All methods lock the bucket mutex before accessing the segment. The lock is
// intentionally a plain sync.Mutex rather than sync.RWMutex because ordinary
// bucket operations are mutation-heavy: push and pop both change the segment.
// Read-only inspection is cheap and should not complicate the locking model.
//
// Copying:
//
// bucket MUST NOT be copied after first use. It contains a sync.Mutex and a
// bucketSegment with mutable slice-backed storage. Buckets should be constructed
// once, published to their owner, and then accessed in place.
//
// Zero value:
//
// The zero value is a disabled bucket. It is safe to inspect and pop from, but
// it has no storage slots and therefore rejects retention.
type bucket struct {
	mu sync.Mutex

	// segment stores retained buffers for this bucket.
	//
	// The segment is accessed only while mu is held.
	segment bucketSegment
}

// newBucket returns an enabled bucket with a segment containing slotLimit slots.
//
// slotLimit MUST be greater than zero. Disabled buckets can be represented by the
// zero value when a higher-level owner intentionally wants a bucket that rejects
// retention. Explicit construction through newBucket is for enabled buckets.
func newBucket(slotLimit int) bucket {
	return bucket{
		segment: newBucketSegment(slotLimit),
	}
}

// tryPush attempts to retain buffer in the bucket.
//
// The method returns true when the buffer was stored and false when the bucket
// cannot retain it. Rejection is intentionally non-diagnostic. Higher-level code
// must classify the reason, such as invalid buffer, admission denial, pressure
// drop, full bucket, or unsupported class.
//
// tryPush assumes the caller has already performed all higher-level checks:
//
//   - ownership or lease validation;
//   - class routing;
//   - policy admission;
//   - pressure admission;
//   - maximum retained capacity checks;
//   - optional sanitization decisions.
//
// The bucket only serializes access and delegates storage mechanics to
// bucketSegment.
func (b *bucket) tryPush(buffer []byte) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.segment.push(buffer)
}

// tryPop attempts to remove and return the most recently retained buffer.
//
// The method returns nil, false when the bucket is empty. A returned buffer has
// len == 0 and preserves its reusable backing capacity.
//
// The caller is responsible for reshaping the buffer to the requested length or
// class-specific length. bucket does not know the request size and must not
// perform request-level slicing.
func (b *bucket) tryPop() ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.segment.pop()
}

// state returns an immutable point-in-time view of local bucket storage state.
//
// The state is intended for shard/class inspection, tests, and future snapshot
// aggregation. It is not a public API snapshot and deliberately does not include
// policy, pressure, ownership, trim, or metrics fields.
func (b *bucket) state() bucketState {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.stateLocked()
}

// stateLocked returns the current bucket state.
//
// The caller MUST hold b.mu. Keeping the state assembly in one helper prevents
// future fields from drifting between state(), tests, and higher-level
// aggregation code.
func (b *bucket) stateLocked() bucketState {
	return bucketState{
		RetainedBuffers: b.segment.len(),
		RetainedBytes:   b.segment.retained(),
		SlotLimit:       b.segment.slotLimit(),
		AvailableSlots:  b.segment.availableSlots(),
	}
}

// len returns the number of buffers currently retained by the bucket.
func (b *bucket) len() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.segment.len()
}

// retained returns the sum of retained buffer capacities in bytes.
func (b *bucket) retained() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.segment.retained()
}

// slotLimit returns the maximum number of buffers this bucket can retain.
func (b *bucket) slotLimit() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.segment.slotLimit()
}

// availableSlots returns the number of free retained-buffer slots in the bucket.
func (b *bucket) availableSlots() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.segment.availableSlots()
}

// isEmpty reports whether the bucket currently retains no buffers.
func (b *bucket) isEmpty() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.segment.isEmpty()
}

// isFull reports whether the bucket cannot retain another buffer.
func (b *bucket) isFull() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.segment.isFull()
}

// bucketState is an immutable local view of bucket storage state.
//
// This is intentionally not named Snapshot. Public snapshots should be modeled
// at higher levels such as pool, partition, group, class, shard, or metrics
// files. bucketState is a small internal storage-state value.
type bucketState struct {
	// RetainedBuffers is the number of buffers currently stored in the bucket.
	RetainedBuffers int

	// RetainedBytes is the sum of cap(buffer) for retained buffers.
	RetainedBytes uint64

	// SlotLimit is the maximum number of buffers this bucket can retain.
	SlotLimit int

	// AvailableSlots is the number of remaining free slots.
	AvailableSlots int
}

// HasAvailableSlots reports whether the state can accept at least one more
// retained buffer from a storage-capacity perspective.
func (s bucketState) HasAvailableSlots() bool {
	return s.AvailableSlots > 0
}

// IsEmpty reports whether the state contains no retained buffers.
func (s bucketState) IsEmpty() bool {
	return s.RetainedBuffers == 0
}

// IsFull reports whether the state has no available slots.
//
// A disabled zero-value bucket has SlotLimit == 0 and AvailableSlots == 0, and
// is therefore considered full from the storage perspective.
func (s bucketState) IsFull() bool {
	return s.AvailableSlots == 0
}
