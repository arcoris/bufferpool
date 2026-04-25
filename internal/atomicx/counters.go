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

const (
	// errUint64GaugeOverflow is used when a non-negative gauge addition would
	// wrap past math.MaxUint64.
	//
	// A wrapped memory or count gauge would make current runtime state appear
	// smaller than it is, so overflow is treated as an accounting invariant
	// violation.
	errUint64GaugeOverflow = "atomicx.Uint64Gauge: addition overflows uint64"

	// errUint64GaugeUnderflow is used when a non-negative gauge subtraction
	// would move below zero.
	//
	// Underflow means the caller attempted to remove more bytes, buffers, or
	// work items than were previously accounted.
	errUint64GaugeUnderflow = "atomicx.Uint64Gauge: subtraction underflows uint64"

	// errInt64GaugeOverflow is used when a signed gauge addition would exceed
	// the largest representable int64 value.
	errInt64GaugeOverflow = "atomicx.Int64Gauge: addition overflows int64"

	// errInt64GaugeUnderflow is used when a signed gauge addition would move
	// below the smallest representable int64 value.
	errInt64GaugeUnderflow = "atomicx.Int64Gauge: addition underflows int64"
)

const (
	// maxUint64 is kept local so gauge overflow checks and tests can name the
	// unsigned boundary without repeating bit tricks at each call site.
	maxUint64 = ^uint64(0)

	// maxInt64 is kept local so overflow checks do not depend on float-based
	// constants or conversions.
	maxInt64 = int64(1<<63 - 1)

	// minInt64 is the smallest signed 64-bit value. It cannot be written as
	// -1 << 63 without relying on untyped constant edge cases in every check.
	minInt64 = -maxInt64 - 1
)

// Uint64Counter is a padded monotonic uint64 counter.
//
// Uint64Counter is intended for lifetime event counters that only move forward,
// for example:
//
//   - shard gets;
//   - shard puts;
//   - hits;
//   - misses;
//   - allocations;
//   - drops;
//   - trim operations;
//   - controller ticks.
//
// The counter is backed by PaddedUint64 so it can sit in hot shared runtime
// structs with reduced risk of false sharing.
//
// Uint64Counter intentionally does not expose Store, Swap, Dec, or Sub methods.
// A lifetime counter should not be reset or decremented by ordinary runtime
// code. Workload windows should compute deltas between snapshots instead of
// resetting the source counter.
//
// Uint64Counter uses ordinary uint64 atomic arithmetic. If the value eventually
// wraps after reaching maxUint64, delta calculation remains correct for a
// single wrap through Uint64CounterDelta. Multiple wraps between two samples are
// not distinguishable and must be prevented by reasonable sampling cadence.
//
// Uint64Counter must not be copied after first use. Copying it would copy the
// underlying atomic value and split one logical counter into independent state.
type Uint64Counter struct {
	value PaddedUint64
}

// Load atomically returns the current lifetime counter value.
func (c *Uint64Counter) Load() uint64 {
	return c.value.Load()
}

// Add atomically adds delta to the counter and returns the new value.
//
// Add is intended for batched event increments. For the common single-event
// case, use Inc.
func (c *Uint64Counter) Add(delta uint64) uint64 {
	return c.value.Add(delta)
}

// Inc atomically adds one event and returns the new value.
func (c *Uint64Counter) Inc() uint64 {
	return c.value.Inc()
}

// Snapshot returns an immutable point-in-time counter sample.
//
// A snapshot is intentionally just a value object. It does not retain a pointer
// to the counter, so later counter updates cannot change the sample.
func (c *Uint64Counter) Snapshot() Uint64CounterSnapshot {
	return Uint64CounterSnapshot{
		Value: c.value.Load(),
	}
}

// Uint64CounterSnapshot is an immutable point-in-time sample of a monotonic
// uint64 counter.
//
// Snapshots are used by workload-window code to compute recent activity without
// resetting lifetime counters.
type Uint64CounterSnapshot struct {
	// Value is the observed lifetime counter value at sampling time.
	Value uint64
}

// DeltaSince returns the monotonic delta from previous to s.
//
// The calculation is wrap-aware for a single uint64 wrap. This is useful for
// long-running processes where lifetime counters should not need to reset.
// The receiver is the newer sample; previous must be the older sample.
func (s Uint64CounterSnapshot) DeltaSince(previous Uint64CounterSnapshot) Uint64CounterDelta {
	return NewUint64CounterDelta(previous.Value, s.Value)
}

// Uint64CounterDelta describes the difference between two monotonic uint64
// counter snapshots.
//
// If Current is greater than or equal to Previous, Value is Current - Previous.
// If Current is less than Previous, the counter is assumed to have wrapped once
// and Value is computed using uint64 modulo arithmetic.
//
// Multiple wraps between two snapshots cannot be detected from two uint64
// samples alone. Sampling cadence must be frequent enough to make that
// impossible in practice.
type Uint64CounterDelta struct {
	// Previous is the older lifetime counter value.
	Previous uint64

	// Current is the newer lifetime counter value.
	Current uint64

	// Value is the computed delta between Previous and Current.
	Value uint64

	// Wrapped reports whether Current was lower than Previous and the delta was
	// interpreted as a single uint64 wrap.
	Wrapped bool
}

// NewUint64CounterDelta computes a wrap-aware delta between two lifetime counter
// values.
//
// uint64 subtraction already has the desired modulo behavior for a single wrap,
// so Current - Previous works for both ordinary and wrapped cases. Wrapped is
// still recorded because observability and tests may need to know that a wrap
// occurred.
func NewUint64CounterDelta(previous, current uint64) Uint64CounterDelta {
	return Uint64CounterDelta{
		Previous: previous,
		Current:  current,
		Value:    current - previous,
		Wrapped:  current < previous,
	}
}

// IsZero reports whether no progress was observed between the two samples.
func (d Uint64CounterDelta) IsZero() bool {
	return d.Value == 0
}

// Uint64Gauge is a padded non-negative uint64 gauge.
//
// Uint64Gauge is intended for current runtime quantities that can move both up
// and down but must never become negative, for example:
//
//   - retained bytes;
//   - retained buffers;
//   - active entry counts;
//   - trim backlog;
//   - pending work counts.
//
// Unlike Uint64Counter, Uint64Gauge exposes both Add and Sub. Both operations
// guard their invariants:
//
//   - Add panics on uint64 overflow;
//   - Sub panics on uint64 underflow.
//
// These panics indicate internal accounting bugs. A non-negative runtime gauge
// must not silently wrap.
//
// Uint64Gauge must not be copied after first use. Copying it would copy the
// underlying atomic value and split one logical gauge into independent state.
type Uint64Gauge struct {
	value PaddedUint64
}

// Load atomically returns the current gauge value.
func (g *Uint64Gauge) Load() uint64 {
	return g.value.Load()
}

// Store atomically replaces the current gauge value.
//
// Store should be used for initialization, tests, or controlled state
// publication. Ordinary runtime accounting should prefer Add and Sub so
// invariant violations are detected at the update point.
func (g *Uint64Gauge) Store(value uint64) {
	g.value.Store(value)
}

// Add atomically adds delta to the gauge and returns the new value.
//
// Add panics if the operation would overflow uint64. Silent wraparound would
// corrupt memory accounting and could make retained memory appear smaller than
// it actually is.
func (g *Uint64Gauge) Add(delta uint64) uint64 {
	if delta == 0 {
		return g.value.Load()
	}

	for {
		current := g.value.Load()
		if current > maxUint64-delta {
			panic(errUint64GaugeOverflow)
		}

		next := current + delta
		if g.value.CompareAndSwap(current, next) {
			return next
		}
	}
}

// Sub atomically subtracts delta from the gauge and returns the new value.
//
// Sub panics if the operation would make the gauge negative. For bufferpool
// accounting, underflow means a caller released, trimmed, or removed more state
// than was previously accounted.
func (g *Uint64Gauge) Sub(delta uint64) uint64 {
	if delta == 0 {
		return g.value.Load()
	}

	for {
		current := g.value.Load()
		if current < delta {
			panic(errUint64GaugeUnderflow)
		}

		next := current - delta
		if g.value.CompareAndSwap(current, next) {
			return next
		}
	}
}

// Inc atomically adds one and returns the new value.
func (g *Uint64Gauge) Inc() uint64 {
	return g.Add(1)
}

// Dec atomically subtracts one and returns the new value.
func (g *Uint64Gauge) Dec() uint64 {
	return g.Sub(1)
}

// Swap atomically stores newValue and returns the previous value.
//
// Swap is useful when a caller owns explicit state handoff semantics. It should
// not be used to hide accounting bugs that should be expressed as Add or Sub.
func (g *Uint64Gauge) Swap(newValue uint64) uint64 {
	return g.value.Swap(newValue)
}

// CompareAndSwap atomically replaces oldValue with newValue when the current
// value still equals oldValue.
//
// This method is exposed for advanced internal state transitions where the
// caller needs to coordinate an explicit expected value.
func (g *Uint64Gauge) CompareAndSwap(oldValue, newValue uint64) bool {
	return g.value.CompareAndSwap(oldValue, newValue)
}

// Int64Gauge is a padded signed int64 gauge.
//
// Int64Gauge is intended for current runtime values where negative values are
// meaningful, for example:
//
//   - signed correction deltas;
//   - signed budget movement;
//   - controller lag corrections;
//   - temporary signed adjustment values.
//
// For monotonically increasing event counters, use Uint64Counter. For
// non-negative memory or count gauges, use Uint64Gauge.
//
// Add guards against int64 overflow and underflow. Silent signed wraparound
// would corrupt control-plane state and make correction logic misleading.
//
// Int64Gauge must not be copied after first use. Copying it would copy the
// underlying atomic value and split one logical gauge into independent state.
type Int64Gauge struct {
	value PaddedInt64
}

// Load atomically returns the current gauge value.
func (g *Int64Gauge) Load() int64 {
	return g.value.Load()
}

// Store atomically replaces the current gauge value.
//
// Store should be used for initialization, tests, or controlled state
// publication. Ordinary runtime changes should prefer Add, Inc, or Dec.
func (g *Int64Gauge) Store(value int64) {
	g.value.Store(value)
}

// Add atomically adds delta to the gauge and returns the new value.
//
// Add panics if the operation would overflow or underflow int64.
func (g *Int64Gauge) Add(delta int64) int64 {
	if delta == 0 {
		return g.value.Load()
	}

	for {
		current := g.value.Load()

		if delta > 0 && current > maxInt64-delta {
			panic(errInt64GaugeOverflow)
		}

		if delta < 0 && current < minInt64-delta {
			panic(errInt64GaugeUnderflow)
		}

		next := current + delta
		if g.value.CompareAndSwap(current, next) {
			return next
		}
	}
}

// Inc atomically adds one and returns the new value.
func (g *Int64Gauge) Inc() int64 {
	return g.Add(1)
}

// Dec atomically subtracts one and returns the new value.
func (g *Int64Gauge) Dec() int64 {
	return g.Add(-1)
}

// Swap atomically stores newValue and returns the previous value.
//
// Swap is useful for explicit handoff or reset-style internal state transitions
// where the caller owns the semantics.
func (g *Int64Gauge) Swap(newValue int64) int64 {
	return g.value.Swap(newValue)
}

// CompareAndSwap atomically replaces oldValue with newValue when the current
// value still equals oldValue.
func (g *Int64Gauge) CompareAndSwap(oldValue, newValue int64) bool {
	return g.value.CompareAndSwap(oldValue, newValue)
}
