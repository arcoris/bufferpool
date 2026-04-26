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
	"time"

	"arcoris.dev/bufferpool/internal/testutil"
)

// TestNewBucket verifies construction of an enabled synchronized bucket.
//
// newBucket should create a bucket with an enabled bucketSegment. The bucket
// starts empty, has all slots available, and retains no bytes.
func TestNewBucket(t *testing.T) {
	t.Parallel()

	b := newBucket(4)

	assertBucketState(t, b.state(), bucketState{
		SlotLimit:      4,
		AvailableSlots: 4,
	})
}

// TestNewBucketPanicsForInvalidSlotLimit verifies constructor validation.
//
// Explicit bucket construction is for enabled buckets. A zero-value bucket is a
// disabled bucket, but newBucket(0) or newBucket(-1) is a construction bug.
func TestNewBucketPanicsForInvalidSlotLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// slotLimit is the invalid enabled-bucket slot limit.
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
				_ = newBucket(tt.slotLimit)
			})
		})
	}
}

// TestZeroValueBucketIsDisabled verifies zero-value bucket behavior.
//
// A zero-value bucket is safe to inspect and pop from, but it has no storage
// slots and therefore rejects retention. This is useful for disabled buckets or
// staged construction.
func TestZeroValueBucketIsDisabled(t *testing.T) {
	t.Parallel()

	var b bucket

	if stored := b.tryPush(make([]byte, 0, 8)); stored {
		t.Fatal("zero-value bucket tryPush returned true, want false")
	}

	buffer, ok := b.tryPop()
	if ok {
		t.Fatal("zero-value bucket tryPop returned ok=true, want false")
	}

	if buffer != nil {
		t.Fatalf("zero-value bucket tryPop returned buffer=%v, want nil", buffer)
	}

	assertBucketState(t, b.state(), bucketState{})
}

// TestBucketTryPushStoresBuffer verifies ordinary bucket retention.
//
// tryPush should serialize access, delegate storage to bucketSegment, and update
// bucket state through segment accounting. Retained memory is based on capacity,
// not length.
func TestBucketTryPushStoresBuffer(t *testing.T) {
	t.Parallel()

	b := newBucket(2)

	buffer := make([]byte, 7, 16)

	if stored := b.tryPush(buffer); !stored {
		t.Fatal("tryPush returned false, want true")
	}

	assertBucketState(t, b.state(), bucketState{
		RetainedBuffers: 1,
		RetainedBytes:   16,
		SlotLimit:       2,
		AvailableSlots:  1,
	})
}

// TestBucketTryPushRejectsZeroCapacityBuffers verifies that bucket preserves
// bucketSegment retention semantics.
//
// Nil and zero-capacity slices cannot provide reusable storage and should be
// rejected without changing bucket state.
func TestBucketTryPushRejectsZeroCapacityBuffers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// buffer is the invalid retention candidate.
		buffer []byte
	}{
		{
			name:   "nil buffer",
			buffer: nil,
		},
		{
			name:   "zero-capacity empty buffer",
			buffer: make([]byte, 0),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBucket(1)

			if stored := b.tryPush(tt.buffer); stored {
				t.Fatal("tryPush returned true, want false")
			}

			assertBucketState(t, b.state(), bucketState{
				SlotLimit:      1,
				AvailableSlots: 1,
			})
		})
	}
}

// TestBucketTryPushRejectsFullBucket verifies bounded retention.
//
// Once all bucket slots are occupied, tryPush should reject additional buffers
// without changing retained state.
func TestBucketTryPushRejectsFullBucket(t *testing.T) {
	t.Parallel()

	b := newBucket(1)

	first := make([]byte, 0, 8)
	second := make([]byte, 0, 16)

	if stored := b.tryPush(first); !stored {
		t.Fatal("first tryPush returned false, want true")
	}

	if stored := b.tryPush(second); stored {
		t.Fatal("second tryPush into full bucket returned true, want false")
	}

	assertBucketState(t, b.state(), bucketState{
		RetainedBuffers: 1,
		RetainedBytes:   8,
		SlotLimit:       1,
	})
}

// TestBucketTryPopReturnsLastBuffer verifies LIFO reuse behavior.
//
// bucket should preserve bucketSegment's LIFO semantics while adding
// synchronization. Returned buffers must have len == 0 and preserve capacity.
func TestBucketTryPopReturnsLastBuffer(t *testing.T) {
	t.Parallel()

	b := newBucket(3)

	first := make([]byte, 3, 8)
	second := make([]byte, 5, 16)
	third := make([]byte, 7, 32)

	if !b.tryPush(first) {
		t.Fatal("tryPush first returned false, want true")
	}

	if !b.tryPush(second) {
		t.Fatal("tryPush second returned false, want true")
	}

	if !b.tryPush(third) {
		t.Fatal("tryPush third returned false, want true")
	}

	tests := []struct {
		name string

		// wantCapacity is the capacity expected from the popped buffer.
		wantCapacity int

		// wantRetainedBytes is the retained byte count after the pop.
		wantRetainedBytes uint64

		// wantRetainedBuffers is the retained buffer count after the pop.
		wantRetainedBuffers int
	}{
		{
			name:                "third buffer first",
			wantCapacity:        32,
			wantRetainedBytes:   24,
			wantRetainedBuffers: 2,
		},
		{
			name:                "second buffer second",
			wantCapacity:        16,
			wantRetainedBytes:   8,
			wantRetainedBuffers: 1,
		},
		{
			name:                "first buffer last",
			wantCapacity:        8,
			wantRetainedBytes:   0,
			wantRetainedBuffers: 0,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			buffer, ok := b.tryPop()
			if !ok {
				t.Fatal("tryPop returned ok=false, want true")
			}

			if got := len(buffer); got != 0 {
				t.Fatalf("len(popped buffer) = %d, want 0", got)
			}

			if got := cap(buffer); got != tt.wantCapacity {
				t.Fatalf("cap(popped buffer) = %d, want %d", got, tt.wantCapacity)
			}

			state := b.state()

			if state.RetainedBytes != tt.wantRetainedBytes {
				t.Fatalf("RetainedBytes after tryPop = %d, want %d", state.RetainedBytes, tt.wantRetainedBytes)
			}

			if state.RetainedBuffers != tt.wantRetainedBuffers {
				t.Fatalf("RetainedBuffers after tryPop = %d, want %d", state.RetainedBuffers, tt.wantRetainedBuffers)
			}
		})
	}
}

// TestBucketTryPopEmpty verifies empty reuse behavior.
//
// Empty pop should be a non-panicking miss. Higher layers can classify this as a
// bucket miss and continue to allocation or fallback paths.
func TestBucketTryPopEmpty(t *testing.T) {
	t.Parallel()

	b := newBucket(1)

	buffer, ok := b.tryPop()
	if ok {
		t.Fatal("tryPop from empty bucket returned ok=true, want false")
	}

	if buffer != nil {
		t.Fatalf("tryPop from empty bucket returned buffer=%v, want nil", buffer)
	}

	assertBucketState(t, b.state(), bucketState{
		SlotLimit:      1,
		AvailableSlots: 1,
	})
}

// TestBucketInspectionHelpers verifies that helper methods reflect the same
// state as bucket.state().
//
// These helpers are small locked reads intended for tests and local internal
// inspection paths.
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

	if !b.tryPush(make([]byte, 0, 8)) {
		t.Fatal("tryPush returned false, want true")
	}

	if b.len() != 1 {
		t.Fatalf("len() after push = %d, want 1", b.len())
	}

	if b.retained() != 8 {
		t.Fatalf("retained() after push = %d, want 8", b.retained())
	}

	if b.slotLimit() != 2 {
		t.Fatalf("slotLimit() after push = %d, want 2", b.slotLimit())
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

// TestBucketStateIsPointInTime verifies that state is a copied value, not a
// live view into bucket storage.
func TestBucketStateIsPointInTime(t *testing.T) {
	t.Parallel()

	b := newBucket(2)
	if !b.tryPush(make([]byte, 0, 8)) {
		t.Fatal("tryPush returned false, want true")
	}

	beforePop := b.state()

	if _, ok := b.tryPop(); !ok {
		t.Fatal("tryPop returned ok=false, want true")
	}

	assertBucketState(t, beforePop, bucketState{
		RetainedBuffers: 1,
		RetainedBytes:   8,
		SlotLimit:       2,
		AvailableSlots:  1,
	})
	assertBucketState(t, b.state(), bucketState{
		SlotLimit:      2,
		AvailableSlots: 2,
	})
}

// TestBucketStatePredicates verifies bucketState helper semantics.
//
// bucketState is an immutable value object. Its helper methods should be value
// receiver methods and should operate only on the captured state fields.
func TestBucketStatePredicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// state is the immutable bucket state being inspected.
		state bucketState

		wantEmpty        bool
		wantFull         bool
		wantHasAvailable bool
	}{
		{
			name: "disabled empty bucket",
			state: bucketState{
				RetainedBuffers: 0,
				RetainedBytes:   0,
				SlotLimit:       0,
				AvailableSlots:  0,
			},
			wantEmpty:        true,
			wantFull:         true,
			wantHasAvailable: false,
		},
		{
			name: "enabled empty bucket",
			state: bucketState{
				RetainedBuffers: 0,
				RetainedBytes:   0,
				SlotLimit:       2,
				AvailableSlots:  2,
			},
			wantEmpty:        true,
			wantFull:         false,
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
			wantEmpty:        false,
			wantFull:         false,
			wantHasAvailable: true,
		},
		{
			name: "full bucket",
			state: bucketState{
				RetainedBuffers: 2,
				RetainedBytes:   24,
				SlotLimit:       2,
				AvailableSlots:  0,
			},
			wantEmpty:        false,
			wantFull:         true,
			wantHasAvailable: false,
		},
	}

	for _, tt := range tests {
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

// TestBucketUnlocksAfterSegmentPanic verifies that bucket methods release the
// mutex even when the underlying segment rejects corrupted state.
func TestBucketUnlocksAfterSegmentPanic(t *testing.T) {
	t.Parallel()

	stale := make([]byte, 0, 8)
	b := bucket{
		segment: bucketSegment{
			slots: [][]byte{stale},
		},
	}

	testutil.MustPanicWithMessage(t, errBucketSegmentNonEmptyFreeSlot, func() {
		_ = b.tryPush(make([]byte, 0, 16))
	})

	states := make(chan bucketState, 1)
	go func() {
		states <- b.state()
	}()

	select {
	case state := <-states:
		assertBucketState(t, state, bucketState{
			SlotLimit:      1,
			AvailableSlots: 1,
		})
		assertSameBucketSegmentBackingArray(t, b.segment.slots[0], stale)

	case <-time.After(time.Second):
		t.Fatal("bucket mutex remained locked after segment panic")
	}
}

// TestBucketConcurrentPush verifies that bucket serializes concurrent retention.
//
// The bucket should not store more buffers than its slot limit even when many
// goroutines attempt to push concurrently.
func TestBucketConcurrentPush(t *testing.T) {
	t.Parallel()

	const slotLimit = 64
	const goroutines = 16
	const attemptsPerGoroutine = 64

	b := newBucket(slotLimit)

	var wg sync.WaitGroup
	var successMu sync.Mutex
	var successes int

	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < attemptsPerGoroutine; j++ {
				if b.tryPush(make([]byte, 0, 8)) {
					successMu.Lock()
					successes++
					successMu.Unlock()
				}
			}
		}()
	}

	wg.Wait()

	if successes != slotLimit {
		t.Fatalf("successful concurrent pushes = %d, want %d", successes, slotLimit)
	}

	assertBucketState(t, b.state(), bucketState{
		RetainedBuffers: slotLimit,
		RetainedBytes:   uint64(slotLimit * 8),
		SlotLimit:       slotLimit,
	})
}

// TestBucketConcurrentPop verifies that bucket serializes concurrent reuse.
//
// The bucket should return each retained buffer at most once and should become
// empty after all retained buffers are popped.
func TestBucketConcurrentPop(t *testing.T) {
	t.Parallel()

	const slotLimit = 64
	const goroutines = 16
	const attemptsPerGoroutine = 64

	b := newBucket(slotLimit)

	for i := 0; i < slotLimit; i++ {
		if !b.tryPush(make([]byte, 0, 8)) {
			t.Fatalf("initial tryPush %d returned false, want true", i)
		}
	}

	var wg sync.WaitGroup
	var successMu sync.Mutex
	var successes int

	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < attemptsPerGoroutine; j++ {
				if _, ok := b.tryPop(); ok {
					successMu.Lock()
					successes++
					successMu.Unlock()
				}
			}
		}()
	}

	wg.Wait()

	if successes != slotLimit {
		t.Fatalf("successful concurrent pops = %d, want %d", successes, slotLimit)
	}

	assertBucketState(t, b.state(), bucketState{
		SlotLimit:      slotLimit,
		AvailableSlots: slotLimit,
	})
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
