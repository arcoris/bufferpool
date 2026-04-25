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

package atomicx

import (
	"sync"
	"testing"
	"unsafe"
)

// TestCacheLinePadLayout verifies the basic layout contract of CacheLinePad.
//
// CacheLinePad is a pure layout type. It has no behavior, but its size is part
// of the package's false-sharing mitigation strategy. If this test fails, the
// padded atomic wrappers no longer have the expected physical separation from
// adjacent fields.
func TestCacheLinePadLayout(t *testing.T) {
	t.Parallel()

	size := unsafe.Sizeof(CacheLinePad{})
	if size != CacheLinePadSize {
		t.Fatalf("unsafe.Sizeof(CacheLinePad{}) = %d, want %d", size, CacheLinePadSize)
	}

	if CacheLinePadSize < 64 {
		t.Fatalf("CacheLinePadSize = %d, want at least 64", CacheLinePadSize)
	}
}

// TestPaddedUint64Layout verifies that PaddedUint64 keeps its atomic value
// separated from surrounding fields by leading and trailing padding.
//
// This test does not prove the absence of false sharing on every CPU. That is a
// performance property and must be measured with benchmarks. The test only
// protects the intended struct layout from accidental refactors that remove or
// shrink padding.
func TestPaddedUint64Layout(t *testing.T) {
	t.Parallel()

	var counter PaddedUint64

	valueOffset := unsafe.Offsetof(counter.value)
	valueSize := unsafe.Sizeof(counter.value)
	totalSize := unsafe.Sizeof(counter)

	if valueOffset < CacheLinePadSize {
		t.Fatalf("PaddedUint64.value offset = %d, want at least %d", valueOffset, CacheLinePadSize)
	}

	trailingPaddingStart := valueOffset + valueSize
	minTotalSize := trailingPaddingStart + CacheLinePadSize

	if totalSize < minTotalSize {
		t.Fatalf("unsafe.Sizeof(PaddedUint64{}) = %d, want at least %d", totalSize, minTotalSize)
	}
}

// TestPaddedInt64Layout verifies that PaddedInt64 keeps its atomic value
// separated from surrounding fields by leading and trailing padding.
//
// Signed and unsigned padded atomics must follow the same layout rule because
// both may be used for hot shared runtime state.
func TestPaddedInt64Layout(t *testing.T) {
	t.Parallel()

	var counter PaddedInt64

	valueOffset := unsafe.Offsetof(counter.value)
	valueSize := unsafe.Sizeof(counter.value)
	totalSize := unsafe.Sizeof(counter)

	if valueOffset < CacheLinePadSize {
		t.Fatalf("PaddedInt64.value offset = %d, want at least %d", valueOffset, CacheLinePadSize)
	}

	trailingPaddingStart := valueOffset + valueSize
	minTotalSize := trailingPaddingStart + CacheLinePadSize

	if totalSize < minTotalSize {
		t.Fatalf("unsafe.Sizeof(PaddedInt64{}) = %d, want at least %d", totalSize, minTotalSize)
	}
}

// TestPaddedUint64ZeroValue verifies that the zero value is immediately usable.
//
// This matters because counter structs should be embeddable in runtime state
// without requiring explicit initialization code for every atomic field.
func TestPaddedUint64ZeroValue(t *testing.T) {
	t.Parallel()

	var counter PaddedUint64

	if got := counter.Load(); got != 0 {
		t.Fatalf("zero-value PaddedUint64 Load() = %d, want 0", got)
	}
}

// TestPaddedUint64Operations verifies the public atomic operations exposed by
// PaddedUint64.
//
// The wrapper should preserve the expected semantics of sync/atomic.Uint64 while
// hiding the underlying atomic field behind the padded layout.
func TestPaddedUint64Operations(t *testing.T) {
	t.Parallel()

	var counter PaddedUint64

	counter.Store(10)
	if got := counter.Load(); got != 10 {
		t.Fatalf("Load after Store = %d, want 10", got)
	}

	if got := counter.Add(5); got != 15 {
		t.Fatalf("Add returned %d, want 15", got)
	}

	if got := counter.Inc(); got != 16 {
		t.Fatalf("Inc returned %d, want 16", got)
	}

	if old := counter.Swap(32); old != 16 {
		t.Fatalf("Swap returned old value %d, want 16", old)
	}

	if got := counter.Load(); got != 32 {
		t.Fatalf("Load after Swap = %d, want 32", got)
	}

	if swapped := counter.CompareAndSwap(16, 64); swapped {
		t.Fatal("CompareAndSwap succeeded with wrong old value")
	}

	if got := counter.Load(); got != 32 {
		t.Fatalf("Load after failed CompareAndSwap = %d, want 32", got)
	}

	if swapped := counter.CompareAndSwap(32, 64); !swapped {
		t.Fatal("CompareAndSwap failed with correct old value")
	}

	if got := counter.Load(); got != 64 {
		t.Fatalf("Load after successful CompareAndSwap = %d, want 64", got)
	}
}

// TestPaddedUint64ConcurrentAdd verifies that PaddedUint64 behaves correctly
// under concurrent increments.
//
// This test checks atomic correctness, not performance. It ensures that the
// wrapper can be used for hot counters such as shard gets, hits, misses, drops,
// allocations, retained buffers, and trim counters.
func TestPaddedUint64ConcurrentAdd(t *testing.T) {
	t.Parallel()

	const goroutines = 16
	const iterations = 4096

	var counter PaddedUint64
	var wg sync.WaitGroup

	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				counter.Inc()
			}
		}()
	}

	wg.Wait()

	want := uint64(goroutines * iterations)
	if got := counter.Load(); got != want {
		t.Fatalf("concurrent PaddedUint64 increments = %d, want %d", got, want)
	}
}

// TestPaddedInt64ZeroValue verifies that the zero value is immediately usable.
//
// Signed padded atomics may be used for deltas, correction values, signed gauges,
// and other runtime state where negative values are meaningful.
func TestPaddedInt64ZeroValue(t *testing.T) {
	t.Parallel()

	var counter PaddedInt64

	if got := counter.Load(); got != 0 {
		t.Fatalf("zero-value PaddedInt64 Load() = %d, want 0", got)
	}
}

// TestPaddedInt64Operations verifies the public atomic operations exposed by
// PaddedInt64.
//
// The wrapper should preserve the expected semantics of sync/atomic.Int64 while
// keeping callers independent of the underlying atomic field.
func TestPaddedInt64Operations(t *testing.T) {
	t.Parallel()

	var counter PaddedInt64

	counter.Store(-10)
	if got := counter.Load(); got != -10 {
		t.Fatalf("Load after Store = %d, want -10", got)
	}

	if got := counter.Add(15); got != 5 {
		t.Fatalf("Add returned %d, want 5", got)
	}

	if got := counter.Inc(); got != 6 {
		t.Fatalf("Inc returned %d, want 6", got)
	}

	if got := counter.Dec(); got != 5 {
		t.Fatalf("Dec returned %d, want 5", got)
	}

	if old := counter.Swap(-32); old != 5 {
		t.Fatalf("Swap returned old value %d, want 5", old)
	}

	if got := counter.Load(); got != -32 {
		t.Fatalf("Load after Swap = %d, want -32", got)
	}

	if swapped := counter.CompareAndSwap(5, 64); swapped {
		t.Fatal("CompareAndSwap succeeded with wrong old value")
	}

	if got := counter.Load(); got != -32 {
		t.Fatalf("Load after failed CompareAndSwap = %d, want -32", got)
	}

	if swapped := counter.CompareAndSwap(-32, 64); !swapped {
		t.Fatal("CompareAndSwap failed with correct old value")
	}

	if got := counter.Load(); got != 64 {
		t.Fatalf("Load after successful CompareAndSwap = %d, want 64", got)
	}
}

// TestPaddedInt64ConcurrentAdd verifies that PaddedInt64 behaves correctly under
// concurrent signed updates.
//
// Each loop performs Add(2) and Dec(), giving a net +1 per iteration. This
// exercises both positive and negative signed updates while keeping the final
// expected value simple and deterministic.
func TestPaddedInt64ConcurrentAdd(t *testing.T) {
	t.Parallel()

	const goroutines = 16
	const iterations = 4096

	var counter PaddedInt64
	var wg sync.WaitGroup

	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				counter.Add(2)
				counter.Dec()
			}
		}()
	}

	wg.Wait()

	want := int64(goroutines * iterations)
	if got := counter.Load(); got != want {
		t.Fatalf("concurrent PaddedInt64 updates = %d, want %d", got, want)
	}
}
