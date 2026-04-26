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

// trim removes up to maxBuffers retained buffers from the bucket.
//
// trim is a local storage-reduction operation. It does not decide why trimming
// is needed, which bucket should be trimmed, or how much memory the whole pool
// should release. Those decisions belong to policy, pressure, controller, and
// trim-planning code.
//
// Responsibility boundary:
//
//   - trim planner decides the victim and requested reduction;
//   - bucket.trim serializes access to this bucket;
//   - bucketSegment executes LIFO removal and retained-byte accounting;
//   - shard/class/pool counters record higher-level trim effects.
//
// maxBuffers follows bucketSegment semantics:
//
//   - maxBuffers == 0 is a valid no-op;
//   - maxBuffers > 0 removes up to that many buffers;
//   - maxBuffers < 0 is an internal trim-planning bug and panics in the segment.
//
// The returned result includes both the removed amount and the bucket state after
// the operation. This lets callers update higher-level accounting without
// immediately taking another locked state sample.
func (b *bucket) trim(maxBuffers int) bucketTrimResult {
	b.mu.Lock()
	defer b.mu.Unlock()

	removed := b.segment.trim(maxBuffers)

	return newBucketTrimResult(removed, b.stateLocked())
}

// clear removes every retained buffer from the bucket.
//
// clear is intended for hard cleanup paths:
//
//   - pool or shard shutdown;
//   - policy changes that invalidate retained storage;
//   - hard pressure cleanup;
//   - tests;
//   - bucket reinitialization paths.
//
// clear is idempotent. Clearing an already empty bucket returns a zero removal
// result and the current empty bucket state.
//
// Like trim, clear does not publish metrics, update shard-level counters, or
// decide cleanup policy. It only executes local synchronized storage reduction.
func (b *bucket) clear() bucketTrimResult {
	b.mu.Lock()
	defer b.mu.Unlock()

	removed := b.segment.clear()

	return newBucketTrimResult(removed, b.stateLocked())
}

// newBucketTrimResult combines removed-capacity accounting with post-operation
// bucket state.
//
// The caller should pass a state captured under the same bucket lock as the
// storage reduction. Keeping this assembly in one helper prevents trim and clear
// from drifting if bucketState gains fields later.
func newBucketTrimResult(removed bucketSegmentTrimResult, state bucketState) bucketTrimResult {
	return bucketTrimResult{
		RemovedBuffers:  removed.Buffers,
		RemovedBytes:    removed.Bytes,
		RetainedBuffers: state.RetainedBuffers,
		RetainedBytes:   state.RetainedBytes,
		SlotLimit:       state.SlotLimit,
		AvailableSlots:  state.AvailableSlots,
	}
}

// bucketTrimResult describes a completed bucket-level storage reduction.
//
// This type intentionally differs from bucketSegmentTrimResult even though the
// removed fields are similar. bucketSegmentTrimResult is a leaf storage result.
// bucketTrimResult is the synchronized bucket-level result that callers above
// bucket care about.
//
// The result contains:
//
//   - how much this operation removed;
//   - what the bucket retained after the operation;
//   - how much local storage capacity remains available.
//
// This shape avoids leaking segment-level details into shard, class, pool, or
// controller code. If bucket later owns more than one segment, bucketTrimResult
// can remain stable while the implementation aggregates segment results.
type bucketTrimResult struct {
	// RemovedBuffers is the number of buffers removed by this operation.
	RemovedBuffers int

	// RemovedBytes is the sum of cap(buffer) for removed buffers.
	RemovedBytes uint64

	// RetainedBuffers is the number of buffers still retained after the
	// operation completes.
	RetainedBuffers int

	// RetainedBytes is the sum of cap(buffer) still retained after the operation
	// completes.
	RetainedBytes uint64

	// SlotLimit is the maximum number of buffers the bucket can retain.
	SlotLimit int

	// AvailableSlots is the number of free retained-buffer slots after the
	// operation completes.
	AvailableSlots int
}

// RemovedState returns only the removed-buffer portion of the trim result.
//
// This helper is useful when higher-level aggregation only needs to accumulate
// released capacity and does not need the post-operation bucket state.
func (r bucketTrimResult) RemovedState() bucketTrimRemovedState {
	return bucketTrimRemovedState{
		Buffers: r.RemovedBuffers,
		Bytes:   r.RemovedBytes,
	}
}

// BucketState returns the bucket state after the trim operation.
//
// The returned state has the same shape as bucket.state(), but it is derived from
// the already completed trim/clear operation and therefore does not require
// taking another bucket lock.
func (r bucketTrimResult) BucketState() bucketState {
	return bucketState{
		RetainedBuffers: r.RetainedBuffers,
		RetainedBytes:   r.RetainedBytes,
		SlotLimit:       r.SlotLimit,
		AvailableSlots:  r.AvailableSlots,
	}
}

// IsZero reports whether the operation removed nothing.
func (r bucketTrimResult) IsZero() bool {
	return r.RemovedState().IsZero()
}

// EmptiedBucket reports whether the bucket retained no buffers after the
// operation completed.
func (r bucketTrimResult) EmptiedBucket() bool {
	return r.BucketState().IsEmpty()
}

// HasAvailableSlots reports whether the bucket can accept at least one retained
// buffer after the operation completed.
func (r bucketTrimResult) HasAvailableSlots() bool {
	return r.BucketState().HasAvailableSlots()
}

// bucketTrimRemovedState is the removed portion of a bucket trim result.
//
// This small value is useful for aggregation code that should not depend on the
// full post-operation bucket state.
type bucketTrimRemovedState struct {
	// Buffers is the number of removed buffers.
	Buffers int

	// Bytes is the sum of cap(buffer) for removed buffers.
	Bytes uint64
}

// IsZero reports whether no buffers and no bytes were removed.
func (s bucketTrimRemovedState) IsZero() bool {
	return s.Buffers == 0 && s.Bytes == 0
}
