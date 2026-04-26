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

package clock

import (
	"sync"
	"testing"
	"time"

	"arcoris.dev/bufferpool/internal/testutil"
)

// Compile-time interface check.
//
// Manual is mutable and contains a mutex, therefore only *Manual should satisfy
// Clock. This protects the intended usage shape and prevents accidental value
// copying in code that depends on the Clock contract.
var _ Clock = (*Manual)(nil)

// TestNewManualInitializesTime verifies explicit deterministic construction.
//
// NewManual should preserve the exact non-zero timestamp supplied by the caller.
// This is important for tests that need stable controller, workload, EWMA,
// idle-expiry, trim-cadence, or lease timing.
func TestNewManualInitializesTime(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.April, 26, 12, 30, 0, 123, time.UTC)

	manual := NewManual(start)

	got := manual.Now()
	if !got.Equal(start) {
		t.Fatalf("NewManual(start).Now() = %s, want %s", got, start)
	}
}

// TestNewManualPanicsForZeroTime verifies explicit constructor validation.
//
// A zero timestamp is used by clock helpers as a missing-initialization sentinel.
// Passing it explicitly to NewManual should fail fast instead of creating a
// manual clock that later fails elapsed-time validation for an unrelated reason.
func TestNewManualPanicsForZeroTime(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errManualZeroTime, func() {
		_ = NewManual(time.Time{})
	})
}

// TestManualZeroValueIsUsable verifies Go zero-value usability.
//
// Manual may be embedded in test fixtures without explicit construction. On
// first use, it should initialize itself to a fixed non-zero deterministic time.
func TestManualZeroValueIsUsable(t *testing.T) {
	t.Parallel()

	var manual Manual

	got := manual.Now()
	if !got.Equal(manualDefaultTime) {
		t.Fatalf("zero-value Manual Now() = %s, want %s", got, manualDefaultTime)
	}

	if got.IsZero() {
		t.Fatal("zero-value Manual Now() returned zero time")
	}
}

// TestManualNowDoesNotAdvanceTime verifies that Now is a read operation.
//
// Manual time should move only through Set, Advance, or Add. Repeated Now calls
// must return the same timestamp so tests can reason about elapsed-time behavior
// precisely.
func TestManualNowDoesNotAdvanceTime(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.April, 26, 12, 30, 0, 0, time.UTC)
	manual := NewManual(start)

	first := manual.Now()
	second := manual.Now()

	if !first.Equal(second) {
		t.Fatalf("two Now() calls returned different values: first=%s second=%s", first, second)
	}
}

// TestManualSetMovesTimeForward verifies monotonic Set behavior.
//
// Set should allow moving the manual clock forward to an exact timestamp. This
// is useful when a test needs a precise instant rather than relative movement by
// duration.
func TestManualSetMovesTimeForward(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.April, 26, 12, 30, 0, 0, time.UTC)
	next := start.Add(10 * time.Second)

	manual := NewManual(start)
	manual.Set(next)

	got := manual.Now()
	if !got.Equal(next) {
		t.Fatalf("Manual.Set(next); Now() = %s, want %s", got, next)
	}
}

// TestManualSetAllowsEqualTime verifies that setting the same timestamp is a
// valid no-op.
//
// Equal timestamps are useful in tests that publish or normalize exact time
// without advancing elapsed duration.
func TestManualSetAllowsEqualTime(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.April, 26, 12, 30, 0, 0, time.UTC)

	manual := NewManual(start)
	manual.Set(start)

	got := manual.Now()
	if !got.Equal(start) {
		t.Fatalf("Manual.Set(start); Now() = %s, want %s", got, start)
	}
}

// TestManualSetInitializesZeroValue verifies that Set can initialize a zero
// value Manual directly.
//
// This gives tests two valid initialization paths: lazy default initialization
// through Now/Advance, or explicit initialization through Set.
func TestManualSetInitializesZeroValue(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.April, 26, 12, 30, 0, 0, time.UTC)

	var manual Manual
	manual.Set(start)

	got := manual.Now()
	if !got.Equal(start) {
		t.Fatalf("zero-value Manual after Set(start) Now() = %s, want %s", got, start)
	}
}

// TestManualSetPanicsForZeroTime verifies Set validation.
//
// Explicitly setting zero would create timestamps that conflict with the clock
// package invariant that zero time means missing initialization.
func TestManualSetPanicsForZeroTime(t *testing.T) {
	t.Parallel()

	manual := NewManual(time.Date(2026, time.April, 26, 12, 30, 0, 0, time.UTC))

	testutil.MustPanicWithMessage(t, errManualZeroTime, func() {
		manual.Set(time.Time{})
	})
}

// TestManualSetPanicsWhenMovingBackwards verifies monotonicity.
//
// Manual is deterministic but still monotonic. Moving backwards would make
// elapsed-time calculations negative and would corrupt controller cadence, EWMA
// decay, workload windows, lease hold duration, snapshot age, idle expiry, and
// trim scheduling.
func TestManualSetPanicsWhenMovingBackwards(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.April, 26, 12, 30, 0, 0, time.UTC)

	manual := NewManual(start)

	testutil.MustPanicWithMessage(t, errManualBackwardTime, func() {
		manual.Set(start.Add(-time.Nanosecond))
	})
}

// TestManualAdvanceMovesTimeForward verifies relative time movement.
//
// Advance is the main primitive for deterministic tests that need to simulate
// elapsed time without sleeping.
func TestManualAdvanceMovesTimeForward(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.April, 26, 12, 30, 0, 0, time.UTC)

	manual := NewManual(start)

	got := manual.Advance(250 * time.Millisecond)
	want := start.Add(250 * time.Millisecond)

	if !got.Equal(want) {
		t.Fatalf("Manual.Advance returned %s, want %s", got, want)
	}

	if now := manual.Now(); !now.Equal(want) {
		t.Fatalf("Manual.Now() after Advance = %s, want %s", now, want)
	}
}

// TestManualAdvanceAllowsZeroDuration verifies zero-duration no-op behavior.
//
// A zero advance can happen in generic test paths where elapsed duration is
// computed or normalized before being applied. It should be valid and should not
// change the current timestamp.
func TestManualAdvanceAllowsZeroDuration(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.April, 26, 12, 30, 0, 0, time.UTC)

	manual := NewManual(start)

	got := manual.Advance(0)
	if !got.Equal(start) {
		t.Fatalf("Manual.Advance(0) returned %s, want %s", got, start)
	}

	if now := manual.Now(); !now.Equal(start) {
		t.Fatalf("Manual.Now() after Advance(0) = %s, want %s", now, start)
	}
}

// TestManualAdvanceInitializesZeroValue verifies lazy initialization through
// relative movement.
//
// If a zero-value Manual is advanced before Now or Set, it should initialize to
// manualDefaultTime and then apply the requested duration.
func TestManualAdvanceInitializesZeroValue(t *testing.T) {
	t.Parallel()

	var manual Manual

	got := manual.Advance(5 * time.Second)
	want := manualDefaultTime.Add(5 * time.Second)

	if !got.Equal(want) {
		t.Fatalf("zero-value Manual Advance returned %s, want %s", got, want)
	}

	if now := manual.Now(); !now.Equal(want) {
		t.Fatalf("zero-value Manual Now() after Advance = %s, want %s", now, want)
	}
}

// TestManualAdvancePanicsForNegativeDuration verifies monotonic advance
// validation.
//
// Negative advances are rejected because tests should not be able to move time
// backwards through a relative operation.
func TestManualAdvancePanicsForNegativeDuration(t *testing.T) {
	t.Parallel()

	manual := NewManual(time.Date(2026, time.April, 26, 12, 30, 0, 0, time.UTC))

	testutil.MustPanicWithMessage(t, errManualNegativeAdvance, func() {
		_ = manual.Advance(-time.Nanosecond)
	})
}

// TestManualAddAliasesAdvance verifies that Add follows Advance semantics.
//
// Add exists only as a readability alias for tests that are written as direct
// time arithmetic. It should return the same result and update the same state as
// Advance.
func TestManualAddAliasesAdvance(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.April, 26, 12, 30, 0, 0, time.UTC)

	manual := NewManual(start)

	got := manual.Add(3 * time.Second)
	want := start.Add(3 * time.Second)

	if !got.Equal(want) {
		t.Fatalf("Manual.Add returned %s, want %s", got, want)
	}

	if now := manual.Now(); !now.Equal(want) {
		t.Fatalf("Manual.Now() after Add = %s, want %s", now, want)
	}
}

// TestManualWorksWithClockHelpers verifies integration with package-level time
// helpers.
//
// This confirms that Manual can be used through the same Clock-dependent helper
// paths as Real, while keeping time deterministic.
func TestManualWorksWithClockHelpers(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.April, 26, 12, 30, 0, 0, time.UTC)

	manual := NewManual(start)

	manual.Advance(750 * time.Millisecond)

	got := Since(manual, start)
	want := 750 * time.Millisecond

	if got != want {
		t.Fatalf("Since(manual, start) = %s, want %s", got, want)
	}

	if !HasElapsed(start, manual.Now(), 750*time.Millisecond) {
		t.Fatal("HasElapsed(start, manual.Now(), 750ms) = false, want true")
	}
}

// TestManualNilReceiverPanics verifies explicit nil-receiver diagnostics.
//
// Calling methods on nil pointer receivers is legal in Go. Manual deliberately
// checks this case and panics with a stable package-specific message instead of
// allowing a generic nil-pointer panic.
func TestManualNilReceiverPanics(t *testing.T) {
	t.Parallel()

	var manual *Manual

	t.Run("Now", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNilManual, func() {
			_ = manual.Now()
		})
	})

	t.Run("Set", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNilManual, func() {
			manual.Set(time.Date(2026, time.April, 26, 12, 30, 0, 0, time.UTC))
		})
	})

	t.Run("Advance", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNilManual, func() {
			_ = manual.Advance(time.Second)
		})
	})

	t.Run("Add", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNilManual, func() {
			_ = manual.Add(time.Second)
		})
	})
}

// TestManualConcurrentAccess verifies that Manual is safe for concurrent use.
//
// Manual is primarily a test utility, but tests may exercise code with
// goroutines. The internal mutex should serialize reads and writes so concurrent
// Now and Advance calls remain race-free and deterministic enough for aggregate
// assertions.
func TestManualConcurrentAccess(t *testing.T) {
	t.Parallel()

	const goroutines = 16
	const iterations = 1024

	start := time.Date(2026, time.April, 26, 12, 30, 0, 0, time.UTC)

	manual := NewManual(start)

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				manual.Advance(time.Nanosecond)
				_ = manual.Now()
			}
		}()
	}

	wg.Wait()

	want := start.Add(time.Duration(goroutines*iterations) * time.Nanosecond)

	got := manual.Now()
	if !got.Equal(want) {
		t.Fatalf("Manual.Now() after concurrent advances = %s, want %s", got, want)
	}
}
