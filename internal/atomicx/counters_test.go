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

	"arcoris.dev/bufferpool/internal/testutil"
)

// TestUint64CounterZeroValue verifies that Uint64Counter is usable without
// explicit initialization.
//
// Runtime counter groups should be embeddable directly into shard, class, pool,
// partition, and controller state structs. Requiring explicit initialization for
// every counter field would make construction more error-prone and provide no
// value over the zero-value atomic semantics.
func TestUint64CounterZeroValue(t *testing.T) {
	t.Parallel()

	var counter Uint64Counter

	if got := counter.Load(); got != 0 {
		t.Fatalf("zero-value Uint64Counter Load() = %d, want 0", got)
	}

	snapshot := counter.Snapshot()
	if snapshot.Value != 0 {
		t.Fatalf("zero-value Uint64Counter Snapshot().Value = %d, want 0", snapshot.Value)
	}
}

// TestUint64CounterAddAndInc verifies monotonic counter updates.
//
// Uint64Counter represents lifetime event counts. It intentionally exposes only
// forward-moving operations so ordinary runtime code cannot reset or decrement
// source counters used by workload-window sampling.
func TestUint64CounterAddAndInc(t *testing.T) {
	t.Parallel()

	var counter Uint64Counter

	if got := counter.Add(0); got != 0 {
		t.Fatalf("Add(0) returned %d, want 0", got)
	}

	if got := counter.Inc(); got != 1 {
		t.Fatalf("Inc returned %d, want 1", got)
	}

	if got := counter.Add(9); got != 10 {
		t.Fatalf("Add returned %d, want 10", got)
	}

	if got := counter.Load(); got != 10 {
		t.Fatalf("Load returned %d, want 10", got)
	}
}

// TestUint64CounterWrapsLikeUint64 verifies the deliberate lifetime-counter
// wrap behavior.
//
// Uint64Counter is a monotonic lifetime counter backed by unsigned atomic
// arithmetic. It does not guard overflow because snapshots and deltas are
// explicitly wrap-aware for one wrap between samples.
func TestUint64CounterWrapsLikeUint64(t *testing.T) {
	t.Parallel()

	var counter Uint64Counter

	counter.Add(maxUint64)

	if got := counter.Load(); got != maxUint64 {
		t.Fatalf("Load after Add(max) = %d, want %d", got, maxUint64)
	}

	if got := counter.Inc(); got != 0 {
		t.Fatalf("Inc after max returned %d, want wrap to 0", got)
	}
}

// TestUint64CounterSnapshot verifies that Snapshot captures an immutable
// point-in-time value.
//
// Workload windows should compute deltas from lifetime snapshots instead of
// resetting hot counters. This preserves observability while still allowing the
// control plane to reason about recent activity.
func TestUint64CounterSnapshot(t *testing.T) {
	t.Parallel()

	var counter Uint64Counter

	counter.Add(64)

	first := counter.Snapshot()

	counter.Add(32)

	second := counter.Snapshot()

	if first.Value != 64 {
		t.Fatalf("first snapshot Value = %d, want 64", first.Value)
	}

	if second.Value != 96 {
		t.Fatalf("second snapshot Value = %d, want 96", second.Value)
	}
}

// TestUint64CounterSnapshotDeltaSince verifies ordinary non-wrapped delta
// calculation between monotonic counter snapshots.
//
// This is the expected path for normal controller sampling: a later lifetime
// counter value is greater than or equal to an earlier lifetime counter value.
func TestUint64CounterSnapshotDeltaSince(t *testing.T) {
	t.Parallel()

	previous := Uint64CounterSnapshot{Value: 100}
	current := Uint64CounterSnapshot{Value: 175}

	delta := current.DeltaSince(previous)

	if delta.Previous != 100 {
		t.Fatalf("delta Previous = %d, want 100", delta.Previous)
	}

	if delta.Current != 175 {
		t.Fatalf("delta Current = %d, want 175", delta.Current)
	}

	if delta.Value != 75 {
		t.Fatalf("delta Value = %d, want 75", delta.Value)
	}

	if delta.Wrapped {
		t.Fatal("delta Wrapped = true, want false")
	}

	if delta.IsZero() {
		t.Fatal("delta IsZero() = true, want false")
	}
}

// TestUint64CounterSnapshotDeltaSinceSameValue verifies that equal snapshots
// produce a zero delta.
//
// A zero delta is a valid observation window: no events were observed between
// the two samples.
func TestUint64CounterSnapshotDeltaSinceSameValue(t *testing.T) {
	t.Parallel()

	previous := Uint64CounterSnapshot{Value: 42}
	current := Uint64CounterSnapshot{Value: 42}

	delta := current.DeltaSince(previous)

	if delta.Value != 0 {
		t.Fatalf("delta Value = %d, want 0", delta.Value)
	}

	if delta.Wrapped {
		t.Fatal("delta Wrapped = true, want false")
	}

	if !delta.IsZero() {
		t.Fatal("delta IsZero() = false, want true")
	}
}

// TestNewUint64CounterDeltaWrapAware verifies single-wrap delta calculation.
//
// uint64 subtraction has the desired modulo behavior for one wrap. This matters
// for very long-running processes where lifetime counters are not reset. Multiple
// wraps between two samples cannot be detected from two values and must be
// prevented by reasonable sampling cadence.
func TestNewUint64CounterDeltaWrapAware(t *testing.T) {
	t.Parallel()

	previous := maxUint64 - 2
	current := uint64(4)

	delta := NewUint64CounterDelta(previous, current)

	if delta.Previous != previous {
		t.Fatalf("delta Previous = %d, want %d", delta.Previous, previous)
	}

	if delta.Current != current {
		t.Fatalf("delta Current = %d, want %d", delta.Current, current)
	}

	if delta.Value != 7 {
		t.Fatalf("delta Value = %d, want 7", delta.Value)
	}

	if !delta.Wrapped {
		t.Fatal("delta Wrapped = false, want true")
	}
}

// TestUint64CounterConcurrentInc verifies atomic correctness under concurrent
// increments.
//
// This test does not measure false sharing or performance. It verifies the
// semantic guarantee needed by hot runtime counters: increments from many
// goroutines are not lost.
func TestUint64CounterConcurrentInc(t *testing.T) {
	t.Parallel()

	const goroutines = 16
	const iterations = 4096

	var counter Uint64Counter
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
		t.Fatalf("concurrent Uint64Counter increments = %d, want %d", got, want)
	}
}

// TestUint64GaugeZeroValue verifies that Uint64Gauge is usable without explicit
// initialization.
//
// Non-negative gauges are used for current runtime quantities such as retained
// bytes, retained buffers, active entries, pending trim work, and other values
// that can move up and down but must never become negative.
func TestUint64GaugeZeroValue(t *testing.T) {
	t.Parallel()

	var gauge Uint64Gauge

	if got := gauge.Load(); got != 0 {
		t.Fatalf("zero-value Uint64Gauge Load() = %d, want 0", got)
	}
}

// TestUint64GaugeOperations verifies the ordinary non-negative gauge lifecycle.
//
// Unlike Uint64Counter, Uint64Gauge supports both Add and Sub because gauges
// represent current state, not lifetime event history.
func TestUint64GaugeOperations(t *testing.T) {
	t.Parallel()

	var gauge Uint64Gauge

	gauge.Store(10)
	if got := gauge.Load(); got != 10 {
		t.Fatalf("Load after Store = %d, want 10", got)
	}

	if got := gauge.Add(5); got != 15 {
		t.Fatalf("Add returned %d, want 15", got)
	}

	if got := gauge.Sub(3); got != 12 {
		t.Fatalf("Sub returned %d, want 12", got)
	}

	if got := gauge.Inc(); got != 13 {
		t.Fatalf("Inc returned %d, want 13", got)
	}

	if got := gauge.Dec(); got != 12 {
		t.Fatalf("Dec returned %d, want 12", got)
	}

	if old := gauge.Swap(64); old != 12 {
		t.Fatalf("Swap returned old value %d, want 12", old)
	}

	if got := gauge.Load(); got != 64 {
		t.Fatalf("Load after Swap = %d, want 64", got)
	}

	if swapped := gauge.CompareAndSwap(12, 128); swapped {
		t.Fatal("CompareAndSwap succeeded with wrong old value")
	}

	if got := gauge.Load(); got != 64 {
		t.Fatalf("Load after failed CompareAndSwap = %d, want 64", got)
	}

	if swapped := gauge.CompareAndSwap(64, 128); !swapped {
		t.Fatal("CompareAndSwap failed with correct old value")
	}

	if got := gauge.Load(); got != 128 {
		t.Fatalf("Load after successful CompareAndSwap = %d, want 128", got)
	}
}

// TestUint64GaugeAddZeroAndSubZero verifies zero-delta operations.
//
// Zero-delta operations should be valid no-ops that return the current value.
// This is useful for generic accounting paths where a computed delta may be zero
// after normalization.
func TestUint64GaugeAddZeroAndSubZero(t *testing.T) {
	t.Parallel()

	var gauge Uint64Gauge

	gauge.Store(42)

	if got := gauge.Add(0); got != 42 {
		t.Fatalf("Add(0) returned %d, want 42", got)
	}

	if got := gauge.Sub(0); got != 42 {
		t.Fatalf("Sub(0) returned %d, want 42", got)
	}

	if got := gauge.Load(); got != 42 {
		t.Fatalf("Load after zero-delta operations = %d, want 42", got)
	}
}

// TestUint64GaugeBoundaryOperations verifies exact boundary transitions.
//
// A gauge may legitimately reach math.MaxUint64 or zero. It should panic only
// when an operation would cross those boundaries.
func TestUint64GaugeBoundaryOperations(t *testing.T) {
	t.Parallel()

	var gauge Uint64Gauge

	if got := gauge.Add(maxUint64); got != maxUint64 {
		t.Fatalf("Add(max) returned %d, want %d", got, maxUint64)
	}

	if got := gauge.Sub(maxUint64); got != 0 {
		t.Fatalf("Sub(max) returned %d, want 0", got)
	}
}

// TestUint64GaugePanicsOnOverflow verifies that non-negative gauges do not
// silently wrap on addition.
//
// Silent uint64 wraparound would corrupt memory accounting. For example,
// retained bytes could appear smaller after crossing the maximum value, hiding a
// severe accounting bug.
func TestUint64GaugePanicsOnOverflow(t *testing.T) {
	t.Parallel()

	var gauge Uint64Gauge

	gauge.Store(maxUint64)

	testutil.MustPanicWithMessage(t, errUint64GaugeOverflow, func() {
		_ = gauge.Add(1)
	})
}

// TestUint64GaugePanicsOnUnderflow verifies that non-negative gauges reject
// subtraction below zero.
//
// Underflow means the caller removed, released, trimmed, or decremented more
// state than had been accounted.
func TestUint64GaugePanicsOnUnderflow(t *testing.T) {
	t.Parallel()

	var gauge Uint64Gauge

	gauge.Store(10)

	testutil.MustPanicWithMessage(t, errUint64GaugeUnderflow, func() {
		_ = gauge.Sub(11)
	})
}

// TestUint64GaugeConcurrentBalancedAccounting verifies atomic correctness for a
// gauge that moves up and down concurrently.
//
// Each iteration adds two and subtracts one, producing a net increase of one.
// This models common runtime accounting where retained bytes or active counts
// are updated from multiple goroutines.
func TestUint64GaugeConcurrentBalancedAccounting(t *testing.T) {
	t.Parallel()

	const goroutines = 16
	const iterations = 4096

	var gauge Uint64Gauge
	var wg sync.WaitGroup

	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				gauge.Add(2)
				gauge.Sub(1)
			}
		}()
	}

	wg.Wait()

	want := uint64(goroutines * iterations)
	if got := gauge.Load(); got != want {
		t.Fatalf("concurrent Uint64Gauge updates = %d, want %d", got, want)
	}
}

// TestInt64GaugeZeroValue verifies that Int64Gauge is usable without explicit
// initialization.
//
// Signed gauges are used for runtime values where negative states are valid,
// such as correction deltas or signed budget movement.
func TestInt64GaugeZeroValue(t *testing.T) {
	t.Parallel()

	var gauge Int64Gauge

	if got := gauge.Load(); got != 0 {
		t.Fatalf("zero-value Int64Gauge Load() = %d, want 0", got)
	}
}

// TestInt64GaugeOperations verifies signed gauge behavior.
//
// Int64Gauge supports positive and negative values, while still guarding against
// signed overflow and underflow.
func TestInt64GaugeOperations(t *testing.T) {
	t.Parallel()

	var gauge Int64Gauge

	gauge.Store(-10)
	if got := gauge.Load(); got != -10 {
		t.Fatalf("Load after Store = %d, want -10", got)
	}

	if got := gauge.Add(15); got != 5 {
		t.Fatalf("Add returned %d, want 5", got)
	}

	if got := gauge.Inc(); got != 6 {
		t.Fatalf("Inc returned %d, want 6", got)
	}

	if got := gauge.Dec(); got != 5 {
		t.Fatalf("Dec returned %d, want 5", got)
	}

	if old := gauge.Swap(-64); old != 5 {
		t.Fatalf("Swap returned old value %d, want 5", old)
	}

	if got := gauge.Load(); got != -64 {
		t.Fatalf("Load after Swap = %d, want -64", got)
	}

	if swapped := gauge.CompareAndSwap(5, 128); swapped {
		t.Fatal("CompareAndSwap succeeded with wrong old value")
	}

	if got := gauge.Load(); got != -64 {
		t.Fatalf("Load after failed CompareAndSwap = %d, want -64", got)
	}

	if swapped := gauge.CompareAndSwap(-64, 128); !swapped {
		t.Fatal("CompareAndSwap failed with correct old value")
	}

	if got := gauge.Load(); got != 128 {
		t.Fatalf("Load after successful CompareAndSwap = %d, want 128", got)
	}
}

// TestInt64GaugeAddZero verifies that a zero signed delta is a valid no-op.
//
// Generic controller and budget code may compute a zero correction. The atomic
// wrapper should return the current value without changing state.
func TestInt64GaugeAddZero(t *testing.T) {
	t.Parallel()

	var gauge Int64Gauge

	gauge.Store(-42)

	if got := gauge.Add(0); got != -42 {
		t.Fatalf("Add(0) returned %d, want -42", got)
	}

	if got := gauge.Load(); got != -42 {
		t.Fatalf("Load after Add(0) = %d, want -42", got)
	}
}

// TestInt64GaugeBoundaryOperations verifies exact signed boundary transitions.
//
// A signed gauge may legitimately reach minInt64 or maxInt64. It should panic
// only when an operation would move outside the int64 range.
func TestInt64GaugeBoundaryOperations(t *testing.T) {
	t.Parallel()

	var gauge Int64Gauge

	if got := gauge.Add(maxInt64); got != maxInt64 {
		t.Fatalf("Add(maxInt64) returned %d, want %d", got, maxInt64)
	}

	if got := gauge.Add(minInt64); got != -1 {
		t.Fatalf("Add(minInt64) from maxInt64 returned %d, want -1", got)
	}

	if got := gauge.Dec(); got != -2 {
		t.Fatalf("Dec after boundary transition returned %d, want -2", got)
	}
}

// TestInt64GaugePanicsOnOverflow verifies that signed gauges reject positive
// overflow.
//
// Signed wraparound would corrupt control-plane correction state and can turn a
// large positive value into a negative one.
func TestInt64GaugePanicsOnOverflow(t *testing.T) {
	t.Parallel()

	var gauge Int64Gauge

	gauge.Store(maxInt64)

	testutil.MustPanicWithMessage(t, errInt64GaugeOverflow, func() {
		_ = gauge.Add(1)
	})
}

// TestInt64GaugePanicsOnUnderflow verifies that signed gauges reject negative
// overflow.
//
// This protects signed deltas and correction values from wrapping around into
// large positive values.
func TestInt64GaugePanicsOnUnderflow(t *testing.T) {
	t.Parallel()

	var gauge Int64Gauge

	gauge.Store(minInt64)

	testutil.MustPanicWithMessage(t, errInt64GaugeUnderflow, func() {
		_ = gauge.Add(-1)
	})
}

// TestInt64GaugeConcurrentSignedUpdates verifies atomic correctness under
// concurrent signed updates.
//
// Each iteration applies Add(2) and Dec(), giving a net +1. This exercises both
// positive and negative update paths while keeping the expected final value
// deterministic.
func TestInt64GaugeConcurrentSignedUpdates(t *testing.T) {
	t.Parallel()

	const goroutines = 16
	const iterations = 4096

	var gauge Int64Gauge
	var wg sync.WaitGroup

	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				gauge.Add(2)
				gauge.Dec()
			}
		}()
	}

	wg.Wait()

	want := int64(goroutines * iterations)
	if got := gauge.Load(); got != want {
		t.Fatalf("concurrent Int64Gauge updates = %d, want %d", got, want)
	}
}
