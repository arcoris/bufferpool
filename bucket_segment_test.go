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
	"testing"

	"arcoris.dev/bufferpool/internal/testutil"
)

// TestNewBucketSegmentRequiresPositiveSlotLimit verifies constructor
// validation.
//
// Enabled retained storage must have at least one slot. A disabled bucket should
// be represented by higher-level policy or construction logic, not by an
// enabled segment with no storage.
func TestNewBucketSegmentRequiresPositiveSlotLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		slotLimit int
	}{
		{
			name:      "zero slot limit",
			slotLimit: 0,
		},
		{
			name:      "negative slot limit",
			slotLimit: -1,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testutil.MustPanicWithMessage(t, errBucketSegmentInvalidSlotLimit, func() {
				_ = newBucketSegment(tt.slotLimit)
			})
		})
	}
}

// TestBucketSegmentInitialState verifies the observable state of a newly
// created segment.
func TestBucketSegmentInitialState(t *testing.T) {
	t.Parallel()

	segment := newBucketSegment(3)
	assertBucketSegmentState(t, &segment, bucketSegmentState{
		slotLimit:      3,
		availableSlots: 3,
		isEmpty:        true,
	})
}

// TestZeroValueBucketSegmentIsDisabled verifies the documented zero-value
// behavior.
//
// The zero value is useful as an empty disabled segment, but enabled storage
// should still be constructed with newBucketSegment.
func TestZeroValueBucketSegmentIsDisabled(t *testing.T) {
	t.Parallel()

	var segment bucketSegment
	assertBucketSegmentState(t, &segment, bucketSegmentState{
		isEmpty: true,
		isFull:  true,
	})

	if segment.push(make([]byte, 0, 8)) {
		t.Fatal("zero segment push() = true, want false")
	}

	if buffer, ok := segment.pop(); ok || buffer != nil {
		t.Fatalf("zero segment pop() = (%v, %t), want (nil, false)", buffer, ok)
	}

	assertBucketSegmentTrimResult(t, segment.trim(10), 0, 0)
	assertBucketSegmentTrimResult(t, segment.clear(), 0, 0)
}

// TestBucketSegmentPushStoresZeroLengthBuffer verifies the retained-buffer
// representation.
//
// The segment retains backing capacity, not caller-visible length. Storing
// buffer[:0] keeps the reusable array while avoiding an occupied slot with
// visible contents.
func TestBucketSegmentPushStoresZeroLengthBuffer(t *testing.T) {
	t.Parallel()

	segment := newBucketSegment(2)
	buffer := make([]byte, 5, 16)

	if !segment.push(buffer) {
		t.Fatal("push() = false, want true")
	}

	assertBucketSegmentState(t, &segment, bucketSegmentState{
		slotLimit:      2,
		len:            1,
		availableSlots: 1,
		retainedBytes:  16,
	})

	stored := segment.slots[0]
	if len(stored) != 0 {
		t.Fatalf("stored len = %d, want 0", len(stored))
	}

	if cap(stored) != cap(buffer) {
		t.Fatalf("stored cap = %d, want %d", cap(stored), cap(buffer))
	}

	assertSameBucketSegmentBackingArray(t, stored, buffer)
}

// TestBucketSegmentPushRejectsInvalidOrFull verifies ordinary rejection paths.
//
// Rejections do not mutate occupancy or retained-byte accounting. Higher-level
// admission code owns the public drop reason; the segment only answers whether
// local storage accepted the buffer.
func TestBucketSegmentPushRejectsInvalidOrFull(t *testing.T) {
	t.Parallel()

	segment := newBucketSegment(1)

	if segment.push(nil) {
		t.Fatal("push(nil) = true, want false")
	}
	assertBucketSegmentState(t, &segment, bucketSegmentState{
		slotLimit:      1,
		availableSlots: 1,
		isEmpty:        true,
	})

	if segment.push(make([]byte, 0)) {
		t.Fatal("push(zero capacity) = true, want false")
	}
	assertBucketSegmentState(t, &segment, bucketSegmentState{
		slotLimit:      1,
		availableSlots: 1,
		isEmpty:        true,
	})

	first := make([]byte, 2, 8)
	second := make([]byte, 0, 16)

	if !segment.push(first) {
		t.Fatal("first push() = false, want true")
	}

	if segment.push(second) {
		t.Fatal("push(full segment) = true, want false")
	}

	assertBucketSegmentState(t, &segment, bucketSegmentState{
		slotLimit:     1,
		len:           1,
		retainedBytes: 8,
		isFull:        true,
	})

	assertSameBucketSegmentBackingArray(t, segment.slots[0], first)
}

// TestBucketSegmentPopReturnsLIFOAndClearsSlots verifies stack ordering,
// retained-byte accounting, and reference clearing.
func TestBucketSegmentPopReturnsLIFOAndClearsSlots(t *testing.T) {
	t.Parallel()

	segment := newBucketSegment(3)
	first := make([]byte, 1, 4)
	second := make([]byte, 2, 8)

	if !segment.push(first) {
		t.Fatal("first push() = false, want true")
	}
	if !segment.push(second) {
		t.Fatal("second push() = false, want true")
	}

	_ = mustPopBucketSegmentBuffer(t, &segment, second, 8)
	assertBucketSegmentSlotCleared(t, &segment, 1)
	assertBucketSegmentState(t, &segment, bucketSegmentState{
		slotLimit:      3,
		len:            1,
		availableSlots: 2,
		retainedBytes:  4,
	})

	_ = mustPopBucketSegmentBuffer(t, &segment, first, 4)
	assertBucketSegmentSlotCleared(t, &segment, 0)
	assertBucketSegmentState(t, &segment, bucketSegmentState{
		slotLimit:      3,
		availableSlots: 3,
		isEmpty:        true,
	})

	if buffer, ok := segment.pop(); ok || buffer != nil {
		t.Fatalf("empty pop() = (%v, %t), want (nil, false)", buffer, ok)
	}
}

// TestBucketSegmentTrimRemovesMostRecentBuffers verifies bounded trim behavior.
func TestBucketSegmentTrimRemovesMostRecentBuffers(t *testing.T) {
	t.Parallel()

	segment := newBucketSegment(4)
	for _, capacity := range []int{4, 8, 16} {
		if !segment.push(make([]byte, capacity/2, capacity)) {
			t.Fatalf("push(cap=%d) = false, want true", capacity)
		}
	}

	assertBucketSegmentTrimResult(t, segment.trim(2), 2, 24)

	assertBucketSegmentState(t, &segment, bucketSegmentState{
		slotLimit:      4,
		len:            1,
		availableSlots: 3,
		retainedBytes:  4,
	})

	assertBucketSegmentSlotCleared(t, &segment, 1)
	assertBucketSegmentSlotCleared(t, &segment, 2)
}

// TestBucketSegmentTrimHandlesZeroAndOversizedLimits verifies trim limit
// boundaries.
func TestBucketSegmentTrimHandlesZeroAndOversizedLimits(t *testing.T) {
	t.Parallel()

	segment := newBucketSegment(2)
	if !segment.push(make([]byte, 0, 8)) {
		t.Fatal("first push() = false, want true")
	}
	if !segment.push(make([]byte, 0, 16)) {
		t.Fatal("second push() = false, want true")
	}

	assertBucketSegmentTrimResult(t, segment.trim(0), 0, 0)

	assertBucketSegmentState(t, &segment, bucketSegmentState{
		slotLimit:     2,
		len:           2,
		retainedBytes: 24,
		isFull:        true,
	})

	assertBucketSegmentTrimResult(t, segment.trim(10), 2, 24)
	assertBucketSegmentState(t, &segment, bucketSegmentState{
		slotLimit:      2,
		availableSlots: 2,
		isEmpty:        true,
	})
}

// TestBucketSegmentTrimRejectsNegativeLimit verifies trim planning invariants.
func TestBucketSegmentTrimRejectsNegativeLimit(t *testing.T) {
	t.Parallel()

	segment := newBucketSegment(1)

	testutil.MustPanicWithMessage(t, errBucketSegmentNegativeTrimLimit, func() {
		_ = segment.trim(-1)
	})
}

// TestBucketSegmentInvalidCountPanics verifies the core slot-bound invariant.
//
// count is the boundary between occupied and free slots. Once it leaves the
// valid range, even read-only inspection becomes unsafe because later mutations
// would no longer know which slots are allowed to contain retained buffers.
func TestBucketSegmentInvalidCountPanics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		segment bucketSegment
	}{
		{
			name: "negative count",
			segment: bucketSegment{
				slots: make([][]byte, 1),
				count: -1,
			},
		},
		{
			name: "count beyond slot limit",
			segment: bucketSegment{
				slots: make([][]byte, 1),
				count: 2,
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testutil.MustPanicWithMessage(t, errBucketSegmentInvalidCount, func() {
				_ = tt.segment.len()
			})
		})
	}
}

// TestBucketSegmentPushRejectsNonEmptyFreeSlot verifies stale free-slot
// detection.
//
// push writes to slots[count]. If that slot already contains a buffer reference,
// accepting a new buffer would overwrite evidence of corrupted retained storage.
func TestBucketSegmentPushRejectsNonEmptyFreeSlot(t *testing.T) {
	t.Parallel()

	stale := make([]byte, 0, 8)
	segment := bucketSegment{
		slots:         [][]byte{stale},
		retainedBytes: 0,
	}

	testutil.MustPanicWithMessage(t, errBucketSegmentNonEmptyFreeSlot, func() {
		_ = segment.push(make([]byte, 0, 16))
	})

	assertBucketSegmentState(t, &segment, bucketSegmentState{
		slotLimit:      1,
		availableSlots: 1,
		isEmpty:        true,
	})
	assertSameBucketSegmentBackingArray(t, segment.slots[0], stale)
}

// TestBucketSegmentClearRemovesEverything verifies clear as a full cleanup
// primitive for close, hard trim, and test cleanup paths.
func TestBucketSegmentClearRemovesEverything(t *testing.T) {
	t.Parallel()

	segment := newBucketSegment(2)
	if !segment.push(make([]byte, 0, 8)) {
		t.Fatal("first push() = false, want true")
	}
	if !segment.push(make([]byte, 0, 16)) {
		t.Fatal("second push() = false, want true")
	}

	assertBucketSegmentTrimResult(t, segment.clear(), 2, 24)
	assertBucketSegmentState(t, &segment, bucketSegmentState{
		slotLimit:      2,
		availableSlots: 2,
		isEmpty:        true,
	})

	assertBucketSegmentSlotCleared(t, &segment, 0)
	assertBucketSegmentSlotCleared(t, &segment, 1)
}

// TestBucketSegmentRetainedBytesOverflowPanics verifies that accounting cannot
// wrap when a buffer is added.
func TestBucketSegmentRetainedBytesOverflowPanics(t *testing.T) {
	t.Parallel()

	segment := bucketSegment{
		slots:         make([][]byte, 1),
		retainedBytes: maxBucketSegmentRetainedBytes - 1,
	}

	testutil.MustPanicWithMessage(t, errBucketSegmentRetainedBytesOverflow, func() {
		_ = segment.push(make([]byte, 0, 2))
	})

	if got := segment.len(); got != 0 {
		t.Fatalf("len() after overflow panic = %d, want 0", got)
	}

	if got := segment.retained(); got != maxBucketSegmentRetainedBytes-1 {
		t.Fatalf("retained() after overflow panic = %d, want %d", got, maxBucketSegmentRetainedBytes-1)
	}
}

// TestBucketSegmentRetainedBytesUnderflowPanics verifies that accounting
// corruption is detected before a slot is removed.
func TestBucketSegmentRetainedBytesUnderflowPanics(t *testing.T) {
	t.Parallel()

	buffer := make([]byte, 0, 8)
	segment := bucketSegment{
		slots:         [][]byte{buffer},
		count:         1,
		retainedBytes: 7,
	}

	testutil.MustPanicWithMessage(t, errBucketSegmentRetainedBytesUnderflow, func() {
		_, _ = segment.pop()
	})

	if got := segment.len(); got != 1 {
		t.Fatalf("len() after underflow panic = %d, want 1", got)
	}

	if segment.slots[0] == nil {
		t.Fatal("slot was cleared despite underflow panic")
	}

	if got := segment.retained(); got != 7 {
		t.Fatalf("retained() after underflow panic = %d, want 7", got)
	}
}

// TestBucketSegmentEmptyOccupiedSlotPanics verifies the occupied-slot capacity
// invariant.
func TestBucketSegmentEmptyOccupiedSlotPanics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		slot []byte
	}{
		{
			name: "nil occupied slot",
		},
		{
			name: "zero-capacity occupied slot",
			slot: make([]byte, 0),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			segment := bucketSegment{
				slots: [][]byte{tt.slot},
				count: 1,
			}

			testutil.MustPanicWithMessage(t, errBucketSegmentEmptyOccupiedSlot, func() {
				_, _ = segment.pop()
			})

			if got := segment.len(); got != 1 {
				t.Fatalf("len() after empty-slot panic = %d, want 1", got)
			}

			if !sameBucketSegmentSliceShape(segment.slots[0], tt.slot) {
				t.Fatal("slot changed despite empty-slot panic")
			}
		})
	}
}

// TestBucketSegmentCapacityHelpers verifies the small helpers that centralize
// retained-capacity classification.
func TestBucketSegmentCapacityHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		buffer    []byte
		wantKeep  bool
		wantBytes uint64
	}{
		{
			name: "nil buffer",
		},
		{
			name:   "zero capacity",
			buffer: make([]byte, 0),
		},
		{
			name:      "positive capacity with zero length",
			buffer:    make([]byte, 0, 8),
			wantKeep:  true,
			wantBytes: 8,
		},
		{
			name:      "positive capacity with visible length",
			buffer:    make([]byte, 3, 16),
			wantKeep:  true,
			wantBytes: 16,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := bucketSegmentCanRetain(tt.buffer); got != tt.wantKeep {
				t.Fatalf("bucketSegmentCanRetain() = %t, want %t", got, tt.wantKeep)
			}

			if got := bucketSegmentBufferCapacity(tt.buffer); got != tt.wantBytes {
				t.Fatalf("bucketSegmentBufferCapacity() = %d, want %d", got, tt.wantBytes)
			}
		})
	}
}

// TestBucketSegmentTrimResultIsZero verifies zero-result classification.
func TestBucketSegmentTrimResultIsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result bucketSegmentTrimResult
		want   bool
	}{
		{
			name: "zero result",
			want: true,
		},
		{
			name: "buffers only",
			result: bucketSegmentTrimResult{
				Buffers: 1,
			},
		},
		{
			name: "bytes only",
			result: bucketSegmentTrimResult{
				Bytes: 8,
			},
		},
		{
			name: "buffers and bytes",
			result: bucketSegmentTrimResult{
				Buffers: 1,
				Bytes:   8,
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.result.isZero(); got != tt.want {
				t.Fatalf("isZero() = %t, want %t", got, tt.want)
			}
		})
	}
}

// TestBucketSegmentTrimResultAdd verifies local trim-result aggregation.
func TestBucketSegmentTrimResultAdd(t *testing.T) {
	t.Parallel()

	var result bucketSegmentTrimResult

	result.add(make([]byte, 0, 8))
	assertBucketSegmentTrimResult(t, result, 1, 8)

	result.add(make([]byte, 4, 16))
	assertBucketSegmentTrimResult(t, result, 2, 24)
}

type bucketSegmentState struct {
	slotLimit      int
	len            int
	availableSlots int
	retainedBytes  uint64
	isEmpty        bool
	isFull         bool
}

func assertBucketSegmentState(t *testing.T, segment *bucketSegment, want bucketSegmentState) {
	t.Helper()

	if got := segment.slotLimit(); got != want.slotLimit {
		t.Fatalf("slotLimit() = %d, want %d", got, want.slotLimit)
	}

	if got := segment.len(); got != want.len {
		t.Fatalf("len() = %d, want %d", got, want.len)
	}

	if got := segment.availableSlots(); got != want.availableSlots {
		t.Fatalf("availableSlots() = %d, want %d", got, want.availableSlots)
	}

	if got := segment.retained(); got != want.retainedBytes {
		t.Fatalf("retained() = %d, want %d", got, want.retainedBytes)
	}

	if got := segment.isEmpty(); got != want.isEmpty {
		t.Fatalf("isEmpty() = %t, want %t", got, want.isEmpty)
	}

	if got := segment.isFull(); got != want.isFull {
		t.Fatalf("isFull() = %t, want %t", got, want.isFull)
	}
}

func assertBucketSegmentTrimResult(t *testing.T, result bucketSegmentTrimResult, wantBuffers int, wantBytes uint64) {
	t.Helper()

	if result.Buffers != wantBuffers {
		t.Fatalf("trim result Buffers = %d, want %d", result.Buffers, wantBuffers)
	}

	if result.Bytes != wantBytes {
		t.Fatalf("trim result Bytes = %d, want %d", result.Bytes, wantBytes)
	}

	wantZero := wantBuffers == 0 && wantBytes == 0
	if got := result.isZero(); got != wantZero {
		t.Fatalf("trim result isZero() = %t, want %t", got, wantZero)
	}
}

func mustPopBucketSegmentBuffer(t *testing.T, segment *bucketSegment, wantBacking []byte, wantCapacity int) []byte {
	t.Helper()

	buffer, ok := segment.pop()
	if !ok {
		t.Fatal("pop() ok = false, want true")
	}

	if len(buffer) != 0 {
		t.Fatalf("pop() len = %d, want 0", len(buffer))
	}

	if cap(buffer) != wantCapacity {
		t.Fatalf("pop() cap = %d, want %d", cap(buffer), wantCapacity)
	}

	assertSameBucketSegmentBackingArray(t, buffer, wantBacking)

	return buffer
}

func assertBucketSegmentSlotCleared(t *testing.T, segment *bucketSegment, index int) {
	t.Helper()

	if segment.slots[index] != nil {
		t.Fatalf("slot %d was not cleared", index)
	}
}

func assertSameBucketSegmentBackingArray(t *testing.T, left, right []byte) {
	t.Helper()

	if !sameBucketSegmentBackingArray(left, right) {
		t.Fatal("buffers do not share the same backing array")
	}
}

func sameBucketSegmentBackingArray(left, right []byte) bool {
	if cap(left) == 0 || cap(right) == 0 {
		return cap(left) == cap(right)
	}

	leftBacking := left[:cap(left)]
	rightBacking := right[:cap(right)]

	return &leftBacking[0] == &rightBacking[0]
}

func sameBucketSegmentSliceShape(left, right []byte) bool {
	if (left == nil) != (right == nil) {
		return false
	}

	return len(left) == len(right) && cap(left) == cap(right)
}
