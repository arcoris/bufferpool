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
	"testing"

	"arcoris.dev/bufferpool/internal/testutil"
)

// TestBucketTrimZeroLimitIsNoop verifies zero-limit trim behavior.
//
// A zero trim limit is valid. It should remove nothing and return the current
// post-operation bucket state.
func TestBucketTrimZeroLimitIsNoop(t *testing.T) {
	t.Parallel()

	b := newBucket(2)

	if !b.tryPush(make([]byte, 0, 8)) {
		t.Fatal("tryPush capacity 8 returned false, want true")
	}

	if !b.tryPush(make([]byte, 0, 16)) {
		t.Fatal("tryPush capacity 16 returned false, want true")
	}

	result := b.trim(0)

	if !result.IsZero() {
		t.Fatalf("trim(0).IsZero() = false, result=%+v", result)
	}

	if result.RemovedBuffers != 0 {
		t.Fatalf("RemovedBuffers = %d, want 0", result.RemovedBuffers)
	}

	if result.RemovedBytes != 0 {
		t.Fatalf("RemovedBytes = %d, want 0", result.RemovedBytes)
	}

	if result.RetainedBuffers != 2 {
		t.Fatalf("RetainedBuffers after trim(0) = %d, want 2", result.RetainedBuffers)
	}

	if result.RetainedBytes != 24 {
		t.Fatalf("RetainedBytes after trim(0) = %d, want 24", result.RetainedBytes)
	}

	if result.AvailableSlots != 0 {
		t.Fatalf("AvailableSlots after trim(0) = %d, want 0", result.AvailableSlots)
	}

	if result.HasAvailableSlots() {
		t.Fatal("HasAvailableSlots() after trim(0) on full bucket = true, want false")
	}

	if result.EmptiedBucket() {
		t.Fatal("EmptiedBucket() after trim(0) = true, want false")
	}
}

// TestBucketTrimPanicsForNegativeLimit verifies trim-planning guard propagation.
//
// bucket.trim delegates the invalid negative limit to bucketSegment, where the
// storage primitive rejects it as an internal trim-planning bug.
func TestBucketTrimPanicsForNegativeLimit(t *testing.T) {
	t.Parallel()

	b := newBucket(1)

	testutil.MustPanicWithMessage(t, errBucketSegmentNegativeTrimLimit, func() {
		_ = b.trim(-1)
	})
}

// TestBucketTrimRemovesUpToLimit verifies partial local storage reduction.
//
// trim should remove buffers from the LIFO end through bucketSegment and return
// both removed capacity and retained state after the operation.
func TestBucketTrimRemovesUpToLimit(t *testing.T) {
	t.Parallel()

	b := newBucket(4)

	for _, capacity := range []int{8, 16, 32, 64} {
		if !b.tryPush(make([]byte, 0, capacity)) {
			t.Fatalf("tryPush capacity %d returned false, want true", capacity)
		}
	}

	result := b.trim(2)

	if result.RemovedBuffers != 2 {
		t.Fatalf("RemovedBuffers = %d, want 2", result.RemovedBuffers)
	}

	if result.RemovedBytes != 96 {
		t.Fatalf("RemovedBytes = %d, want 96", result.RemovedBytes)
	}

	if result.RetainedBuffers != 2 {
		t.Fatalf("RetainedBuffers = %d, want 2", result.RetainedBuffers)
	}

	if result.RetainedBytes != 24 {
		t.Fatalf("RetainedBytes = %d, want 24", result.RetainedBytes)
	}

	if result.SlotLimit != 4 {
		t.Fatalf("SlotLimit = %d, want 4", result.SlotLimit)
	}

	if result.AvailableSlots != 2 {
		t.Fatalf("AvailableSlots = %d, want 2", result.AvailableSlots)
	}

	if result.IsZero() {
		t.Fatal("IsZero() = true, want false")
	}

	if result.EmptiedBucket() {
		t.Fatal("EmptiedBucket() = true, want false")
	}

	if !result.HasAvailableSlots() {
		t.Fatal("HasAvailableSlots() = false, want true")
	}

	state := b.state()

	if state.RetainedBuffers != result.RetainedBuffers {
		t.Fatalf("bucket state RetainedBuffers = %d, want result value %d", state.RetainedBuffers, result.RetainedBuffers)
	}

	if state.RetainedBytes != result.RetainedBytes {
		t.Fatalf("bucket state RetainedBytes = %d, want result value %d", state.RetainedBytes, result.RetainedBytes)
	}

	if state.AvailableSlots != result.AvailableSlots {
		t.Fatalf("bucket state AvailableSlots = %d, want result value %d", state.AvailableSlots, result.AvailableSlots)
	}
}

// TestBucketTrimStopsAtEmpty verifies over-limit local trim behavior.
//
// Asking to remove more buffers than retained should remove everything and
// report the actual removed amount.
func TestBucketTrimStopsAtEmpty(t *testing.T) {
	t.Parallel()

	b := newBucket(4)

	for _, capacity := range []int{8, 16} {
		if !b.tryPush(make([]byte, 0, capacity)) {
			t.Fatalf("tryPush capacity %d returned false, want true", capacity)
		}
	}

	result := b.trim(10)

	if result.RemovedBuffers != 2 {
		t.Fatalf("RemovedBuffers = %d, want 2", result.RemovedBuffers)
	}

	if result.RemovedBytes != 24 {
		t.Fatalf("RemovedBytes = %d, want 24", result.RemovedBytes)
	}

	if result.RetainedBuffers != 0 {
		t.Fatalf("RetainedBuffers = %d, want 0", result.RetainedBuffers)
	}

	if result.RetainedBytes != 0 {
		t.Fatalf("RetainedBytes = %d, want 0", result.RetainedBytes)
	}

	if result.AvailableSlots != 4 {
		t.Fatalf("AvailableSlots = %d, want 4", result.AvailableSlots)
	}

	if result.IsZero() {
		t.Fatal("IsZero() = true, want false")
	}

	if !result.EmptiedBucket() {
		t.Fatal("EmptiedBucket() = false, want true")
	}

	if !result.HasAvailableSlots() {
		t.Fatal("HasAvailableSlots() = false, want true")
	}
}

// TestBucketTrimEmptyBucket verifies trimming an already empty bucket.
//
// Empty trim should be safe and should return zero removal with unchanged empty
// bucket state.
func TestBucketTrimEmptyBucket(t *testing.T) {
	t.Parallel()

	b := newBucket(3)

	result := b.trim(2)

	if !result.IsZero() {
		t.Fatalf("trim empty bucket result = %+v, want zero removal", result)
	}

	if result.RemovedBuffers != 0 {
		t.Fatalf("RemovedBuffers = %d, want 0", result.RemovedBuffers)
	}

	if result.RemovedBytes != 0 {
		t.Fatalf("RemovedBytes = %d, want 0", result.RemovedBytes)
	}

	if result.RetainedBuffers != 0 {
		t.Fatalf("RetainedBuffers = %d, want 0", result.RetainedBuffers)
	}

	if result.RetainedBytes != 0 {
		t.Fatalf("RetainedBytes = %d, want 0", result.RetainedBytes)
	}

	if result.SlotLimit != 3 {
		t.Fatalf("SlotLimit = %d, want 3", result.SlotLimit)
	}

	if result.AvailableSlots != 3 {
		t.Fatalf("AvailableSlots = %d, want 3", result.AvailableSlots)
	}

	if !result.EmptiedBucket() {
		t.Fatal("EmptiedBucket() = false, want true")
	}

	if !result.HasAvailableSlots() {
		t.Fatal("HasAvailableSlots() = false, want true")
	}
}

// TestZeroValueBucketTrim verifies trim behavior for disabled zero-value
// buckets.
//
// The zero-value bucket has no slots and no retained buffers. Trimming it should
// be a safe no-op for non-negative trim limits.
func TestZeroValueBucketTrim(t *testing.T) {
	t.Parallel()

	var b bucket

	result := b.trim(1)

	if !result.IsZero() {
		t.Fatalf("zero-value bucket trim result = %+v, want zero removal", result)
	}

	if result.RetainedBuffers != 0 {
		t.Fatalf("RetainedBuffers = %d, want 0", result.RetainedBuffers)
	}

	if result.RetainedBytes != 0 {
		t.Fatalf("RetainedBytes = %d, want 0", result.RetainedBytes)
	}

	if result.SlotLimit != 0 {
		t.Fatalf("SlotLimit = %d, want 0", result.SlotLimit)
	}

	if result.AvailableSlots != 0 {
		t.Fatalf("AvailableSlots = %d, want 0", result.AvailableSlots)
	}

	if !result.EmptiedBucket() {
		t.Fatal("EmptiedBucket() = false, want true")
	}

	if result.HasAvailableSlots() {
		t.Fatal("HasAvailableSlots() = true, want false")
	}
}

// TestBucketClearRemovesAllBuffers verifies hard local cleanup.
//
// clear should remove all retained buffers, report the removed capacity, and
// return the empty post-operation bucket state.
func TestBucketClearRemovesAllBuffers(t *testing.T) {
	t.Parallel()

	b := newBucket(3)

	for _, capacity := range []int{8, 16, 32} {
		if !b.tryPush(make([]byte, 0, capacity)) {
			t.Fatalf("tryPush capacity %d returned false, want true", capacity)
		}
	}

	result := b.clear()

	if result.RemovedBuffers != 3 {
		t.Fatalf("RemovedBuffers = %d, want 3", result.RemovedBuffers)
	}

	if result.RemovedBytes != 56 {
		t.Fatalf("RemovedBytes = %d, want 56", result.RemovedBytes)
	}

	if result.RetainedBuffers != 0 {
		t.Fatalf("RetainedBuffers = %d, want 0", result.RetainedBuffers)
	}

	if result.RetainedBytes != 0 {
		t.Fatalf("RetainedBytes = %d, want 0", result.RetainedBytes)
	}

	if result.SlotLimit != 3 {
		t.Fatalf("SlotLimit = %d, want 3", result.SlotLimit)
	}

	if result.AvailableSlots != 3 {
		t.Fatalf("AvailableSlots = %d, want 3", result.AvailableSlots)
	}

	if result.IsZero() {
		t.Fatal("IsZero() = true, want false")
	}

	if !result.EmptiedBucket() {
		t.Fatal("EmptiedBucket() = false, want true")
	}

	if !result.HasAvailableSlots() {
		t.Fatal("HasAvailableSlots() = false, want true")
	}

	state := b.state()
	if !state.IsEmpty() {
		t.Fatal("bucket state after clear IsEmpty() = false, want true")
	}
}

// TestBucketClearEmptyBucket verifies idempotent clear behavior.
//
// clear should be safe to call on an empty bucket. This is important for close,
// cleanup, and hard-reset paths.
func TestBucketClearEmptyBucket(t *testing.T) {
	t.Parallel()

	b := newBucket(2)

	result := b.clear()

	if !result.IsZero() {
		t.Fatalf("clear empty bucket result = %+v, want zero removal", result)
	}

	if result.RetainedBuffers != 0 {
		t.Fatalf("RetainedBuffers = %d, want 0", result.RetainedBuffers)
	}

	if result.RetainedBytes != 0 {
		t.Fatalf("RetainedBytes = %d, want 0", result.RetainedBytes)
	}

	if result.SlotLimit != 2 {
		t.Fatalf("SlotLimit = %d, want 2", result.SlotLimit)
	}

	if result.AvailableSlots != 2 {
		t.Fatalf("AvailableSlots = %d, want 2", result.AvailableSlots)
	}

	if !result.EmptiedBucket() {
		t.Fatal("EmptiedBucket() = false, want true")
	}
}

// TestZeroValueBucketClear verifies clear behavior for disabled zero-value
// buckets.
//
// A disabled zero-value bucket has no retained state and no storage capacity.
// clear should report that nothing was removed.
func TestZeroValueBucketClear(t *testing.T) {
	t.Parallel()

	var b bucket

	result := b.clear()

	if !result.IsZero() {
		t.Fatalf("zero-value clear result = %+v, want zero removal", result)
	}

	if result.RetainedBuffers != 0 {
		t.Fatalf("RetainedBuffers = %d, want 0", result.RetainedBuffers)
	}

	if result.RetainedBytes != 0 {
		t.Fatalf("RetainedBytes = %d, want 0", result.RetainedBytes)
	}

	if result.SlotLimit != 0 {
		t.Fatalf("SlotLimit = %d, want 0", result.SlotLimit)
	}

	if result.AvailableSlots != 0 {
		t.Fatalf("AvailableSlots = %d, want 0", result.AvailableSlots)
	}

	if !result.EmptiedBucket() {
		t.Fatal("EmptiedBucket() = false, want true")
	}

	if result.HasAvailableSlots() {
		t.Fatal("HasAvailableSlots() = true, want false")
	}
}

// TestBucketTrimResultRemovedState verifies conversion to the removed-only
// result shape.
//
// Higher-level aggregation may only need removed buffers and bytes. RemovedState
// should return that narrow value without exposing post-operation bucket state.
func TestBucketTrimResultRemovedState(t *testing.T) {
	t.Parallel()

	result := bucketTrimResult{
		RemovedBuffers:  3,
		RemovedBytes:    56,
		RetainedBuffers: 1,
		RetainedBytes:   8,
		SlotLimit:       4,
		AvailableSlots:  3,
	}

	removed := result.RemovedState()

	if removed.Buffers != 3 {
		t.Fatalf("RemovedState().Buffers = %d, want 3", removed.Buffers)
	}

	if removed.Bytes != 56 {
		t.Fatalf("RemovedState().Bytes = %d, want 56", removed.Bytes)
	}

	if removed.IsZero() {
		t.Fatal("RemovedState().IsZero() = true, want false")
	}
}

// TestBucketTrimResultBucketState verifies conversion to bucketState.
//
// BucketState should expose the post-operation retained state using the same
// value type returned by bucket.state().
func TestBucketTrimResultBucketState(t *testing.T) {
	t.Parallel()

	result := bucketTrimResult{
		RemovedBuffers:  2,
		RemovedBytes:    96,
		RetainedBuffers: 2,
		RetainedBytes:   24,
		SlotLimit:       4,
		AvailableSlots:  2,
	}

	state := result.BucketState()

	if state.RetainedBuffers != 2 {
		t.Fatalf("BucketState().RetainedBuffers = %d, want 2", state.RetainedBuffers)
	}

	if state.RetainedBytes != 24 {
		t.Fatalf("BucketState().RetainedBytes = %d, want 24", state.RetainedBytes)
	}

	if state.SlotLimit != 4 {
		t.Fatalf("BucketState().SlotLimit = %d, want 4", state.SlotLimit)
	}

	if state.AvailableSlots != 2 {
		t.Fatalf("BucketState().AvailableSlots = %d, want 2", state.AvailableSlots)
	}

	if state.IsEmpty() {
		t.Fatal("BucketState().IsEmpty() = true, want false")
	}

	if state.IsFull() {
		t.Fatal("BucketState().IsFull() = true, want false")
	}

	if !state.HasAvailableSlots() {
		t.Fatal("BucketState().HasAvailableSlots() = false, want true")
	}
}

// TestBucketTrimResultPredicates verifies bucketTrimResult helper semantics.
func TestBucketTrimResultPredicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// result is the trim result being inspected.
		result bucketTrimResult

		wantZero         bool
		wantEmptied      bool
		wantHasAvailable bool
	}{
		{
			name: "zero removal and empty disabled bucket",
			result: bucketTrimResult{
				RemovedBuffers:  0,
				RemovedBytes:    0,
				RetainedBuffers: 0,
				RetainedBytes:   0,
				SlotLimit:       0,
				AvailableSlots:  0,
			},
			wantZero:         true,
			wantEmptied:      true,
			wantHasAvailable: false,
		},
		{
			name: "removed data but bucket still retained buffers",
			result: bucketTrimResult{
				RemovedBuffers:  1,
				RemovedBytes:    64,
				RetainedBuffers: 2,
				RetainedBytes:   24,
				SlotLimit:       4,
				AvailableSlots:  2,
			},
			wantZero:         false,
			wantEmptied:      false,
			wantHasAvailable: true,
		},
		{
			name: "removed data and emptied bucket",
			result: bucketTrimResult{
				RemovedBuffers:  2,
				RemovedBytes:    24,
				RetainedBuffers: 0,
				RetainedBytes:   0,
				SlotLimit:       4,
				AvailableSlots:  4,
			},
			wantZero:         false,
			wantEmptied:      true,
			wantHasAvailable: true,
		},
		{
			name: "no removal but full bucket",
			result: bucketTrimResult{
				RemovedBuffers:  0,
				RemovedBytes:    0,
				RetainedBuffers: 4,
				RetainedBytes:   120,
				SlotLimit:       4,
				AvailableSlots:  0,
			},
			wantZero:         true,
			wantEmptied:      false,
			wantHasAvailable: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.result.IsZero(); got != tt.wantZero {
				t.Fatalf("IsZero() = %t, want %t", got, tt.wantZero)
			}

			if got := tt.result.EmptiedBucket(); got != tt.wantEmptied {
				t.Fatalf("EmptiedBucket() = %t, want %t", got, tt.wantEmptied)
			}

			if got := tt.result.HasAvailableSlots(); got != tt.wantHasAvailable {
				t.Fatalf("HasAvailableSlots() = %t, want %t", got, tt.wantHasAvailable)
			}
		})
	}
}

// TestBucketTrimRemovedStateIsZero verifies removed-only zero classification.
func TestBucketTrimRemovedStateIsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// state is the removed-only state being inspected.
		state bucketTrimRemovedState

		want bool
	}{
		{
			name:  "zero",
			state: bucketTrimRemovedState{},
			want:  true,
		},
		{
			name: "buffers only",
			state: bucketTrimRemovedState{
				Buffers: 1,
			},
			want: false,
		},
		{
			name: "bytes only",
			state: bucketTrimRemovedState{
				Bytes: 8,
			},
			want: false,
		},
		{
			name: "buffers and bytes",
			state: bucketTrimRemovedState{
				Buffers: 1,
				Bytes:   8,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.state.IsZero(); got != tt.want {
				t.Fatalf("bucketTrimRemovedState(%+v).IsZero() = %t, want %t", tt.state, got, tt.want)
			}
		})
	}
}

// TestBucketConcurrentTrim verifies that bucket.trim serializes local storage
// reduction.
//
// Multiple concurrent trim calls should collectively remove each retained buffer
// at most once and should leave the bucket in a consistent empty state.
func TestBucketConcurrentTrim(t *testing.T) {
	t.Parallel()

	const slotLimit = 64
	const goroutines = 16
	const trimLimitPerGoroutine = 8

	b := newBucket(slotLimit)

	for i := 0; i < slotLimit; i++ {
		if !b.tryPush(make([]byte, 0, 8)) {
			t.Fatalf("initial tryPush %d returned false, want true", i)
		}
	}

	var wg sync.WaitGroup
	var resultMu sync.Mutex

	var removedBuffers int
	var removedBytes uint64

	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			result := b.trim(trimLimitPerGoroutine)

			resultMu.Lock()
			removedBuffers += result.RemovedBuffers
			removedBytes += result.RemovedBytes
			resultMu.Unlock()
		}()
	}

	wg.Wait()

	if removedBuffers != slotLimit {
		t.Fatalf("concurrent trim removed buffers = %d, want %d", removedBuffers, slotLimit)
	}

	if removedBytes != uint64(slotLimit*8) {
		t.Fatalf("concurrent trim removed bytes = %d, want %d", removedBytes, uint64(slotLimit*8))
	}

	state := b.state()

	if state.RetainedBuffers != 0 {
		t.Fatalf("RetainedBuffers after concurrent trim = %d, want 0", state.RetainedBuffers)
	}

	if state.RetainedBytes != 0 {
		t.Fatalf("RetainedBytes after concurrent trim = %d, want 0", state.RetainedBytes)
	}

	if state.AvailableSlots != slotLimit {
		t.Fatalf("AvailableSlots after concurrent trim = %d, want %d", state.AvailableSlots, slotLimit)
	}
}

// TestBucketConcurrentClear verifies that clear is idempotent under concurrent
// cleanup attempts.
//
// Only one clear call should remove the retained buffers. Other calls should
// observe an already empty bucket and report zero removal.
func TestBucketConcurrentClear(t *testing.T) {
	t.Parallel()

	const slotLimit = 64
	const goroutines = 16

	b := newBucket(slotLimit)

	for i := 0; i < slotLimit; i++ {
		if !b.tryPush(make([]byte, 0, 8)) {
			t.Fatalf("initial tryPush %d returned false, want true", i)
		}
	}

	var wg sync.WaitGroup
	var resultMu sync.Mutex

	var removedBuffers int
	var removedBytes uint64
	var nonZeroResults int

	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			result := b.clear()

			resultMu.Lock()
			removedBuffers += result.RemovedBuffers
			removedBytes += result.RemovedBytes
			if !result.IsZero() {
				nonZeroResults++
			}
			resultMu.Unlock()
		}()
	}

	wg.Wait()

	if removedBuffers != slotLimit {
		t.Fatalf("concurrent clear removed buffers = %d, want %d", removedBuffers, slotLimit)
	}

	if removedBytes != uint64(slotLimit*8) {
		t.Fatalf("concurrent clear removed bytes = %d, want %d", removedBytes, uint64(slotLimit*8))
	}

	if nonZeroResults != 1 {
		t.Fatalf("non-zero clear results = %d, want 1", nonZeroResults)
	}

	state := b.state()

	if state.RetainedBuffers != 0 {
		t.Fatalf("RetainedBuffers after concurrent clear = %d, want 0", state.RetainedBuffers)
	}

	if state.RetainedBytes != 0 {
		t.Fatalf("RetainedBytes after concurrent clear = %d, want 0", state.RetainedBytes)
	}

	if state.AvailableSlots != slotLimit {
		t.Fatalf("AvailableSlots after concurrent clear = %d, want %d", state.AvailableSlots, slotLimit)
	}
}
