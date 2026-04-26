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
	"time"
)

const (
	// errNilManual is used when a Manual method is called on a nil receiver.
	//
	// A nil manual clock is always a test or internal wiring error. The panic is
	// explicit so failures point at the clock dependency rather than producing a
	// generic nil-pointer panic.
	errNilManual = "clock.Manual: receiver must not be nil"

	// errManualZeroTime is used when the caller explicitly sets Manual to the
	// zero time.
	//
	// Helpers in clock.go treat zero timestamps as missing initialization. Manual
	// therefore rejects explicit zero values so tests cannot accidentally create
	// timestamps that later fail elapsed-time validation for the wrong reason.
	errManualZeroTime = "clock.Manual: time must not be zero"

	// errManualNegativeAdvance is used when the caller attempts to advance Manual
	// by a negative duration.
	//
	// Moving time backwards would corrupt controller cadence, EWMA decay,
	// workload windows, lease hold durations, snapshot age, idle expiry, and trim
	// scheduling. Tests that need earlier time should create a new Manual clock
	// with the desired initial timestamp instead.
	errManualNegativeAdvance = "clock.Manual: advance duration must not be negative"

	// errManualBackwardTime is used when the caller attempts to set Manual to a
	// timestamp earlier than its current timestamp.
	//
	// Manual is monotonic by contract. It may stay at the same time or move
	// forward, but it must not move backward.
	errManualBackwardTime = "clock.Manual: time must not move backwards"
)

// manualDefaultTime is the deterministic timestamp used by a zero-value Manual.
//
// The value is intentionally fixed and non-zero. It gives zero-value Manual a
// stable starting point without depending on time.Now, process start time, or
// wall-clock state.
//
// This value is a package variable instead of a constant because time.Time
// values cannot be declared as constants.
var manualDefaultTime = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

// Compile-time interface check.
//
// Manual has pointer receiver methods because it is mutable and contains a
// mutex. Therefore *Manual, not Manual, implements Clock.
var _ Clock = (*Manual)(nil)

// Manual is a deterministic mutable Clock implementation for tests.
//
// Manual exists so tests can control time without time.Sleep, real timers,
// scheduler jitter, or wall-clock dependency. It is intended for deterministic
// testing of runtime logic that depends on elapsed time.
//
// Typical bufferpool use cases include:
//
//   - partition controller cadence;
//   - workload-window elapsed duration;
//   - EWMA decay duration;
//   - checked-out buffer hold duration;
//   - idle expiry;
//   - trim cadence;
//   - snapshot age;
//   - controller lag measurement.
//
// Manual is safe for concurrent use. Its methods serialize access through an
// internal mutex. This makes it suitable for tests that exercise code with
// goroutines, while still keeping the production Real clock stateless and cheap.
//
// Manual is not a scheduler. It does not expose Sleep, After, Timer, or Ticker.
// Higher-level runtime code owns event loops and tick decisions. Manual only
// answers and mutates deterministic current time.
//
// Zero-value behavior:
//
//	var c clock.Manual
//	now := c.Now()
//
// The zero value is valid. On first use, Manual initializes itself to a fixed
// non-zero default timestamp. Explicit construction through NewManual requires a
// non-zero timestamp so accidentally passing time.Time{} is still caught.
type Manual struct {
	mu sync.Mutex

	// now stores the current deterministic timestamp.
	now time.Time

	// initialized reports whether now was explicitly initialized or lazily
	// initialized from manualDefaultTime.
	initialized bool
}

// NewManual returns a Manual clock initialized to startTime.
//
// startTime MUST be non-zero. A zero start time is rejected because zero
// timestamps are used by clock helpers as missing-initialization sentinels.
//
// The returned clock starts exactly at startTime. Future calls to Advance or Set
// may keep time equal or move it forward, but must not move it backward.
func NewManual(startTime time.Time) *Manual {
	if startTime.IsZero() {
		panic(errManualZeroTime)
	}

	return &Manual{
		now:         startTime,
		initialized: true,
	}
}

// Now returns the current deterministic time.
//
// If Manual is used as a zero value, Now lazily initializes it to
// manualDefaultTime before returning. This keeps Manual embeddable in test
// structs without requiring explicit setup for every case.
//
// Now does not advance time. Tests must call Advance or Set explicitly.
func (m *Manual) Now() time.Time {
	m.mustNotBeNil()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.ensureInitializedLocked()

	return m.now
}

// Set moves the manual clock to nextTime.
//
// nextTime MUST be non-zero. If the clock is already initialized, nextTime MUST
// be greater than or equal to the current time. Equal timestamps are allowed and
// are useful when a test wants to publish an exact timestamp without advancing
// time.
//
// Set is intentionally monotonic. Allowing backward movement would make elapsed
// time negative and would hide bugs in controller cadence, workload windows,
// EWMA decay, idle expiry, and trim scheduling.
//
// If Set is called before Now or Advance on a zero-value Manual, it initializes
// the clock directly to nextTime.
func (m *Manual) Set(nextTime time.Time) {
	m.mustNotBeNil()

	if nextTime.IsZero() {
		panic(errManualZeroTime)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.initialized && nextTime.Before(m.now) {
		panic(errManualBackwardTime)
	}

	m.now = nextTime
	m.initialized = true
}

// Advance moves the manual clock forward by duration and returns the new time.
//
// duration MUST be greater than or equal to zero. A zero duration is a valid
// no-op and returns the current time.
//
// If Advance is called on a zero-value Manual before any explicit Set or Now,
// the clock is first initialized to manualDefaultTime and then advanced.
func (m *Manual) Advance(duration time.Duration) time.Time {
	m.mustNotBeNil()

	if duration < 0 {
		panic(errManualNegativeAdvance)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.ensureInitializedLocked()

	m.now = m.now.Add(duration)

	return m.now
}

// Add is an alias for Advance.
//
// The method exists for tests that read more naturally as direct time arithmetic:
//
//	now := manual.Add(100 * time.Millisecond)
//
// It follows the same invariants as Advance.
func (m *Manual) Add(duration time.Duration) time.Time {
	return m.Advance(duration)
}

// mustNotBeNil validates the receiver before accessing fields.
//
// Methods on nil pointer receivers are legal to call in Go. Providing an
// explicit panic message makes wiring failures easier to diagnose than a generic
// nil-pointer dereference.
func (m *Manual) mustNotBeNil() {
	if m == nil {
		panic(errNilManual)
	}
}

// ensureInitializedLocked initializes a zero-value Manual.
//
// The caller MUST hold m.mu. The helper is intentionally private so all mutation
// remains serialized through the Manual methods.
func (m *Manual) ensureInitializedLocked() {
	if m.initialized {
		return
	}

	m.now = manualDefaultTime
	m.initialized = true
}
