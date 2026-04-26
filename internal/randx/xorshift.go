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

package randx

import "math/bits"

/*
Xorshift64* source overview

This file implements a small stateful xorshift64* source for internal runtime
distribution decisions.

What xorshift is

Xorshift generators are simple pseudo-random generators based on repeated XOR
and bit-shift operations. A generator keeps an internal integer state and updates
that state using transformations such as:

	x ^= x >> a
	x ^= x << b
	x ^= x >> c

These operations are cheap and deterministic. Given the same initial state, the
generator always produces the same sequence. That property is useful for tests,
benchmarks, and reproducible runtime behavior.

Why xorshift64* instead of plain xorshift64

A plain xorshift transition is fast, but some output bits, especially low bits,
may have weaker statistical quality. xorshift64* improves the output by applying
a fixed odd multiplication after the xorshift transition. The multiplication
acts as a lightweight output finalizer and improves practical bit distribution.

This file uses the common xorshift64* transition:

	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	return x * 2685821657736338717

The internal state must be non-zero for the mathematical period guarantees of
xorshift generators. A zero state is an absorbing state for the raw transition:
shifting and XORing zero still produces zero. To make the type safe as a Go
zero-value, this implementation replaces zero state with a fixed non-zero seed
before generating the next value.

Why bufferpool needs a small stateful source

bufferpool uses sharded data-plane structures to reduce contention. Some shard
selection strategies need a cheap sequence of distributed values without using
global randomness, without allocating, and without synchronizing through a shared
source.

Examples:

  - selecting a default shard when no affinity key is provided;
  - deriving fallback shard candidates;
  - spreading probes across shards;
  - making tests and benchmarks deterministic.

This source is intentionally local and stateful. Callers should keep it in a
local selector, controller, benchmark, or goroutine-owned structure rather than
using one shared global generator.

Advantages

  - Very small state: one uint64.
  - Very fast: shifts, XORs, and one multiplication.
  - Deterministic and reproducible.
  - No allocation.
  - No global state.
  - No dependency on math/rand or crypto/rand.
  - Good enough for internal non-adversarial runtime spreading.

Limitations

  - Not cryptographically secure.
  - Not safe for secrets, tokens, authentication, or adversarial randomness.
  - Not goroutine-safe; external synchronization is required if shared.
  - Not a stable public randomness contract.
  - The zero-state repair is an implementation choice for Go zero-value
    usability, not a property of the raw xorshift64* algorithm.
  - Distribution quality is suitable for internal load spreading, not for
    statistical simulation or security-sensitive random sampling.

Security boundary

These helpers MUST NOT be used for security-sensitive randomness. Components
requiring unpredictable values must use crypto/rand or another primitive with an
explicit security contract.
*/

const (
	// errXorShift64StarPowerOfTwoIndexZeroBound is used when a power-of-two
	// index is requested for an empty index space.
	//
	// A zero bound cannot produce a value in [0, bound). In bufferpool internals
	// this usually means shard-count or fallback-count preparation failed before
	// the selection path was reached.
	errXorShift64StarPowerOfTwoIndexZeroBound = "randx.XorShift64Star.PowerOfTwoIndexUint64: bound must be greater than zero"

	// errXorShift64StarPowerOfTwoIndexInvalidBound is used when mask-based index
	// selection is requested with a non-power-of-two bound.
	//
	// Masking with bound-1 is correct only for power-of-two bounds. Callers with
	// arbitrary bounds must use BoundedIndexUint64 instead.
	errXorShift64StarPowerOfTwoIndexInvalidBound = "randx.XorShift64Star.PowerOfTwoIndexUint64: bound must be a power of two"

	// errXorShift64StarBoundedIndexZeroBound is used when bounded index selection
	// is requested for an empty index space.
	errXorShift64StarBoundedIndexZeroBound = "randx.XorShift64Star.BoundedIndexUint64: bound must be greater than zero"
)

const (
	// xorShift64StarDefaultSeed is used to make the zero-value generator usable.
	//
	// Raw xorshift generators cannot use zero as state because zero maps to zero
	// forever. Go types should be useful in their zero value when practical, so a
	// zero state is repaired to this fixed non-zero seed before generation.
	xorShift64StarDefaultSeed = uint64(0x9e3779b97f4a7c15)

	// xorShift64StarMultiplier is the output multiplier used by the common
	// xorshift64* variant.
	//
	// The multiplier is odd, which is important for preserving period properties
	// over the non-zero uint64 state space.
	xorShift64StarMultiplier = uint64(2685821657736338717)
)

// XorShift64Star is a small deterministic non-cryptographic uint64 source.
//
// The type keeps a single uint64 state and produces values with the xorshift64*
// transition. It is intended for local internal runtime distribution decisions,
// not for security-sensitive randomness.
//
// Zero value behavior:
//
//	var source XorShift64Star
//	value := source.NextUint64()
//
// The zero value is valid. Before generating a value, zero state is replaced by
// a fixed non-zero seed. This avoids the raw xorshift zero-state trap and keeps
// the type easy to embed in larger runtime structs.
//
// Concurrency:
//
// XorShift64Star is not goroutine-safe. If several goroutines need to use the
// same source, the caller must synchronize access externally. In hot data-plane
// code, prefer per-shard, per-selector, or goroutine-owned sources instead of a
// shared global source.
type XorShift64Star struct {
	state uint64
}

// NewXorShift64Star returns a source initialized with seed.
//
// A zero seed is allowed and is normalized to the package default non-zero seed.
// This keeps construction convenient while preserving the xorshift invariant
// that active state must not be zero.
func NewXorShift64Star(seed uint64) XorShift64Star {
	return XorShift64Star{
		state: normalizeXorShift64StarSeed(seed),
	}
}

// Seed replaces the source state.
//
// A zero seed is normalized to a fixed non-zero seed because zero is an
// absorbing state for raw xorshift transitions. Re-seeding is deterministic:
// using the same seed produces the same subsequent sequence.
func (s *XorShift64Star) Seed(seed uint64) {
	s.state = normalizeXorShift64StarSeed(seed)
}

// State returns the current internal state.
//
// State is exposed for deterministic tests, snapshots, and reproducible
// benchmark setup. It should not be treated as random output. Use NextUint64 to
// advance the source and obtain a generated value.
func (s *XorShift64Star) State() uint64 {
	return s.state
}

// NextUint64 advances the source and returns the next uint64 value.
//
// The returned value is the multiplied xorshift64* output. The stored state is
// the raw post-transition state, not the multiplied output. This is the standard
// shape of xorshift* generators: multiplication finalizes the output but does
// not become the next state.
//
// The method is allocation-free and constant time.
func (s *XorShift64Star) NextUint64() uint64 {
	state := nextXorShift64StarState(normalizeXorShift64StarSeed(s.state))
	s.state = state

	return state * xorShift64StarMultiplier
}

// NextUint32 advances the source and returns the low 32 bits of the next uint64
// output.
//
// Most bufferpool runtime code should prefer NextUint64. NextUint32 exists for
// callers that naturally need a uint32 value.
func (s *XorShift64Star) NextUint32() uint32 {
	return uint32(s.NextUint64())
}

// NextUint advances the source and returns a platform-width uint value.
//
// NextUint is architecture-width aware by design. For stable cross-architecture
// behavior, callers should prefer NextUint64 and convert explicitly.
func (s *XorShift64Star) NextUint() uint {
	if bits.UintSize == 64 {
		return uint(s.NextUint64())
	}

	return uint(s.NextUint32())
}

// PowerOfTwoIndexUint64 advances the source and returns an index in [0, bound)
// for a power-of-two bound.
//
// This is the fast path for already-normalized index spaces such as power-of-two
// shard counts. It uses:
//
//	index = NextUint64() & (bound - 1)
//
// The caller MUST pass a non-zero power-of-two bound. Invalid bounds indicate a
// bug in caller-side normalization and cause a panic.
func (s *XorShift64Star) PowerOfTwoIndexUint64(bound uint64) uint64 {
	if bound == 0 {
		panic(errXorShift64StarPowerOfTwoIndexZeroBound)
	}

	if !isPowerOfTwoUint64(bound) {
		panic(errXorShift64StarPowerOfTwoIndexInvalidBound)
	}

	return s.NextUint64() & (bound - 1)
}

// BoundedIndexUint64 advances the source and returns an index in [0, bound).
//
// Unlike PowerOfTwoIndexUint64, bound does not have to be a power of two. The
// function uses the high half of a 64x64 multiplication:
//
//	index = high64(NextUint64() * bound)
//
// This multiply-high mapping avoids ordinary modulo division and is appropriate
// for internal runtime load-spreading decisions.
func (s *XorShift64Star) BoundedIndexUint64(bound uint64) uint64 {
	if bound == 0 {
		panic(errXorShift64StarBoundedIndexZeroBound)
	}

	high, _ := bits.Mul64(s.NextUint64(), bound)
	return high
}

// normalizeXorShift64StarSeed returns an active non-zero generator state.
//
// Raw xorshift transitions cannot escape zero. Keeping normalization in one
// helper makes constructor, explicit seeding, and zero-value repair follow the
// same rule.
func normalizeXorShift64StarSeed(seed uint64) uint64 {
	if seed == 0 {
		return xorShift64StarDefaultSeed
	}

	return seed
}

// nextXorShift64StarState applies the xorshift64* state transition.
//
// The caller must pass a non-zero state. This helper returns the raw
// post-transition state; output multiplication is intentionally performed by
// NextUint64 so the stored state and returned value remain distinct.
func nextXorShift64StarState(state uint64) uint64 {
	state ^= state >> 12
	state ^= state << 25
	state ^= state >> 27

	return state
}
