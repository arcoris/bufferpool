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
)

// Compile-time interface checks.
//
// These assertions ensure the production clock can be passed anywhere the
// runtime expects the minimal Clock contract.
var (
	_ Clock = Real{}
)

// TestRealZeroValueIsUsable verifies that Real requires no initialization.
//
// Real is intentionally stateless. Runtime constructors should be able to use
// clock.Real{} directly when no test clock is supplied.
func TestRealZeroValueIsUsable(t *testing.T) {
	t.Parallel()

	var c Real

	got := c.Now()
	if got.IsZero() {
		t.Fatal("Real zero-value Now() returned zero time")
	}
}

// TestRealNowReturnsCurrentTime verifies that Real.Now returns a timestamp
// within the call window.
//
// The test does not assert an exact timestamp. It only verifies that Real.Now is
// connected to the production time source and returns a value between the local
// before/after observations.
func TestRealNowReturnsCurrentTime(t *testing.T) {
	t.Parallel()

	c := Real{}

	before := time.Now()
	got := c.Now()
	after := time.Now()

	if got.Before(before) {
		t.Fatalf("Real.Now() = %s, want not before call window start %s", got, before)
	}

	if got.After(after) {
		t.Fatalf("Real.Now() = %s, want not after call window end %s", got, after)
	}
}

// TestRealNowMovesForward verifies basic production-clock progression.
//
// This is intentionally weak: real clocks and scheduling are not deterministic.
// The only required property here is that two consecutive calls should not
// produce a negative elapsed duration when compared through the package helper.
func TestRealNowMovesForward(t *testing.T) {
	t.Parallel()

	c := Real{}

	first := c.Now()
	second := c.Now()

	elapsed := Elapsed(first, second)
	if elapsed < 0 {
		t.Fatalf("Elapsed(first, second) = %s, want non-negative", elapsed)
	}
}

// TestRealWorksWithSince verifies that Real can be used through helper paths
// expecting the Clock interface.
//
// Since should accept Real as a Clock and compute a non-negative duration from a
// timestamp produced by the same production time source.
func TestRealWorksWithSince(t *testing.T) {
	t.Parallel()

	c := Real{}

	start := c.Now()
	elapsed := Since(c, start)

	if elapsed < 0 {
		t.Fatalf("Since(Real{}, start) = %s, want non-negative", elapsed)
	}
}

// TestDefaultReturnsClock verifies that Default exposes the production clock
// behind the Clock interface.
//
// This protects constructor code that wants to normalize nil clock dependencies
// to a single production default.
func TestDefaultReturnsClock(t *testing.T) {
	t.Parallel()

	c := Default()
	if c == nil {
		t.Fatal("Default() returned nil Clock")
	}

	if _, ok := c.(Real); !ok {
		t.Fatalf("Default() returned %T, want clock.Real", c)
	}
}

// TestDefaultClockIsUsable verifies that the default production Clock can return
// current time immediately.
//
// This test covers the full default construction path:
//
//	Default() -> Clock -> Now()
func TestDefaultClockIsUsable(t *testing.T) {
	t.Parallel()

	c := Default()

	before := time.Now()
	got := c.Now()
	after := time.Now()

	if got.IsZero() {
		t.Fatal("Default().Now() returned zero time")
	}

	if got.Before(before) {
		t.Fatalf("Default().Now() = %s, want not before call window start %s", got, before)
	}

	if got.After(after) {
		t.Fatalf("Default().Now() = %s, want not after call window end %s", got, after)
	}
}
