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
	"testing"
	"time"

	"arcoris.dev/bufferpool/internal/testutil"
)

// Compile-time interface checks for shared test clocks.
//
// testutil.FakeClock intentionally lives outside this package so higher-level
// tests can reuse a lightweight Clock-compatible double without importing
// clock internals.
var (
	_ Clock = testutil.FakeClock{}
	_ Clock = (*testutil.FakeClock)(nil)
)

// TestNowReturnsClockTime verifies that Now delegates to the provided Clock.
//
// The helper is intentionally thin, but it centralizes nil-clock validation.
// This test protects the delegation behavior so higher-level runtime code can
// use clock.Now(c) without changing the timestamp produced by c.
func TestNowReturnsClockTime(t *testing.T) {
	t.Parallel()

	want := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	got := Now(testutil.NewFakeClock(want))

	if !got.Equal(want) {
		t.Fatalf("Now(fake clock) = %s, want %s", got, want)
	}
}

// TestNowPanicsForNilClock verifies fail-fast behavior for missing time wiring.
//
// A nil Clock means an internal constructor or runtime component failed to
// choose an explicit time source. Silently falling back to time.Now would make
// deterministic tests less reliable and hide dependency wiring bugs.
func TestNowPanicsForNilClock(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errNilClock, func() {
		_ = Now(nil)
	})
}

// TestSinceReturnsElapsedDuration verifies elapsed-time calculation through the
// Clock abstraction.
//
// Since should use the supplied Clock's current time, not time.Now directly.
// This is the core reason the Clock interface exists: controller, workload,
// lease, idle-expiry, and trim-cadence tests must be able to control time.
func TestSinceReturnsElapsedDuration(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	start := now.Add(-250 * time.Millisecond)

	got := Since(testutil.NewFakeClock(now), start)
	want := 250 * time.Millisecond

	if got != want {
		t.Fatalf("Since(fake clock, start) = %s, want %s", got, want)
	}
}

// TestSincePanicsForNilClock verifies that Since preserves nil-clock validation.
//
// Since delegates current-time retrieval through Now, so the same wiring
// invariant applies here as well.
func TestSincePanicsForNilClock(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	testutil.MustPanicWithMessage(t, errNilClock, func() {
		_ = Since(nil, start)
	})
}

// TestSincePanicsForZeroStartTime verifies that missing start timestamps are
// rejected.
//
// A zero start timestamp usually means sampling, lease timing, controller
// timing, snapshot timing, or idle-expiry state was not initialized before
// elapsed time was requested.
func TestSincePanicsForZeroStartTime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	testutil.MustPanicWithMessage(t, errZeroStartTime, func() {
		_ = Since(testutil.NewFakeClock(now), time.Time{})
	})
}

// TestSinceUsesCurrentFakeClockValue verifies that Since observes the clock at
// call time.
//
// This protects the dependency boundary that higher-level tests rely on:
// advancing a deterministic clock should affect elapsed-time helpers without
// rewriting the original start timestamp.
func TestSinceUsesCurrentFakeClockValue(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	clock := testutil.NewFakeClock(start)

	clock.Advance(750 * time.Millisecond)

	got := Since(&clock, start)
	want := 750 * time.Millisecond

	if got != want {
		t.Fatalf("Since(advanced fake clock, start) = %s, want %s", got, want)
	}
}

// TestElapsedReturnsDuration verifies ordinary non-negative elapsed-time
// calculation.
//
// Elapsed is the central helper for duration math used by controller, workload,
// lease, snapshot, idle-expiry, and trim-cadence code.
func TestElapsedReturnsDuration(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	end := start.Add(2*time.Second + 500*time.Millisecond)

	got := Elapsed(start, end)
	want := 2*time.Second + 500*time.Millisecond

	if got != want {
		t.Fatalf("Elapsed(start, end) = %s, want %s", got, want)
	}
}

// TestElapsedAllowsEqualTimestamps verifies that zero elapsed duration is valid.
//
// Equal non-zero timestamps can happen in deterministic tests or very fast
// sampling paths. This is not an error: it represents a valid zero-duration
// observation window.
func TestElapsedAllowsEqualTimestamps(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	got := Elapsed(start, start)
	if got != 0 {
		t.Fatalf("Elapsed(start, start) = %s, want 0", got)
	}
}

// TestElapsedPanicsForZeroStartTime verifies that the start timestamp is
// required.
//
// This protects elapsed-time users from silently treating missing initialization
// as a valid time origin.
func TestElapsedPanicsForZeroStartTime(t *testing.T) {
	t.Parallel()

	end := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	testutil.MustPanicWithMessage(t, errZeroStartTime, func() {
		_ = Elapsed(time.Time{}, end)
	})
}

// TestElapsedPanicsForZeroEndTime verifies that the end timestamp is required.
//
// A zero end timestamp usually means a caller bypassed the Clock abstraction or
// constructed an invalid manual timestamp.
func TestElapsedPanicsForZeroEndTime(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	testutil.MustPanicWithMessage(t, errZeroEndTime, func() {
		_ = Elapsed(start, time.Time{})
	})
}

// TestElapsedPanicsForNegativeDuration verifies monotonic-time invariants.
//
// Negative elapsed time would corrupt EWMA decay, controller cadence, workload
// windows, lease hold duration, snapshot age, idle expiry, and trim scheduling.
func TestElapsedPanicsForNegativeDuration(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	end := start.Add(-time.Nanosecond)

	testutil.MustPanicWithMessage(t, errNegativeElapsedTime, func() {
		_ = Elapsed(start, end)
	})
}

// TestHasElapsed verifies cadence and expiry checks for positive durations.
//
// HasElapsed is the predicate form used by higher-level runtime logic such as
// controller tick intervals, idle expiry, trim cadence, and snapshot age checks.
func TestHasElapsed(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name string

		// end is the timestamp being compared against start.
		end time.Time

		// duration is the required elapsed duration.
		duration time.Duration

		// want is true when end-start is at least duration.
		want bool
	}{
		{
			name:     "less than duration",
			end:      start.Add(99 * time.Millisecond),
			duration: 100 * time.Millisecond,
			want:     false,
		},
		{
			name:     "exact duration",
			end:      start.Add(100 * time.Millisecond),
			duration: 100 * time.Millisecond,
			want:     true,
		},
		{
			name:     "greater than duration",
			end:      start.Add(101 * time.Millisecond),
			duration: 100 * time.Millisecond,
			want:     true,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := HasElapsed(start, tt.end, tt.duration)
			if got != tt.want {
				t.Fatalf("HasElapsed(start, %s, %s) = %t, want %t",
					tt.end,
					tt.duration,
					got,
					tt.want,
				)
			}
		})
	}
}

// TestHasElapsedReturnsTrueForNonPositiveDurations verifies immediate cadence
// behavior for zero and negative intervals after timestamp validation.
//
// The helper treats non-positive intervals as already elapsed. This is useful
// when higher-level configuration intentionally disables waiting or normalizes a
// cadence to immediate progress. The timestamps are still valid here because
// invalid timestamps should never be hidden by duration configuration.
func TestHasElapsedReturnsTrueForNonPositiveDurations(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	end := start

	tests := []struct {
		name string

		// duration is the configured cadence or expiry interval.
		duration time.Duration
	}{
		{
			name:     "zero duration",
			duration: 0,
		},
		{
			name:     "negative duration",
			duration: -time.Nanosecond,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if !HasElapsed(start, end, tt.duration) {
				t.Fatalf("HasElapsed(start, end, %s) = false, want true", tt.duration)
			}
		})
	}
}

// TestHasElapsedPanicsForInvalidTimes verifies that HasElapsed always preserves
// Elapsed validation.
//
// Broken timestamps must fail fast even when duration <= 0 would otherwise mean
// immediate progress. Configuration should not hide missing or backwards time.
func TestHasElapsedPanicsForInvalidTimes(t *testing.T) {
	t.Parallel()

	valid := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("zero start", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errZeroStartTime, func() {
			_ = HasElapsed(time.Time{}, valid, time.Nanosecond)
		})
	})

	t.Run("zero end", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errZeroEndTime, func() {
			_ = HasElapsed(valid, time.Time{}, time.Nanosecond)
		})
	})

	t.Run("negative elapsed", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNegativeElapsedTime, func() {
			_ = HasElapsed(valid, valid.Add(-time.Nanosecond), time.Nanosecond)
		})
	})

	t.Run("zero start with non-positive duration", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errZeroStartTime, func() {
			_ = HasElapsed(time.Time{}, valid, 0)
		})
	})

	t.Run("negative elapsed with non-positive duration", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errNegativeElapsedTime, func() {
			_ = HasElapsed(valid, valid.Add(-time.Nanosecond), -time.Nanosecond)
		})
	})
}

// TestMaxTime verifies that MaxTime returns the later timestamp.
//
// MaxTime is useful when merging sampled timestamps, snapshot timestamps, or
// controller progress markers.
func TestMaxTime(t *testing.T) {
	t.Parallel()

	earlier := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	later := earlier.Add(time.Second)

	if got := MaxTime(earlier, later); !got.Equal(later) {
		t.Fatalf("MaxTime(earlier, later) = %s, want %s", got, later)
	}

	if got := MaxTime(later, earlier); !got.Equal(later) {
		t.Fatalf("MaxTime(later, earlier) = %s, want %s", got, later)
	}
}

// TestMaxTimeAllowsZeroValues verifies optional timestamp folding behavior.
//
// MaxTime intentionally does not reject zero timestamps because callers may use
// it when folding optional timestamp values.
func TestMaxTimeAllowsZeroValues(t *testing.T) {
	t.Parallel()

	zero := time.Time{}
	value := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	if got := MaxTime(zero, value); !got.Equal(value) {
		t.Fatalf("MaxTime(zero, value) = %s, want %s", got, value)
	}

	if got := MaxTime(value, zero); !got.Equal(value) {
		t.Fatalf("MaxTime(value, zero) = %s, want %s", got, value)
	}

	if got := MaxTime(zero, zero); !got.IsZero() {
		t.Fatalf("MaxTime(zero, zero) = %s, want zero", got)
	}
}

// TestMinTime verifies that MinTime returns the earlier timestamp.
//
// MinTime is useful when merging timestamp ranges or finding the oldest sampled
// timestamp.
func TestMinTime(t *testing.T) {
	t.Parallel()

	earlier := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	later := earlier.Add(time.Second)

	if got := MinTime(earlier, later); !got.Equal(earlier) {
		t.Fatalf("MinTime(earlier, later) = %s, want %s", got, earlier)
	}

	if got := MinTime(later, earlier); !got.Equal(earlier) {
		t.Fatalf("MinTime(later, earlier) = %s, want %s", got, earlier)
	}
}

// TestMinTimeAllowsZeroValues verifies optional timestamp folding behavior.
//
// A zero time is earlier than ordinary runtime timestamps. This behavior is
// intentional and should remain visible to callers using optional timestamp
// aggregation.
func TestMinTimeAllowsZeroValues(t *testing.T) {
	t.Parallel()

	zero := time.Time{}
	value := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	if got := MinTime(zero, value); !got.IsZero() {
		t.Fatalf("MinTime(zero, value) = %s, want zero", got)
	}

	if got := MinTime(value, zero); !got.IsZero() {
		t.Fatalf("MinTime(value, zero) = %s, want zero", got)
	}

	if got := MinTime(zero, zero); !got.IsZero() {
		t.Fatalf("MinTime(zero, zero) = %s, want zero", got)
	}
}
