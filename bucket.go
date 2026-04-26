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
	// non-positive slot limit.
	errBucketInvalidSlotLimit = "bufferpool.bucket: slot limit must be greater than zero"

	// errBucketInvalidCount is used when the bucket count is outside the valid
	// slot range.
	errBucketInvalidCount = "bufferpool.bucket: count must be within slot bounds"

	// errBucketNonEmptyFreeSlot is used when a slot outside the occupied range is
	// not empty.
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

// bucket is a bounded LIFO storage primitive for reusable byte buffers.
//
// bucket is not goroutine-safe. The owning shard must serialize all access.
// bucket does not decide admission, policy, pressure, ownership, budget, or
// metrics. It only stores already admitted buffers and maintains local storage
// accounting.
//
// Storage behavior:
//
//   - push stores buffer[:0];
//   - pop returns the most recently pushed buffer;
//   - trim removes buffers from the same LIFO end;
//   - clear removes all retained buffers;
//   - every removed slot is cleared to nil;
//   - retained bytes are accounted by cap(buffer), not len(buffer).
//
// The zero value is a disabled bucket. It is safe to inspect, pop, trim, and
// clear. It rejects push because it has no storage slots.
//
// bucket MUST NOT be copied after first use. It contains a slice pointing to
// mutable slot storage.
type bucket struct {
	// slots stores retained buffers in stack order.
	//
	// Only slots in [0, count) are occupied. Slots in [count, len(slots)) must be
	// nil unless temporarily overwritten during a mutation.
	slots [][]byte

	// count is the number of occupied slots.
	count int

	// retainedBytes is the sum of cap(buffer) for all occupied slots.
	retainedBytes uint64
}

// newBucket returns an empty enabled bucket with slotLimit storage slots.
func newBucket(slotLimit int) bucket {
	if slotLimit <= 0 {
		panic(errBucketInvalidSlotLimit)
	}

	return bucket{
		slots: make([][]byte, slotLimit),
	}
}

// push attempts to retain buffer in the bucket.
//
// The method returns true when buffer was stored and false when it was rejected.
// Rejection is intentionally non-diagnostic because class and shard code own
// admission and drop classification.
func (b *bucket) push(buffer []byte) bool {
	b.mustHaveValidCount()

	if b.count == len(b.slots) {
		return false
	}

	if !bucketCanRetain(buffer) {
		return false
	}

	b.mustHaveFreeSlot(b.count)

	capacity := bucketBufferCapacity(buffer)
	b.addRetained(capacity)

	b.slots[b.count] = buffer[:0]
	b.count++

	return true
}

// pop removes and returns the most recently retained buffer.
func (b *bucket) pop() ([]byte, bool) {
	b.mustHaveValidCount()

	if b.count == 0 {
		return nil, false
	}

	index := b.count - 1
	buffer := b.mustHaveOccupiedSlot(index)
	capacity := bucketBufferCapacity(buffer)

	b.removeRetained(capacity)
	b.slots[index] = nil
	b.count--

	return buffer[:0], true
}

// trim removes up to maxBuffers retained buffers from the bucket.
//
// A zero maxBuffers is a valid no-op. A negative maxBuffers is an internal trim
// planning bug and causes a panic.
func (b *bucket) trim(maxBuffers int) bucketTrimResult {
	if maxBuffers < 0 {
		panic(errBucketNegativeTrimLimit)
	}

	b.mustHaveValidCount()

	limit := maxBuffers
	if limit > b.count {
		limit = b.count
	}

	var removedBuffers int
	var removedBytes uint64

	for removedBuffers < limit {
		buffer, _ := b.pop()

		removedBuffers++
		removedBytes += bucketBufferCapacity(buffer)
	}

	return newBucketTrimResult(removedBuffers, removedBytes, b.state())
}

// clear removes every retained buffer from the bucket.
func (b *bucket) clear() bucketTrimResult {
	b.mustHaveValidCount()

	return b.trim(b.count)
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

	return b.count
}

// retained returns the sum of retained buffer capacities in bytes.
func (b *bucket) retained() uint64 {
	b.mustHaveValidCount()

	return b.retainedBytes
}

// slotLimit returns the maximum number of buffers this bucket can retain.
func (b *bucket) slotLimit() int {
	return len(b.slots)
}

// availableSlots returns the number of free retained-buffer slots in the bucket.
func (b *bucket) availableSlots() int {
	b.mustHaveValidCount()

	return len(b.slots) - b.count
}

// isEmpty reports whether the bucket currently retains no buffers.
func (b *bucket) isEmpty() bool {
	b.mustHaveValidCount()

	return b.count == 0
}

// isFull reports whether the bucket cannot retain another buffer.
func (b *bucket) isFull() bool {
	b.mustHaveValidCount()

	return b.count == len(b.slots)
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

// mustHaveValidCount verifies the core slot-range invariant.
func (b *bucket) mustHaveValidCount() {
	if b.count < 0 || b.count > len(b.slots) {
		panic(errBucketInvalidCount)
	}
}

// mustHaveFreeSlot verifies that index is a free slot ready to be written.
func (b *bucket) mustHaveFreeSlot(index int) {
	if b.slots[index] != nil {
		panic(errBucketNonEmptyFreeSlot)
	}
}

// mustHaveOccupiedSlot returns the occupied buffer at index.
func (b *bucket) mustHaveOccupiedSlot(index int) []byte {
	buffer := b.slots[index]
	if !bucketCanRetain(buffer) {
		panic(errBucketEmptyOccupiedSlot)
	}

	return buffer
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
