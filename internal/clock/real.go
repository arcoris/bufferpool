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

// Real is the production Clock implementation backed by time.Now.
//
// Real is intentionally stateless. It exists so runtime code can depend on the
// Clock interface and remain testable with deterministic clock implementations,
// while production paths still use Go's standard wall-clock and monotonic-clock
// source.
//
// The zero value is ready to use:
//
//	var c clock.Real
//	now := c.Now()
//
// Real does not expose Sleep, After, Timer, or Ticker methods. Scheduling and
// event-loop ownership belong to higher-level runtime components such as
// partition controllers. This type only answers one question: what time is it
// now?
//
// The time.Time values returned by time.Now include a monotonic clock reading in
// addition to wall-clock time. When durations are computed between two time.Time
// values produced by time.Now in the same process, Go uses the monotonic reading
// for Sub comparisons. This is important for bufferpool internals because
// elapsed-time calculations should not be corrupted by wall-clock adjustments.
//
// Typical bufferpool use cases include:
//
//   - controller tick timing;
//   - workload-window elapsed duration;
//   - EWMA decay duration;
//   - checked-out buffer hold duration;
//   - idle expiry;
//   - trim cadence;
//   - snapshot age;
//   - controller lag measurement.
//
// Real is safe to share between goroutines because it has no mutable state.
type Real struct{}

// Now returns the current production time.
//
// The implementation deliberately delegates directly to time.Now. The returned
// timestamp carries Go's monotonic clock reading when available, which allows
// duration calculations such as end.Sub(start) to be resilient to wall-clock
// changes when both timestamps come from time.Now.
func (Real) Now() time.Time {
	return time.Now()
}

// Default returns the default production Clock.
//
// Default is a small convenience for constructors that need an explicit
// production clock when no clock is supplied by the caller. It returns Real
// behind the Clock interface so production code and tests can use the same
// dependency shape.
//
// The function does not allocate in practice: Real is stateless, and the
// returned interface carries only the concrete zero-size value.
func Default() Clock {
	return Real{}
}
