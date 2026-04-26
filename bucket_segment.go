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
	// errBucketSegmentInvalidSlotLimit is used when a bucket segment is created
	// with a non-positive slot limit.
	//
	// A segment with no storage capacity is not useful as a retained-buffer
	// container. Higher-level bucket construction must decide whether a disabled
	// bucket should be omitted entirely rather than represented as a zero-sized
	// segment.
	errBucketSegmentInvalidSlotLimit = "bufferpool.bucketSegment: slot limit must be greater than zero"

	// errBucketSegmentNegativeTrimLimit is used when trim is requested with a
	// negative buffer limit.
	//
	// A negative trim limit has no meaningful interpretation. A zero trim limit is
	// a valid no-op; a negative one indicates broken caller-side trim planning.
	errBucketSegmentNegativeTrimLimit = "bufferpool.bucketSegment: trim limit must not be negative"

	// errBucketSegmentRetainedBytesOverflow is used when adding a buffer would
	// overflow retained byte accounting.
	//
	// This should be practically unreachable with realistic segment limits, but
	// retained memory accounting is a correctness invariant and must not wrap.
	errBucketSegmentRetainedBytesOverflow = "bufferpool.bucketSegment: retained bytes overflow"

	// errBucketSegmentRetainedBytesUnderflow is used when removing a buffer would
	// underflow retained byte accounting.
	//
	// This indicates internal segment corruption: the segment is trying to remove
	// more retained capacity than it has recorded.
	errBucketSegmentRetainedBytesUnderflow = "bufferpool.bucketSegment: retained bytes underflow"

	// errBucketSegmentEmptyOccupiedSlot is used when an occupied slot does not
	// contain positive reusable capacity.
	//
	// push never creates such a slot. Seeing one during pop or trim indicates an
	// internal mutation bug, copied segment state, or manual test corruption.
	errBucketSegmentEmptyOccupiedSlot = "bufferpool.bucketSegment: occupied slot must contain positive capacity"
)

const maxBucketSegmentRetainedBytes = ^uint64(0)

// bucketSegment is a bounded LIFO storage segment for reusable byte buffers.
//
// A segment stores buffers that have already passed higher-level admission
// decisions. It does not decide whether a buffer should be retained. It only
// keeps admitted buffers in a compact slot array and maintains local retained
// byte accounting.
//
// The segment deliberately stores raw []byte values instead of ownership handles.
// Ownership, lease validation, size-class selection, pressure decisions, and
// trim policy belong to higher layers.
//
// Storage model:
//
//   - push stores buffer[:0];
//   - pop returns the most recently pushed buffer;
//   - trim removes buffers from the same LIFO end;
//   - every removed slot is cleared to nil so the backing array can be collected
//     when no longer referenced elsewhere.
//
// LIFO is intentional. Recently returned buffers are more likely to be warm in
// CPU caches and usually represent the most recent capacity shape for that
// bucket. The segment does not attempt fairness; fairness and adaptive retention
// are bucket/shard/controller responsibilities.
//
// Concurrency:
//
// bucketSegment is not goroutine-safe. The owner, usually bucket or shard code,
// MUST serialize access. This keeps the segment small and avoids embedding locks
// into a primitive that may be used under different synchronization strategies.
//
// Copying:
//
// bucketSegment MUST NOT be copied after first use. It contains a slice pointing
// to mutable slot storage. Copying it would alias the same backing array and
// split retained byte accounting across two segment values.
//
// Zero value:
//
// The zero value is an empty disabled segment. Inspection, pop, trim, and clear
// are safe. push rejects because the segment has no slots. Enabled segments
// should still be created through newBucketSegment so the intended slot limit is
// explicit.
type bucketSegment struct {
	// slots stores retained buffers in stack order.
	//
	// Only slots in [0, count) are considered occupied. Slots in [count,
	// len(slots)) must be nil unless temporarily overwritten during a mutation.
	slots [][]byte

	// count is the number of occupied slots.
	count int

	// retainedBytes is the sum of cap(buffer) for all occupied slots.
	retainedBytes uint64
}

// newBucketSegment returns an empty segment with slotLimit storage slots.
//
// slotLimit MUST be greater than zero. If a higher-level policy wants to disable
// retention for a bucket, it should avoid constructing the bucket or route all
// candidates to discard logic instead of creating a zero-sized segment.
func newBucketSegment(slotLimit int) bucketSegment {
	if slotLimit <= 0 {
		panic(errBucketSegmentInvalidSlotLimit)
	}

	return bucketSegment{
		slots: make([][]byte, slotLimit),
	}
}

// slotLimit returns the maximum number of buffers this segment can retain.
func (s *bucketSegment) slotLimit() int {
	return len(s.slots)
}

// len returns the number of buffers currently retained by this segment.
func (s *bucketSegment) len() int {
	return s.count
}

// availableSlots returns the number of free slots left in this segment.
func (s *bucketSegment) availableSlots() int {
	return len(s.slots) - s.count
}

// isEmpty reports whether the segment contains no retained buffers.
func (s *bucketSegment) isEmpty() bool {
	return s.count == 0
}

// isFull reports whether the segment cannot accept another retained buffer.
func (s *bucketSegment) isFull() bool {
	return s.count == len(s.slots)
}

// retained returns the sum of retained buffer capacities in bytes.
//
// The value is based on cap(buffer), not len(buffer), because retained memory is
// determined by backing-array capacity. Buffers are stored with len == 0, but
// their backing capacity still consumes memory.
func (s *bucketSegment) retained() uint64 {
	return s.retainedBytes
}

// push attempts to retain buffer in the segment.
//
// The method returns true when buffer was stored and false when it was rejected.
// Rejection is intentionally non-diagnostic because higher-level admission code
// should classify the reason before calling into the segment. Typical rejection
// reasons include full segment, nil buffer, or zero-capacity buffer.
//
// The stored slice always has length zero. This preserves reusable capacity while
// avoiding accidental retention of caller-visible contents through len.
//
// push does not copy buffer contents and does not clear the underlying array.
// Sanitization, if required by a future security policy, belongs to an explicit
// higher-level wipe/sanitize path.
func (s *bucketSegment) push(buffer []byte) bool {
	if s.isFull() {
		return false
	}

	if !bucketSegmentCanRetain(buffer) {
		return false
	}

	capacity := bucketSegmentBufferCapacity(buffer)
	s.addRetained(capacity)

	s.slots[s.count] = buffer[:0]
	s.count++

	return true
}

// pop removes and returns the most recently retained buffer.
//
// The returned buffer has length zero and preserves its backing capacity. If the
// segment is empty, pop returns nil, false.
//
// The vacated slot is cleared to nil to avoid retaining a backing array after it
// has left the segment.
func (s *bucketSegment) pop() ([]byte, bool) {
	if s.isEmpty() {
		return nil, false
	}

	index := s.count - 1
	buffer := s.slots[index]
	capacity := bucketSegmentBufferCapacity(buffer)

	if capacity == 0 {
		panic(errBucketSegmentEmptyOccupiedSlot)
	}

	s.removeRetained(capacity)

	s.slots[index] = nil
	s.count--

	return buffer[:0], true
}

// trim removes up to maxBuffers retained buffers from the segment.
//
// A zero maxBuffers is a valid no-op. A negative maxBuffers is an internal trim
// planning bug and causes a panic.
//
// trim removes from the same LIFO end as pop. This keeps the operation simple,
// constant per removed buffer, and consistent with segment storage order.
// Higher-level trim planning decides how many buffers to remove and which
// buckets or segments should be targeted.
func (s *bucketSegment) trim(maxBuffers int) bucketSegmentTrimResult {
	if maxBuffers < 0 {
		panic(errBucketSegmentNegativeTrimLimit)
	}

	var result bucketSegmentTrimResult
	limit := maxBuffers
	if limit > s.count {
		limit = s.count
	}

	for result.Buffers < limit {
		// limit is bounded by count, so pop cannot report an empty segment while
		// the segment state is internally consistent.
		buffer, _ := s.pop()

		result.Buffers++
		result.Bytes += uint64(cap(buffer))
	}

	return result
}

// clear removes every retained buffer from the segment.
//
// It returns the number of removed buffers and their total retained capacity.
// clear is useful for close paths, hard trims, test cleanup, and policy changes
// that invalidate all retained buffers in a bucket.
func (s *bucketSegment) clear() bucketSegmentTrimResult {
	return s.trim(s.count)
}

// addRetained records newly retained capacity.
//
// The caller must pass capacity > 0. Overflow is treated as an internal
// accounting violation because retained byte counters must never wrap.
func (s *bucketSegment) addRetained(capacity uint64) {
	if s.retainedBytes > maxBucketSegmentRetainedBytes-capacity {
		panic(errBucketSegmentRetainedBytesOverflow)
	}

	s.retainedBytes += capacity
}

// removeRetained records removed retained capacity.
//
// The caller must pass capacity > 0. Underflow is treated as internal
// corruption and is checked before slot state is mutated, so a failing pop does
// not partially remove the buffer from the segment.
func (s *bucketSegment) removeRetained(capacity uint64) {
	if s.retainedBytes < capacity {
		panic(errBucketSegmentRetainedBytesUnderflow)
	}

	s.retainedBytes -= capacity
}

// bucketSegmentCanRetain reports whether buffer has reusable backing capacity.
//
// Nil and zero-capacity slices are not useful retained storage entries. They do
// not contribute reusable capacity and would make retained-byte accounting
// ambiguous.
func bucketSegmentCanRetain(buffer []byte) bool {
	return cap(buffer) > 0
}

// bucketSegmentBufferCapacity returns the retained-memory cost of buffer.
func bucketSegmentBufferCapacity(buffer []byte) uint64 {
	return uint64(cap(buffer))
}

// bucketSegmentTrimResult describes buffers removed from a segment.
//
// The type is intentionally local to bucket segment mechanics. Higher-level trim
// files may aggregate these values into richer trim plans, trim results, metrics,
// and snapshots.
type bucketSegmentTrimResult struct {
	// Buffers is the number of buffers removed from the segment.
	Buffers int

	// Bytes is the sum of cap(buffer) for all removed buffers.
	Bytes uint64
}

// isZero reports whether the trim result removed nothing.
func (r bucketSegmentTrimResult) isZero() bool {
	return r.Buffers == 0 && r.Bytes == 0
}
