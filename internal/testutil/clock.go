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

import "time"

// FakeClock is a deterministic test clock.
//
// The type intentionally depends only on time.Time, not on internal/clock. That
// keeps testutil independent while still allowing FakeClock to satisfy any
// interface with a Now() time.Time method.
//
// FakeClock is deliberately permissive: Set can jump to any timestamp and
// Advance accepts negative durations. That makes it useful for testing
// caller-side validation of invalid time movement. Tests that need a monotonic
// clock should use the clock package's manual clock instead.
//
// FakeClock is not synchronized. Tests that share one instance across
// goroutines must coordinate access themselves or give each goroutine its own
// clock.
type FakeClock struct {
	now time.Time
}

// NewFakeClock returns a deterministic clock initialized to now.
func NewFakeClock(now time.Time) FakeClock {
	return FakeClock{now: now}
}

// Now returns the clock's current deterministic timestamp.
func (c FakeClock) Now() time.Time {
	return c.now
}

// Set replaces the clock's current timestamp exactly.
//
// Set does not validate zero or backwards movement. The helper is a generic
// test double; validation belongs to the code under test.
func (c *FakeClock) Set(now time.Time) {
	c.now = now
}

// Advance moves the clock forward by duration and returns the updated time.
//
// Advance does not reject negative durations. Some tests intentionally need to
// construct invalid time movement in order to verify caller-side validation.
func (c *FakeClock) Advance(duration time.Duration) time.Time {
	c.now = c.now.Add(duration)
	return c.now
}
