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

// newBucketTrimResult combines removed-capacity accounting with post-operation
// bucket state.
func newBucketTrimResult(removedBuffers int, removedBytes uint64, state bucketState) bucketTrimResult {
	return bucketTrimResult{
		RemovedBuffers:  removedBuffers,
		RemovedBytes:    removedBytes,
		RetainedBuffers: state.RetainedBuffers,
		RetainedBytes:   state.RetainedBytes,
		SlotLimit:       state.SlotLimit,
		AvailableSlots:  state.AvailableSlots,
	}
}

// bucketTrimResult describes a completed bucket-level storage reduction.
//
// The result contains both the removed amount and the bucket state after trim or
// clear. Callers can update higher-level accounting without taking another
// bucket state sample.
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
func (r bucketTrimResult) RemovedState() bucketTrimRemovedState {
	return bucketTrimRemovedState{
		Buffers: r.RemovedBuffers,
		Bytes:   r.RemovedBytes,
	}
}

// BucketState returns the bucket state after the trim operation.
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
