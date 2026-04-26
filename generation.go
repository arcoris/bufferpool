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

package bufferpool

import (
	"strconv"
	"sync/atomic"
)

const (
	// errGenerationOverflow is used when a generation would advance past the
	// largest representable uint64 value.
	//
	// Generation wrap-around is deliberately forbidden. Runtime code compares
	// generations as ordinary monotonically increasing values. If wrapping were
	// allowed, an older state could appear newer than a later state, corrupting
	// snapshot publication, policy transitions, controller coordination, and
	// observability.
	errGenerationOverflow = "bufferpool.Generation: generation overflow"

	// errNilAtomicGeneration is used when AtomicGeneration methods are called on
	// a nil receiver.
	//
	// A nil atomic generation holder is always an internal wiring error. The
	// explicit panic message gives a clearer failure than a generic nil-pointer
	// panic from sync/atomic internals.
	errNilAtomicGeneration = "bufferpool.AtomicGeneration: receiver must not be nil"
)

const (
	// NoGeneration is the zero generation.
	//
	// It represents the absence of a published generation. The zero value is
	// useful for structs that have not yet published a snapshot, policy view,
	// controller cycle, or runtime state transition.
	NoGeneration Generation = 0

	// InitialGeneration is the first ordinary generation value.
	//
	// Advancing from NoGeneration produces InitialGeneration. Runtime publishers
	// can use this as the first visible generation for snapshots, policies, and
	// controller state.
	InitialGeneration Generation = 1

	// MaxGeneration is the largest representable generation.
	//
	// Advancing MaxGeneration panics instead of wrapping to NoGeneration.
	MaxGeneration Generation = ^Generation(0)
)

// Generation is a monotonically increasing runtime publication version.
//
// Generation identifies published runtime state. It is intentionally a small
// value type so it can be embedded into snapshots, metrics, policy views,
// controller reports, budget publications, and trim/contraction records without
// allocation or pointer ownership.
//
// Typical bufferpool use cases include:
//
//   - policy snapshot publication;
//   - group snapshot publication;
//   - partition snapshot publication;
//   - pool snapshot publication;
//   - controller cycle reports;
//   - budget target publication;
//   - pressure/contraction state publication;
//   - trim-plan and trim-result correlation.
//
// Generation is not a wall-clock timestamp, lifecycle state enum, or ownership
// token. It only answers whether one publication version is older, equal, or
// newer than another publication version from the same publisher stream.
//
// Values are compared using ordinary unsigned ordering. This is safe only
// because wrap-around is forbidden by Next and AtomicGeneration.Advance. Code
// must not intentionally construct wrapped generation sequences.
//
// Zero-value behavior:
//
//	var generation Generation
//
// The zero value is NoGeneration and is valid as an "unpublished" or "not yet
// initialized" marker.
type Generation uint64

// Uint64 returns generation as a raw uint64 value.
//
// This is useful for metrics, snapshots, logging, serialization boundaries, and
// tests. Runtime code should prefer Generation methods for semantic comparisons
// when possible.
func (g Generation) Uint64() uint64 {
	return uint64(g)
}

// String returns the decimal representation of generation.
//
// The method is primarily useful for diagnostics, logs, and test failure output.
func (g Generation) String() string {
	return strconv.FormatUint(uint64(g), 10)
}

// IsZero reports whether generation is NoGeneration.
//
// NoGeneration usually means no snapshot or runtime state has been published
// yet.
func (g Generation) IsZero() bool {
	return g == NoGeneration
}

// Next returns the next generation.
//
// Advancing MaxGeneration panics because wrap-around would violate the monotonic
// ordering contract used by snapshot and policy publication.
func (g Generation) Next() Generation {
	if g == MaxGeneration {
		panic(errGenerationOverflow)
	}

	return g + 1
}

// Compare compares generation with other using unsigned generation ordering.
//
// It returns:
//
//   - -1 when g is older than other;
//   - 0 when g equals other;
//   - +1 when g is newer than other.
func (g Generation) Compare(other Generation) int {
	switch {
	case g < other:
		return -1
	case g > other:
		return 1
	default:
		return 0
	}
}

// Before reports whether generation is older than other.
//
// The comparison is meaningful only inside the same generation stream. Do not
// compare generations from unrelated publishers as if they described global
// runtime order.
func (g Generation) Before(other Generation) bool {
	return g < other
}

// After reports whether generation is newer than other.
//
// The comparison is meaningful only inside the same generation stream. Do not
// compare generations from unrelated publishers as if they described global
// runtime order.
func (g Generation) After(other Generation) bool {
	return g > other
}

// AtomicGeneration is an atomic holder for one generation stream.
//
// AtomicGeneration is intended for publishers that need to expose or advance a
// generation without external locking, for example:
//
//   - policy snapshot publishers;
//   - group snapshot publishers;
//   - partition controllers;
//   - coordinator state publishers;
//   - budget publication paths;
//   - pressure/contraction state publishers.
//
// The zero value is ready to use and contains NoGeneration. Each
// AtomicGeneration instance represents one independent stream; generation values
// loaded from different instances should not be interpreted as globally ordered.
//
// AtomicGeneration MUST NOT be copied after first use. It wraps sync/atomic
// state, and copying it after use would split the logical generation stream into
// independent atomic cells.
type AtomicGeneration struct {
	value atomic.Uint64
}

// Load atomically loads and returns the current generation.
func (g *AtomicGeneration) Load() Generation {
	g.mustNotBeNil()

	return Generation(g.value.Load())
}

// Store atomically replaces the current generation.
//
// Store should be used for initialization, restoration, or explicit state
// publication. Ordinary monotonic advancement should prefer Advance so overflow
// checks and generation movement stay centralized. Store does not enforce
// monotonic movement because restore and explicit publication paths may already
// have validated their ordering externally.
func (g *AtomicGeneration) Store(generation Generation) {
	g.mustNotBeNil()

	g.value.Store(uint64(generation))
}

// Advance atomically advances the generation by one and returns the new value.
//
// If the current generation is NoGeneration, the result is InitialGeneration.
//
// Advance panics if the current generation is MaxGeneration. It never wraps.
func (g *AtomicGeneration) Advance() Generation {
	g.mustNotBeNil()

	for {
		current := Generation(g.value.Load())
		next := current.Next()

		if g.value.CompareAndSwap(uint64(current), uint64(next)) {
			return next
		}
	}
}

// CompareAndSwap executes an atomic compare-and-swap operation.
//
// It returns true when the current generation matched oldGeneration and was
// replaced by newGeneration. Like Store, this method does not enforce
// monotonicity by itself; callers that use it for publication must validate the
// transition they are attempting.
func (g *AtomicGeneration) CompareAndSwap(oldGeneration, newGeneration Generation) bool {
	g.mustNotBeNil()

	return g.value.CompareAndSwap(uint64(oldGeneration), uint64(newGeneration))
}

// mustNotBeNil validates the receiver before accessing atomic state.
//
// Methods on nil pointer receivers are legal to call in Go. Providing an
// explicit panic message makes internal wiring failures easier to diagnose.
func (g *AtomicGeneration) mustNotBeNil() {
	if g == nil {
		panic(errNilAtomicGeneration)
	}
}
