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

func TestNewBucketRequiresPositiveSlotLimit(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		slotLimit int
	}{
		{name: "zero slot limit", slotLimit: 0},
		{name: "negative slot limit", slotLimit: -1},
	} {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testutil.MustPanicWithMessage(t, errBucketInvalidSlotLimit, func() {
				_ = newBucket(tt.slotLimit)
			})
		})
	}
}

func TestNewBucketInitialState(t *testing.T) {
	t.Parallel()

	b := newBucket(3)

	assertBucketState(t, b.state(), bucketState{
		SlotLimit:      3,
		AvailableSlots: 3,
	})
}

func TestZeroValueBucketIsDisabled(t *testing.T) {
	t.Parallel()

	var b bucket

	assertBucketState(t, b.state(), bucketState{})

	if b.push(make([]byte, 0, 8)) {
		t.Fatal("zero bucket push() = true, want false")
	}

	if buffer, ok := b.pop(); ok || buffer != nil {
		t.Fatalf("zero bucket pop() = (%v, %t), want (nil, false)", buffer, ok)
	}

	assertBucketTrimResult(t, b.trim(10), bucketTrimResult{})
	assertBucketTrimResult(t, b.clear(), bucketTrimResult{})
}

func TestBucketPushStoresZeroLengthBuffer(t *testing.T) {
	t.Parallel()

	b := newBucket(2)
	buffer := make([]byte, 5, 16)

	if !b.push(buffer) {
		t.Fatal("push() = false, want true")
	}

	assertBucketState(t, b.state(), bucketState{
		RetainedBuffers: 1,
		RetainedBytes:   16,
		SlotLimit:       2,
		AvailableSlots:  1,
	})

	stored := b.slots[0]
	if len(stored) != 0 {
		t.Fatalf("stored len = %d, want 0", len(stored))
	}
	if cap(stored) != cap(buffer) {
		t.Fatalf("stored cap = %d, want %d", cap(stored), cap(buffer))
	}

	assertSameBucketBackingArray(t, stored, buffer)
}

func TestBucketPushRejectsInvalidOrFull(t *testing.T) {
	t.Parallel()

	b := newBucket(1)

	if b.push(nil) {
		t.Fatal("push(nil) = true, want false")
	}
	if b.push(make([]byte, 0)) {
		t.Fatal("push(zero capacity) = true, want false")
	}

	assertBucketState(t, b.state(), bucketState{
		SlotLimit:      1,
		AvailableSlots: 1,
	})

	first := make([]byte, 2, 8)
	second := make([]byte, 0, 16)

	if !b.push(first) {
		t.Fatal("first push() = false, want true")
	}
	if b.push(second) {
		t.Fatal("push(full bucket) = true, want false")
	}

	assertBucketState(t, b.state(), bucketState{
		RetainedBuffers: 1,
		RetainedBytes:   8,
		SlotLimit:       1,
	})
	assertSameBucketBackingArray(t, b.slots[0], first)
}

func TestBucketPopReturnsLIFOAndClearsSlots(t *testing.T) {
	t.Parallel()

	b := newBucket(3)
	first := make([]byte, 1, 4)
	second := make([]byte, 2, 8)

	if !b.push(first) {
		t.Fatal("first push() = false, want true")
	}
	if !b.push(second) {
		t.Fatal("second push() = false, want true")
	}

	_ = mustPopBucketBuffer(t, &b, second, 8)
	assertBucketSlotCleared(t, &b, 1)
	assertBucketState(t, b.state(), bucketState{
		RetainedBuffers: 1,
		RetainedBytes:   4,
		SlotLimit:       3,
		AvailableSlots:  2,
	})

	_ = mustPopBucketBuffer(t, &b, first, 4)
	assertBucketSlotCleared(t, &b, 0)
	assertBucketState(t, b.state(), bucketState{
		SlotLimit:      3,
		AvailableSlots: 3,
	})

	if buffer, ok := b.pop(); ok || buffer != nil {
		t.Fatalf("empty pop() = (%v, %t), want (nil, false)", buffer, ok)
	}
}

func TestBucketInspectionHelpers(t *testing.T) {
	t.Parallel()

	b := newBucket(2)

	if b.len() != 0 {
		t.Fatalf("len() initially = %d, want 0", b.len())
	}
	if b.retained() != 0 {
		t.Fatalf("retained() initially = %d, want 0", b.retained())
	}
	if b.slotLimit() != 2 {
		t.Fatalf("slotLimit() initially = %d, want 2", b.slotLimit())
	}
	if b.availableSlots() != 2 {
		t.Fatalf("availableSlots() initially = %d, want 2", b.availableSlots())
	}
	if !b.isEmpty() {
		t.Fatal("isEmpty() initially = false, want true")
	}
	if b.isFull() {
		t.Fatal("isFull() initially = true, want false")
	}

	if !b.push(make([]byte, 0, 8)) {
		t.Fatal("push() = false, want true")
	}

	if b.len() != 1 {
		t.Fatalf("len() after push = %d, want 1", b.len())
	}
	if b.retained() != 8 {
		t.Fatalf("retained() after push = %d, want 8", b.retained())
	}
	if b.availableSlots() != 1 {
		t.Fatalf("availableSlots() after push = %d, want 1", b.availableSlots())
	}
	if b.isEmpty() {
		t.Fatal("isEmpty() after push = true, want false")
	}
	if b.isFull() {
		t.Fatal("isFull() after one push into two-slot bucket = true, want false")
	}
}

func TestBucketStatePredicates(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name             string
		state            bucketState
		wantEmpty        bool
		wantFull         bool
		wantHasAvailable bool
	}{
		{name: "disabled empty bucket", state: bucketState{}, wantEmpty: true, wantFull: true},
		{
			name: "enabled empty bucket",
			state: bucketState{
				SlotLimit:      2,
				AvailableSlots: 2,
			},
			wantEmpty:        true,
			wantHasAvailable: true,
		},
		{
			name: "partially filled bucket",
			state: bucketState{
				RetainedBuffers: 1,
				RetainedBytes:   8,
				SlotLimit:       2,
				AvailableSlots:  1,
			},
			wantHasAvailable: true,
		},
		{
			name: "full bucket",
			state: bucketState{
				RetainedBuffers: 2,
				RetainedBytes:   24,
				SlotLimit:       2,
			},
			wantFull: true,
		},
	} {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.state.IsEmpty(); got != tt.wantEmpty {
				t.Fatalf("IsEmpty() = %t, want %t", got, tt.wantEmpty)
			}
			if got := tt.state.IsFull(); got != tt.wantFull {
				t.Fatalf("IsFull() = %t, want %t", got, tt.wantFull)
			}
			if got := tt.state.HasAvailableSlots(); got != tt.wantHasAvailable {
				t.Fatalf("HasAvailableSlots() = %t, want %t", got, tt.wantHasAvailable)
			}
		})
	}
}

func TestBucketTrimRemovesMostRecentBuffers(t *testing.T) {
	t.Parallel()

	b := newBucket(4)
	for _, capacity := range []int{4, 8, 16} {
		if !b.push(make([]byte, capacity/2, capacity)) {
			t.Fatalf("push(cap=%d) = false, want true", capacity)
		}
	}

	result := b.trim(2)
	assertBucketTrimResult(t, result, bucketTrimResult{
		RemovedBuffers:  2,
		RemovedBytes:    24,
		RetainedBuffers: 1,
		RetainedBytes:   4,
		SlotLimit:       4,
		AvailableSlots:  3,
	})

	assertBucketState(t, b.state(), result.BucketState())
	assertBucketSlotCleared(t, &b, 1)
	assertBucketSlotCleared(t, &b, 2)
}

func TestBucketTrimHandlesZeroAndOversizedLimits(t *testing.T) {
	t.Parallel()

	b := newBucket(2)
	if !b.push(make([]byte, 0, 8)) {
		t.Fatal("first push() = false, want true")
	}
	if !b.push(make([]byte, 0, 16)) {
		t.Fatal("second push() = false, want true")
	}

	assertBucketTrimResult(t, b.trim(0), bucketTrimResult{
		RetainedBuffers: 2,
		RetainedBytes:   24,
		SlotLimit:       2,
	})
	assertBucketState(t, b.state(), bucketState{
		RetainedBuffers: 2,
		RetainedBytes:   24,
		SlotLimit:       2,
	})

	assertBucketTrimResult(t, b.trim(10), bucketTrimResult{
		RemovedBuffers: 2,
		RemovedBytes:   24,
		SlotLimit:      2,
		AvailableSlots: 2,
	})
	assertBucketState(t, b.state(), bucketState{
		SlotLimit:      2,
		AvailableSlots: 2,
	})
}

func TestBucketTrimRejectsNegativeLimit(t *testing.T) {
	t.Parallel()

	b := newBucket(1)

	testutil.MustPanicWithMessage(t, errBucketNegativeTrimLimit, func() {
		_ = b.trim(-1)
	})
}

func TestBucketClearRemovesEverythingAndIsRepeatable(t *testing.T) {
	t.Parallel()

	b := newBucket(2)
	if !b.push(make([]byte, 0, 8)) {
		t.Fatal("first push() = false, want true")
	}
	if !b.push(make([]byte, 0, 16)) {
		t.Fatal("second push() = false, want true")
	}

	assertBucketTrimResult(t, b.clear(), bucketTrimResult{
		RemovedBuffers: 2,
		RemovedBytes:   24,
		SlotLimit:      2,
		AvailableSlots: 2,
	})
	assertBucketState(t, b.state(), bucketState{
		SlotLimit:      2,
		AvailableSlots: 2,
	})
	assertBucketSlotCleared(t, &b, 0)
	assertBucketSlotCleared(t, &b, 1)

	assertBucketTrimResult(t, b.clear(), bucketTrimResult{
		SlotLimit:      2,
		AvailableSlots: 2,
	})
}

func TestBucketTrimResultHelpers(t *testing.T) {
	t.Parallel()

	result := bucketTrimResult{
		RemovedBuffers:  2,
		RemovedBytes:    96,
		RetainedBuffers: 1,
		RetainedBytes:   8,
		SlotLimit:       4,
		AvailableSlots:  3,
	}

	removed := result.RemovedState()
	if removed.Buffers != 2 || removed.Bytes != 96 {
		t.Fatalf("RemovedState() = %+v, want buffers=2 bytes=96", removed)
	}
	if removed.IsZero() {
		t.Fatal("RemovedState().IsZero() = true, want false")
	}

	assertBucketState(t, result.BucketState(), bucketState{
		RetainedBuffers: 1,
		RetainedBytes:   8,
		SlotLimit:       4,
		AvailableSlots:  3,
	})
	if result.IsZero() {
		t.Fatal("IsZero() = true, want false")
	}
	if result.EmptiedBucket() {
		t.Fatal("EmptiedBucket() = true, want false")
	}
	if !result.HasAvailableSlots() {
		t.Fatal("HasAvailableSlots() = false, want true")
	}
}

func TestBucketInvalidCountPanics(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		bucket bucket
	}{
		{
			name: "negative count",
			bucket: bucket{
				slots: make([][]byte, 1),
				count: -1,
			},
		},
		{
			name: "count beyond slot limit",
			bucket: bucket{
				slots: make([][]byte, 1),
				count: 2,
			},
		},
	} {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testutil.MustPanicWithMessage(t, errBucketInvalidCount, func() {
				_ = tt.bucket.len()
			})
		})
	}
}

func TestBucketPushRejectsNonEmptyFreeSlot(t *testing.T) {
	t.Parallel()

	stale := make([]byte, 0, 8)
	b := bucket{
		slots: [][]byte{stale},
	}

	testutil.MustPanicWithMessage(t, errBucketNonEmptyFreeSlot, func() {
		_ = b.push(make([]byte, 0, 16))
	})

	assertBucketState(t, b.state(), bucketState{
		SlotLimit:      1,
		AvailableSlots: 1,
	})
	assertSameBucketBackingArray(t, b.slots[0], stale)
}

func TestBucketEmptyOccupiedSlotPanics(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		slot []byte
	}{
		{name: "nil slot", slot: nil},
		{name: "zero-capacity slot", slot: make([]byte, 0)},
	} {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := bucket{
				slots:         [][]byte{tt.slot},
				count:         1,
				retainedBytes: 1,
			}

			testutil.MustPanicWithMessage(t, errBucketEmptyOccupiedSlot, func() {
				_, _ = b.pop()
			})

			if !sameBucketSliceShape(b.slots[0], tt.slot) {
				t.Fatalf("slot changed after panic: got len=%d cap=%d, want len=%d cap=%d", len(b.slots[0]), cap(b.slots[0]), len(tt.slot), cap(tt.slot))
			}
		})
	}
}

func TestBucketRetainedBytesOverflowPanics(t *testing.T) {
	t.Parallel()

	b := bucket{
		slots:         make([][]byte, 1),
		retainedBytes: maxBucketRetainedBytes - 1,
	}

	testutil.MustPanicWithMessage(t, errBucketRetainedBytesOverflow, func() {
		_ = b.push(make([]byte, 0, 2))
	})

	if got := b.retained(); got != maxBucketRetainedBytes-1 {
		t.Fatalf("retained() after overflow panic = %d, want %d", got, maxBucketRetainedBytes-1)
	}
	if b.count != 0 {
		t.Fatalf("count after overflow panic = %d, want 0", b.count)
	}
	if b.slots[0] != nil {
		t.Fatal("slot after overflow panic is non-nil, want nil")
	}
}

func TestBucketRetainedBytesUnderflowPanics(t *testing.T) {
	t.Parallel()

	buffer := make([]byte, 0, 8)
	b := bucket{
		slots:         [][]byte{buffer},
		count:         1,
		retainedBytes: 4,
	}

	testutil.MustPanicWithMessage(t, errBucketRetainedBytesUnderflow, func() {
		_, _ = b.pop()
	})

	if got := b.count; got != 1 {
		t.Fatalf("count after underflow panic = %d, want 1", got)
	}
	assertSameBucketBackingArray(t, b.slots[0], buffer)
}

func TestBucketRetainedPanicsForInvalidCount(t *testing.T) {
	t.Parallel()

	b := bucket{
		slots: make([][]byte, 1),
		count: 2,
	}

	testutil.MustPanicWithMessage(t, errBucketInvalidCount, func() {
		_ = b.retained()
	})
}

func TestBucketCapacityHelpers(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		buffer    []byte
		wantKeep  bool
		wantBytes uint64
	}{
		{name: "nil buffer"},
		{name: "zero-capacity buffer", buffer: make([]byte, 0)},
		{name: "positive-capacity empty buffer", buffer: make([]byte, 0, 8), wantKeep: true, wantBytes: 8},
		{name: "positive-capacity non-empty buffer", buffer: make([]byte, 3, 16), wantKeep: true, wantBytes: 16},
	} {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := bucketCanRetain(tt.buffer); got != tt.wantKeep {
				t.Fatalf("bucketCanRetain() = %t, want %t", got, tt.wantKeep)
			}
			if got := bucketBufferCapacity(tt.buffer); got != tt.wantBytes {
				t.Fatalf("bucketBufferCapacity() = %d, want %d", got, tt.wantBytes)
			}
		})
	}
}

func assertBucketState(t *testing.T, got, want bucketState) {
	t.Helper()

	if got.RetainedBuffers != want.RetainedBuffers {
		t.Fatalf("RetainedBuffers = %d, want %d", got.RetainedBuffers, want.RetainedBuffers)
	}
	if got.RetainedBytes != want.RetainedBytes {
		t.Fatalf("RetainedBytes = %d, want %d", got.RetainedBytes, want.RetainedBytes)
	}
	if got.SlotLimit != want.SlotLimit {
		t.Fatalf("SlotLimit = %d, want %d", got.SlotLimit, want.SlotLimit)
	}
	if got.AvailableSlots != want.AvailableSlots {
		t.Fatalf("AvailableSlots = %d, want %d", got.AvailableSlots, want.AvailableSlots)
	}
	if got.IsEmpty() != (want.RetainedBuffers == 0) {
		t.Fatalf("IsEmpty() = %t, want %t", got.IsEmpty(), want.RetainedBuffers == 0)
	}
	if got.IsFull() != (want.AvailableSlots == 0) {
		t.Fatalf("IsFull() = %t, want %t", got.IsFull(), want.AvailableSlots == 0)
	}
	if got.HasAvailableSlots() != (want.AvailableSlots > 0) {
		t.Fatalf("HasAvailableSlots() = %t, want %t", got.HasAvailableSlots(), want.AvailableSlots > 0)
	}
}

func assertBucketTrimResult(t *testing.T, got, want bucketTrimResult) {
	t.Helper()

	if got.RemovedBuffers != want.RemovedBuffers {
		t.Fatalf("RemovedBuffers = %d, want %d", got.RemovedBuffers, want.RemovedBuffers)
	}
	if got.RemovedBytes != want.RemovedBytes {
		t.Fatalf("RemovedBytes = %d, want %d", got.RemovedBytes, want.RemovedBytes)
	}
	if got.RetainedBuffers != want.RetainedBuffers {
		t.Fatalf("RetainedBuffers = %d, want %d", got.RetainedBuffers, want.RetainedBuffers)
	}
	if got.RetainedBytes != want.RetainedBytes {
		t.Fatalf("RetainedBytes = %d, want %d", got.RetainedBytes, want.RetainedBytes)
	}
	if got.SlotLimit != want.SlotLimit {
		t.Fatalf("SlotLimit = %d, want %d", got.SlotLimit, want.SlotLimit)
	}
	if got.AvailableSlots != want.AvailableSlots {
		t.Fatalf("AvailableSlots = %d, want %d", got.AvailableSlots, want.AvailableSlots)
	}
}

func mustPopBucketBuffer(t *testing.T, b *bucket, wantBacking []byte, wantCapacity int) []byte {
	t.Helper()

	buffer, ok := b.pop()
	if !ok {
		t.Fatal("pop() = ok false, want true")
	}
	if len(buffer) != 0 {
		t.Fatalf("popped len = %d, want 0", len(buffer))
	}
	if cap(buffer) != wantCapacity {
		t.Fatalf("popped cap = %d, want %d", cap(buffer), wantCapacity)
	}

	assertSameBucketBackingArray(t, buffer, wantBacking)

	return buffer
}

func assertBucketSlotCleared(t *testing.T, b *bucket, index int) {
	t.Helper()

	if b.slots[index] != nil {
		t.Fatalf("slot %d = %v, want nil", index, b.slots[index])
	}
}

func assertSameBucketBackingArray(t *testing.T, left, right []byte) {
	t.Helper()

	if !sameBucketBackingArray(left, right) {
		t.Fatalf("slices do not share backing array: left len/cap=%d/%d right len/cap=%d/%d", len(left), cap(left), len(right), cap(right))
	}
}

func sameBucketBackingArray(left, right []byte) bool {
	if cap(left) == 0 || cap(right) == 0 {
		return cap(left) == cap(right)
	}

	return &left[:cap(left)][0] == &right[:cap(right)][0]
}

func sameBucketSliceShape(left, right []byte) bool {
	return len(left) == len(right) && cap(left) == cap(right)
}
