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

package testutil

import (
	"testing"
	"time"
)

// TestFakeClockReturnsConfiguredTime verifies deterministic Now behavior.
func TestFakeClockReturnsConfiguredTime(t *testing.T) {
	t.Parallel()

	want := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	clock := NewFakeClock(want)

	if got := clock.Now(); !got.Equal(want) {
		t.Fatalf("FakeClock.Now() = %s, want %s", got, want)
	}
}

// TestFakeClockZeroValueReturnsZeroTime documents zero-value behavior.
//
// Unlike clock.Manual, FakeClock does not repair zero time. It is a permissive
// test double, so callers can deliberately pass zero through code under test.
func TestFakeClockZeroValueReturnsZeroTime(t *testing.T) {
	t.Parallel()

	var clock FakeClock

	if got := clock.Now(); !got.IsZero() {
		t.Fatalf("zero-value FakeClock.Now() = %s, want zero", got)
	}
}

// TestFakeClockSetReplacesCurrentTime verifies absolute time replacement.
func TestFakeClockSetReplacesCurrentTime(t *testing.T) {
	t.Parallel()

	initial := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	want := initial.Add(time.Second)

	clock := NewFakeClock(initial)
	clock.Set(want)

	if got := clock.Now(); !got.Equal(want) {
		t.Fatalf("FakeClock.Now() after Set = %s, want %s", got, want)
	}
}

// TestFakeClockAdvanceMovesRelativeToCurrentTime verifies relative movement.
func TestFakeClockAdvanceMovesRelativeToCurrentTime(t *testing.T) {
	t.Parallel()

	initial := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	clock := NewFakeClock(initial)

	got := clock.Advance(250 * time.Millisecond)
	want := initial.Add(250 * time.Millisecond)

	if !got.Equal(want) {
		t.Fatalf("FakeClock.Advance returned %s, want %s", got, want)
	}

	if now := clock.Now(); !now.Equal(want) {
		t.Fatalf("FakeClock.Now() after Advance = %s, want %s", now, want)
	}
}

// TestFakeClockAdvanceAllowsNegativeDuration keeps invalid-time tests possible.
func TestFakeClockAdvanceAllowsNegativeDuration(t *testing.T) {
	t.Parallel()

	initial := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	clock := NewFakeClock(initial)

	got := clock.Advance(-time.Nanosecond)
	want := initial.Add(-time.Nanosecond)

	if !got.Equal(want) {
		t.Fatalf("FakeClock.Advance returned %s, want %s", got, want)
	}
}
