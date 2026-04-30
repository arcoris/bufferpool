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

const (
	// errBucketInvalidSlotLimit is used when an enabled bucket is created with a
	// non-positive total slot limit.
	errBucketInvalidSlotLimit = "bufferpool.bucket: slot limit must be greater than zero"

	// errBucketInvalidSegmentSlotLimit is used when lazy segment metadata cannot
	// represent the bucket's total slot limit.
	errBucketInvalidSegmentSlotLimit = "bufferpool.bucket: segment slot limit must be within bucket slot bounds"

	// errBucketInvalidCount is used when bucket-level or segment-level retained
	// counts are outside their valid slot bounds.
	errBucketInvalidCount = "bufferpool.bucket: count must be within slot bounds"

	// errBucketNonEmptyFreeSlot is used when a slot that should be writable still
	// contains a retained buffer reference.
	errBucketNonEmptyFreeSlot = "bufferpool.bucket: free slot must be empty"

	// errBucketEmptyOccupiedSlot is used when an occupied slot does not contain
	// positive reusable capacity.
	errBucketEmptyOccupiedSlot = "bufferpool.bucket: occupied slot must contain positive capacity"

	// errBucketNegativeTrimLimit is used when trim is requested with a negative
	// buffer limit.
	errBucketNegativeTrimLimit = "bufferpool.bucket: trim limit must not be negative"

	// errBucketRetainedBytesOverflow is used when adding a buffer would overflow
	// retained byte accounting.
	errBucketRetainedBytesOverflow = "bufferpool.bucket: retained bytes overflow"

	// errBucketRetainedBytesUnderflow is used when removing a buffer would
	// underflow retained byte accounting.
	errBucketRetainedBytesUnderflow = "bufferpool.bucket: retained bytes underflow"
)

const maxBucketRetainedBytes = ^uint64(0)

// bucket is a bounded segmented LIFO storage primitive for reusable byte
// buffers.
//
// bucket is not goroutine-safe. The owning shard must serialize all access.
// bucket does not decide policy admission, shard credit, or owner-side result
// mapping. It stores already admitted buffers and maintains local storage
// accounting.
//
// Storage behavior:
//
//   - newBucket records limits only and does not allocate segment slot storage;
//   - push stores buffer[:0] in the current head segment;
//   - pop returns the most recently pushed buffer;
//   - trim removes buffers from the same LIFO end;
//   - clear removes all retained buffers and drops the segment chain;
//   - every removed slot is cleared to nil before a segment is released;
//   - retained bytes are accounted by cap(buffer), not len(buffer).
//
// The zero value is a disabled bucket. It is safe to inspect, pop, trim, and
// clear. It rejects push because it has no storage capacity.
//
// bucket MUST NOT be copied after first use. It contains a linked segment stack
// whose slots own retained buffer references.
type bucket struct {
	// head is the most recent bucket segment. Push and pop operate only here.
	head *bucketSegment

	// retainedBuffers is the total number of buffers stored across all segments.
	retainedBuffers int

	// retainedBytes is the sum of cap(buffer) for all occupied slots.
	retainedBytes uint64

	// slotLimitValue is the total number of buffers this bucket may retain.
	slotLimitValue int

	// segmentSlotLimitValue is the maximum number of slots allocated in one
	// segment.
	segmentSlotLimitValue int
}

// newBucket returns an empty enabled bucket with lazy segment allocation.
//
// slotLimit is the total retained-buffer capacity. segmentSlotLimit optionally
// overrides the lazy metadata allocation size; when omitted, the package default
// segment size is clamped to the bucket slot limit.
func newBucket(slotLimit int, segmentSlotLimit ...int) bucket {
	if slotLimit <= 0 {
		panic(errBucketInvalidSlotLimit)
	}

	resolvedSegmentSlotLimit := bucketSegmentSlotLimit(slotLimit, segmentSlotLimit...)
	if resolvedSegmentSlotLimit <= 0 || resolvedSegmentSlotLimit > slotLimit {
		panic(errBucketInvalidSegmentSlotLimit)
	}

	return bucket{
		slotLimitValue:        slotLimit,
		segmentSlotLimitValue: resolvedSegmentSlotLimit,
	}
}

// push attempts to retain buffer in the bucket.
//
// The method returns true when buffer was stored and false when it was rejected.
// Rejection is intentionally non-diagnostic because class and shard code own
// admission and drop classification.
func (b *bucket) push(buffer []byte) bool {
	b.mustHaveValidCount()

	if b.isFull() || !bucketCanRetain(buffer) {
		return false
	}

	segment := b.head
	if segment != nil {
		segment.mustHaveValidCount()
		if segment.isFull() {
			segment = nil
		} else {
			segment.mustHaveFreeSlot(segment.count)
		}
	}

	capacity := bucketBufferCapacity(buffer)
	b.addRetained(capacity)

	if segment == nil {
		segment = b.ensureHeadSegment()
		segment.mustHaveFreeSlot(segment.count)
	}

	segment.slots[segment.count] = buffer[:0]
	segment.count++
	b.retainedBuffers++

	return true
}

// pop removes and returns the most recently retained buffer.
func (b *bucket) pop() ([]byte, bool) {
	b.mustHaveValidCount()

	if b.retainedBuffers == 0 {
		return nil, false
	}

	segment := b.mustHaveHeadSegment()
	index := segment.count - 1
	buffer := segment.mustHaveOccupiedSlot(index)
	capacity := bucketBufferCapacity(buffer)

	b.removeRetained(capacity)
	segment.slots[index] = nil
	segment.count--
	b.retainedBuffers--
	b.releaseEmptyHeadSegment()

	return buffer[:0], true
}

// trim removes up to maxBuffers retained buffers from the bucket.
//
// A zero maxBuffers is a valid no-op. A negative maxBuffers is an internal trim
// planning bug and causes a panic.
func (b *bucket) trim(maxBuffers int) bucketTrimResult {
	return b.trimBounded(maxBuffers, maxBucketRetainedBytes)
}

// trimBounded removes retained buffers while respecting both buffer and byte
// limits.
//
// maxBuffers bounds operation count. maxBytes bounds the sum of removed buffer
// capacities; the next buffer is not removed when it would exceed maxBytes. The
// byte limit is useful for future cold trim planners without changing bucket's
// ordinary max-buffer trim API.
func (b *bucket) trimBounded(maxBuffers int, maxBytes uint64) bucketTrimResult {
	if maxBuffers < 0 {
		panic(errBucketNegativeTrimLimit)
	}

	b.mustHaveValidCount()

	limit := maxBuffers
	if limit > b.retainedBuffers {
		limit = b.retainedBuffers
	}

	var removedBuffers int
	var removedBytes uint64

	for removedBuffers < limit {
		capacity, ok := b.peekHeadCapacity()
		if !ok || capacity > maxBytes-removedBytes {
			break
		}

		buffer, _ := b.pop()
		removedBuffers++
		removedBytes += bucketBufferCapacity(buffer)
	}

	return newBucketTrimResult(removedBuffers, removedBytes, b.state())
}

// clear removes every retained buffer from the bucket.
//
// The method clears all occupied slots before dropping segment links so no
// segment can keep stale byte-slice references alive after clear returns.
func (b *bucket) clear() bucketTrimResult {
	b.mustHaveValidCount()

	removedBuffers := b.retainedBuffers
	removedBytes := b.retainedBytes

	for segment := b.head; segment != nil; segment = segment.previous {
		segment.clearOccupiedSlots()
	}

	b.head = nil
	b.retainedBuffers = 0
	b.retainedBytes = 0

	return newBucketTrimResult(removedBuffers, removedBytes, b.state())
}

// state returns the current bucket state.
func (b *bucket) state() bucketState {
	return bucketState{
		RetainedBuffers: b.len(),
		RetainedBytes:   b.retained(),
		SlotLimit:       b.slotLimit(),
		AvailableSlots:  b.availableSlots(),
	}
}

// len returns the number of buffers currently retained by the bucket.
func (b *bucket) len() int {
	b.mustHaveValidCount()

	return b.retainedBuffers
}

// retained returns the sum of retained buffer capacities in bytes.
func (b *bucket) retained() uint64 {
	b.mustHaveValidCount()

	return b.retainedBytes
}

// slotLimit returns the maximum number of buffers this bucket can retain.
func (b *bucket) slotLimit() int {
	return b.slotLimitValue
}

// segmentCount returns the number of allocated metadata segments.
//
// The method is intentionally narrow: production accounting uses retained
// buffer and byte counts, while tests and benchmarks use segment count to prove
// that bucket metadata is allocated lazily and released after pop or clear.
func (b *bucket) segmentCount() int {
	var count int
	for segment := b.head; segment != nil; segment = segment.previous {
		count++
	}

	return count
}

// availableSlots returns the number of free retained-buffer slots in the bucket.
func (b *bucket) availableSlots() int {
	b.mustHaveValidCount()

	return b.slotLimitValue - b.retainedBuffers
}

// isEmpty reports whether the bucket currently retains no buffers.
func (b *bucket) isEmpty() bool {
	b.mustHaveValidCount()

	return b.retainedBuffers == 0
}

// isFull reports whether the bucket cannot retain another buffer.
func (b *bucket) isFull() bool {
	b.mustHaveValidCount()

	return b.retainedBuffers == b.slotLimitValue
}

// ensureHeadSegment returns a writable head segment, allocating one lazily when
// the bucket is empty or the current head is full.
func (b *bucket) ensureHeadSegment() *bucketSegment {
	if b.head != nil {
		b.head.mustHaveValidCount()
		if !b.head.isFull() {
			return b.head
		}
	}

	segmentSlotLimit := b.nextSegmentSlotLimit()
	b.head = newBucketSegment(segmentSlotLimit, b.head)

	return b.head
}

// mustHaveHeadSegment returns the non-empty head segment for pop-like
// operations.
func (b *bucket) mustHaveHeadSegment() *bucketSegment {
	if b.head == nil {
		panic(errBucketInvalidCount)
	}

	b.head.mustHaveValidCount()
	if b.head.count == 0 {
		panic(errBucketInvalidCount)
	}

	return b.head
}

// releaseEmptyHeadSegment drops an empty head segment after pop.
//
// Empty segments are not cached. The bucket stays fully lazy: after the final
// pop, head becomes nil and no slot metadata remains retained by the bucket.
func (b *bucket) releaseEmptyHeadSegment() {
	if b.head == nil || b.head.count != 0 {
		return
	}

	b.head = b.head.previous
}

// nextSegmentSlotLimit computes the slot length for a newly allocated head
// segment.
//
// The last segment may be smaller than segmentSlotLimit when slotLimit is not a
// multiple of the segment size.
func (b *bucket) nextSegmentSlotLimit() int {
	remaining := b.slotLimitValue - b.retainedBuffers
	if remaining <= 0 {
		panic(errBucketInvalidCount)
	}

	if remaining < b.segmentSlotLimitValue {
		return remaining
	}

	return b.segmentSlotLimitValue
}

// peekHeadCapacity returns the capacity of the next buffer that pop would
// remove.
func (b *bucket) peekHeadCapacity() (uint64, bool) {
	if b.retainedBuffers == 0 {
		return 0, false
	}

	segment := b.mustHaveHeadSegment()
	buffer := segment.mustHaveOccupiedSlot(segment.count - 1)

	return bucketBufferCapacity(buffer), true
}

// addRetained records newly retained capacity.
func (b *bucket) addRetained(capacity uint64) {
	if b.retainedBytes > maxBucketRetainedBytes-capacity {
		panic(errBucketRetainedBytesOverflow)
	}

	b.retainedBytes += capacity
}

// removeRetained records removed retained capacity.
func (b *bucket) removeRetained(capacity uint64) {
	if b.retainedBytes < capacity {
		panic(errBucketRetainedBytesUnderflow)
	}

	b.retainedBytes -= capacity
}

// mustHaveValidCount verifies the bucket-level retained count invariant.
func (b *bucket) mustHaveValidCount() {
	if b.retainedBuffers < 0 || b.retainedBuffers > b.slotLimitValue {
		panic(errBucketInvalidCount)
	}
}

// bucketSegmentSlotLimit resolves optional segment sizing for newBucket.
func bucketSegmentSlotLimit(slotLimit int, values ...int) int {
	if len(values) > 0 {
		return values[0]
	}

	if slotLimit < DefaultPolicyBucketSegmentSlotsPerShard {
		return slotLimit
	}

	return DefaultPolicyBucketSegmentSlotsPerShard
}

// bucketCanRetain reports whether buffer has reusable backing capacity.
func bucketCanRetain(buffer []byte) bool {
	return bucketBufferCapacity(buffer) > 0
}

// bucketBufferCapacity returns the retained-memory cost of buffer.
func bucketBufferCapacity(buffer []byte) uint64 {
	return uint64(cap(buffer))
}

// bucketState is an immutable local view of bucket storage state.
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
func (s bucketState) IsFull() bool {
	return s.AvailableSlots == 0
}
