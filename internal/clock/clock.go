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

import "time"

const (
	// errNilClock is used when a helper receives a nil Clock.
	//
	// A nil clock is always an internal wiring error. Runtime code should choose
	// an explicit real clock for production paths or a manual clock for tests.
	errNilClock = "clock: Clock must not be nil"

	// errZeroStartTime is used when elapsed time is requested from a zero start
	// timestamp.
	//
	// A zero timestamp usually means the caller forgot to initialize sampling,
	// lease timing, controller timing, or snapshot timing state before computing
	// an elapsed duration.
	errZeroStartTime = "clock: start time must not be zero"

	// errZeroEndTime is used when elapsed time is requested with a zero end
	// timestamp.
	//
	// This normally indicates broken caller-side sampling or a manually
	// constructed timestamp that bypassed the Clock abstraction.
	errZeroEndTime = "clock: end time must not be zero"

	// errNegativeElapsedTime is used when end occurs before start.
	//
	// Negative elapsed time would corrupt EWMA decay, controller cadence,
	// workload windows, lease hold durations, snapshot age, and trim scheduling.
	// Production real-clock values from time.Now carry monotonic information when
	// compared to each other, so negative elapsed time is usually a caller bug or
	// invalid manual-clock movement.
	errNegativeElapsedTime = "clock: elapsed time must not be negative"
)

// Clock is the minimal time source used by bufferpool internals.
//
// The interface intentionally exposes only Now. It does not expose sleeping,
// timers, tickers, or scheduling primitives. This keeps the package focused on
// deterministic time measurement instead of event-loop orchestration.
//
// Typical bufferpool use cases include:
//
//   - computing workload-window elapsed time;
//   - computing EWMA decay from real elapsed duration;
//   - measuring checked-out buffer hold duration;
//   - computing controller lag;
//   - computing snapshot age;
//   - evaluating idle expiry;
//   - scheduling bounded trim cadence at higher layers.
//
// Production code should use a real clock implementation backed by time.Now.
// Tests should use a manual clock implementation so controller cadence, EWMA
// decay, idle expiry, and hold-duration behavior remain deterministic.
//
// Implementations SHOULD return non-zero timestamps. Helpers in this package
// treat zero timestamps as internal invariant violations where appropriate.
type Clock interface {
	// Now returns the current time according to this clock.
	//
	// Real-clock implementations should return time.Now so Go's monotonic clock
	// reading is preserved when durations are computed between two values
	// produced by the same process.
	//
	// Manual-clock implementations should return their current deterministic
	// time and should not move backwards.
	Now() time.Time
}

// Now returns the current time from clock.
//
// The helper exists to centralize nil-clock validation. Lower runtime layers
// should fail fast when time dependencies are not wired correctly instead of
// silently falling back to time.Now or returning a zero timestamp.
func Now(clock Clock) time.Time {
	if clock == nil {
		panic(errNilClock)
	}

	return clock.Now()
}

// Since returns the non-negative duration from startTime to clock.Now().
//
// Since is useful when the caller owns a start timestamp and wants elapsed time
// according to the same clock abstraction used by the rest of the runtime.
//
// Typical bufferpool use cases include:
//
//   - lease hold duration;
//   - controller cycle duration;
//   - controller lag;
//   - snapshot age;
//   - idle duration;
//   - time since last trim.
//
// startTime MUST be non-zero. A zero start time indicates missing initialization.
func Since(clock Clock, startTime time.Time) time.Duration {
	return Elapsed(startTime, Now(clock))
}

// Elapsed returns the non-negative duration from startTime to endTime.
//
// Elapsed centralizes duration validation for runtime code that depends on
// monotonic time progression. Negative elapsed time is rejected because it would
// corrupt adaptive control calculations.
//
// For real time values produced by time.Now, Go carries a monotonic clock
// reading inside time.Time and Sub uses that monotonic reading when both values
// have it. For manual clocks, callers must preserve monotonic behavior by never
// moving time backwards.
//
// Both startTime and endTime MUST be non-zero.
func Elapsed(startTime, endTime time.Time) time.Duration {
	if startTime.IsZero() {
		panic(errZeroStartTime)
	}

	if endTime.IsZero() {
		panic(errZeroEndTime)
	}

	elapsed := endTime.Sub(startTime)
	if elapsed < 0 {
		panic(errNegativeElapsedTime)
	}

	return elapsed
}

// HasElapsed reports whether at least duration has elapsed between startTime and
// endTime.
//
// The helper is intended for cadence and expiry checks where callers need a
// clear, reusable predicate:
//
//   - has the controller tick interval elapsed;
//   - has the idle timeout elapsed;
//   - has the trim interval elapsed;
//   - is a snapshot older than the allowed age.
//
// The duration argument is treated literally. A duration less than or equal to
// zero is considered already elapsed after timestamp validation. This matches
// common scheduling semantics: a non-positive interval should not block
// progress, but it also should not hide broken timestamps.
//
// startTime and endTime are validated by Elapsed.
func HasElapsed(startTime, endTime time.Time, duration time.Duration) bool {
	elapsed := Elapsed(startTime, endTime)
	if duration <= 0 {
		return true
	}

	return elapsed >= duration
}

// MaxTime returns the later of a and b.
//
// This helper is useful when merging sampled timestamps, snapshot timestamps, or
// controller progress markers without repeating time comparison logic at higher
// layers.
//
// Zero values are not rejected here because callers may intentionally use
// MaxTime to fold optional timestamps. If both values are zero, the result is
// zero.
func MaxTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return b
	}

	return a
}

// MinTime returns the earlier of a and b.
//
// This helper is useful when merging timestamp ranges or finding the oldest
// sampled timestamp.
//
// Zero values are not rejected here because callers may intentionally use
// MinTime with optional timestamps. However, note that time.Time{} is earlier
// than ordinary runtime timestamps.
func MinTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}

	return b
}
