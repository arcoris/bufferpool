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

import "sync/atomic"

const (
	// CacheLinePadSize is the padding width used by atomicx padded primitives.
	//
	// Go does not expose a stable public constant for the current CPU cache-line
	// size. Many common production targets use 64-byte cache lines; 128 bytes is
	// a conservative project default for wider or adjacent-line-sensitive systems.
	//
	// The memory cost is intentional. Padded atomics are for hot shared fields
	// where false sharing is plausible, not for ordinary object fields or
	// per-buffer metadata.
	CacheLinePadSize = 128
)

// CacheLinePad is an explicit cache-line padding block.
//
// Use it only when a struct needs manual separation between independently hot
// fields. Prefer the padded atomic wrappers below when the field itself is an
// atomic value.
type CacheLinePad struct {
	_ [CacheLinePadSize]byte
}

// PaddedUint64 is an atomic uint64 isolated from nearby fields by padding.
//
// It is intended for hot counters and gauges updated by multiple goroutines,
// such as shard operation counters, retained-byte gauges, drop counters, trim
// counters, or controller tick counters.
//
// Padding reduces false sharing: independent frequently written values should
// not occupy the same CPU cache line when they are updated from different cores.
//
// Layout:
//
//	[leading pad][atomic uint64][trailing pad]
//
// Both pads matter. The leading pad separates the value from the previous field;
// the trailing pad separates it from the next field or array/slice element.
//
// PaddedUint64 must not be copied after first use. It follows the same rule as
// sync/atomic typed values: copying an active atomic can split logical state.
type PaddedUint64 struct {
	_     CacheLinePad
	value atomic.Uint64
	_     CacheLinePad
}

// Load atomically returns the current value.
func (p *PaddedUint64) Load() uint64 {
	return p.value.Load()
}

// Store atomically replaces the current value.
func (p *PaddedUint64) Store(value uint64) {
	p.value.Store(value)
}

// Add atomically adds delta to the current value and returns the new value.
//
// Add uses unsigned arithmetic. Callers MUST NOT use it to model values that can
// legitimately become negative. For signed gauges or deltas, use PaddedInt64.
func (p *PaddedUint64) Add(delta uint64) uint64 {
	return p.value.Add(delta)
}

// Inc atomically adds one and returns the new value.
func (p *PaddedUint64) Inc() uint64 {
	return p.value.Add(1)
}

// Swap atomically stores newValue and returns the previous value.
//
// Swap is useful for snapshot-and-reset style counters when the caller owns the
// sampling semantics.
func (p *PaddedUint64) Swap(newValue uint64) uint64 {
	return p.value.Swap(newValue)
}

// CompareAndSwap atomically replaces oldValue with newValue when the current
// value still equals oldValue.
func (p *PaddedUint64) CompareAndSwap(oldValue, newValue uint64) bool {
	return p.value.CompareAndSwap(oldValue, newValue)
}

// PaddedInt64 is an atomic int64 isolated from nearby fields by padding.
//
// It is intended for signed hot values where negative intermediate states are
// meaningful, such as correction deltas, signed budget movement, or signed
// gauges maintained by lower-level runtime code.
//
// Most monotonically increasing counters should use PaddedUint64 instead.
//
// PaddedInt64 must not be copied after first use. It follows the same rule as
// sync/atomic typed values.
type PaddedInt64 struct {
	_     CacheLinePad
	value atomic.Int64
	_     CacheLinePad
}

// Load atomically returns the current value.
func (p *PaddedInt64) Load() int64 {
	return p.value.Load()
}

// Store atomically replaces the current value.
func (p *PaddedInt64) Store(value int64) {
	p.value.Store(value)
}

// Add atomically adds delta to the current value and returns the new value.
func (p *PaddedInt64) Add(delta int64) int64 {
	return p.value.Add(delta)
}

// Inc atomically adds one and returns the new value.
func (p *PaddedInt64) Inc() int64 {
	return p.value.Add(1)
}

// Dec atomically subtracts one and returns the new value.
func (p *PaddedInt64) Dec() int64 {
	return p.value.Add(-1)
}

// Swap atomically stores newValue and returns the previous value.
//
// Swap is useful for snapshot-and-reset style signed gauges or deltas when the
// caller owns the sampling semantics.
func (p *PaddedInt64) Swap(newValue int64) int64 {
	return p.value.Swap(newValue)
}

// CompareAndSwap atomically replaces oldValue with newValue when the current
// value still equals oldValue.
func (p *PaddedInt64) CompareAndSwap(oldValue, newValue int64) bool {
	return p.value.CompareAndSwap(oldValue, newValue)
}
