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
Package randx provides small deterministic mixing helpers for internal runtime
distribution decisions.

This file does not implement a general-purpose random number generator. It also
does not implement cryptographic hashing. The helpers below are deterministic
bit mixers: they take a numeric input and transform it so that structured,
sequential, aligned, or otherwise correlated inputs produce better-distributed
output bits.

Why bufferpool needs bit mixing

bufferpool uses sharded data-plane structures to reduce lock contention and
localize hot-path work. A common operation is mapping some input value to a shard
or another bounded index space:

	index = value % shardCount

or, when shardCount is a power of two:

	index = value & (shardCount - 1)

The second form is very fast, but it relies heavily on the low bits of value.
That is dangerous when input values are structured. For example, monotonically
increasing counters, aligned addresses, class identifiers, pool identifiers, or
request-derived values may have weak or repetitive low-bit patterns. If raw low
bits are used directly, the selection path can overload some shards while leaving
others underused.

A mixer improves this by applying an avalanche transformation before indexing.
Avalanche means that a small input change should affect many output bits. After
mixing, even sequential inputs such as:

	1, 2, 3, 4, 5, ...

produce outputs whose low bits are much less correlated with the original
sequence. That makes fast mask-based indexing safer for shard selection.

Algorithms used here

MixUint64 uses the SplitMix64 finalizer. SplitMix64 is commonly used as a simple
non-cryptographic generator and as a high-quality integer finalizer. This file
uses only its finalization step: xor-shifts, multiplication by fixed odd
constants, and another xor-shift. The finalizer is compact, deterministic,
allocation-free, and has good avalanche behavior for 64-bit values.

MixUint32 uses the MurmurHash3 fmix32 finalizer. MurmurHash3
fmix32 is a final avalanche step from MurmurHash3's 32-bit variant. It is useful
when a caller naturally owns an uint32 value and wants a compact 32-bit
non-cryptographic mix.

Why these finalizers were chosen

The selected finalizers are appropriate for bufferpool because they are:

  - deterministic;
  - allocation-free;
  - branch-light;
  - fast enough for internal hot-path-adjacent helpers;
  - suitable for spreading structured numeric inputs;
  - small enough to keep randx as a leaf utility package;
  - independent of hash/map packages and external dependencies.

The goal is not to generate unpredictable values. The goal is to avoid accidental
shard skew when low bits of the original input are poor.

Advantages

  - Good practical avalanche behavior for simple numeric keys.
  - Cheap enough for runtime index selection.
  - Deterministic behavior makes tests and benchmarks reproducible.
  - No global state and no synchronization.
  - No dependency on math/rand, crypto/rand, hash/maphash, or external packages.
  - Works well with power-of-two index spaces after mixing.

Limitations

  - Not cryptographically secure.
  - Not suitable for secrets, tokens, authentication, or adversarial hashing.
  - Not a stable public hashing contract.
  - Not a replacement for hash/maphash when keyed hash randomization is needed.
  - Does not guarantee perfectly uniform distribution for every possible input
    set and bound.
  - BoundedIndexUint64 uses multiply-high mapping, which is appropriate for
    internal distribution but is not a cryptographic or adversarially robust
    selection primitive.

Security boundary

These helpers MUST NOT be used for security-sensitive randomness. They are
internal distribution utilities for bufferpool runtime mechanics. If a future
component needs unpredictable randomness or adversarially resistant hashing, it
must use a different primitive with an explicit security contract.
*/

const (
	// errPowerOfTwoIndexUint64ZeroBound is used when a power-of-two index is
	// requested for an empty index space.
	//
	// A zero bound cannot produce a value in [0, bound). In bufferpool internals
	// this usually means shard-count or bucket-count normalization failed before
	// the selection path was reached.
	errPowerOfTwoIndexUint64ZeroBound = "randx.PowerOfTwoIndexUint64: bound must be greater than zero"

	// errPowerOfTwoIndexUint64InvalidBound is used when the fast mask-based path
	// is requested with a non-power-of-two bound.
	//
	// Masking with bound-1 is correct only for power-of-two bounds. Callers with
	// arbitrary bounds must use BoundedIndexUint64 instead.
	errPowerOfTwoIndexUint64InvalidBound = "randx.PowerOfTwoIndexUint64: bound must be a power of two"

	// errBoundedIndexUint64ZeroBound is used when bounded index selection is
	// requested for an empty index space.
	errBoundedIndexUint64ZeroBound = "randx.BoundedIndexUint64: bound must be greater than zero"
)

const (
	// mixUint64MultiplierA is the first SplitMix64 finalizer multiplier.
	//
	// The constants used by MixUint64 are the standard SplitMix64 avalanche
	// finalizer constants. They are suitable for deterministic bit mixing and
	// hash finalization, not for cryptographic randomness.
	mixUint64MultiplierA = uint64(0xbf58476d1ce4e5b9)

	// mixUint64MultiplierB is the second SplitMix64 finalizer multiplier.
	mixUint64MultiplierB = uint64(0x94d049bb133111eb)

	// mixUint32MultiplierA is the first MurmurHash3 fmix32 multiplier.
	//
	// The constants used by MixUint32 are the standard 32-bit MurmurHash3
	// finalizer constants. They provide a compact avalanche step for uint32
	// values.
	mixUint32MultiplierA = uint32(0x85ebca6b)

	// mixUint32MultiplierB is the second MurmurHash3 fmix32 multiplier.
	mixUint32MultiplierB = uint32(0xc2b2ae35)
)

// MixUint64 returns a deterministic avalanche mix of an uint64 value.
//
// MixUint64 is a low-level bit mixer for turning structured or correlated
// numeric inputs into better-distributed bit patterns. It is not a pseudo-random
// number generator, does not maintain state, and is not cryptographically
// secure.
//
// Typical bufferpool use cases include:
//
//   - deriving shard-selection entropy from affinity keys;
//   - mixing monotonically increasing counters before indexing shards;
//   - mixing pool/class/request-derived values before bounded index selection;
//   - avoiding dependence on weak low bits in structured inputs.
//
// The function uses the SplitMix64 finalizer. The finalizer has good avalanche
// behavior for non-cryptographic runtime distribution: a small input change
// tends to change many output bits, including the low bits used by fast masked
// indexing.
//
// MixUint64 is deterministic, allocation-free, and has constant time complexity.
func MixUint64(value uint64) uint64 {
	value ^= value >> 30
	value *= mixUint64MultiplierA
	value ^= value >> 27
	value *= mixUint64MultiplierB
	value ^= value >> 31

	return value
}

// MixUint32 returns a deterministic avalanche mix of an uint32 value.
//
// MixUint32 exists for callers that naturally operate on uint32 values. Most
// bufferpool runtime code should prefer MixUint64 because sizes, counters, and
// derived keys are usually represented as uint64.
//
// The function uses the MurmurHash3 fmix32 finalizer. It is suitable for
// deterministic non-cryptographic distribution, not for security-sensitive
// randomness or stable cross-width hashing.
func MixUint32(value uint32) uint32 {
	value ^= value >> 16
	value *= mixUint32MultiplierA
	value ^= value >> 13
	value *= mixUint32MultiplierB
	value ^= value >> 16

	return value
}

// MixUint returns a deterministic avalanche mix of value using the platform uint
// width.
//
// MixUint is useful when the caller naturally owns an uint value, for example a
// length, count, index, or value returned by Go APIs. It preserves the caller's
// uint shape while selecting the appropriate uint32 or uint64 mixer for the
// target architecture.
//
// MixUint is architecture-width aware by design. For stable cross-architecture
// behavior, callers should prefer MixUint64 with an explicit uint64 input.
func MixUint(value uint) uint {
	if bits.UintSize == 64 {
		return uint(MixUint64(uint64(value)))
	}

	return uint(MixUint32(uint32(value)))
}

// PowerOfTwoIndexUint64 maps value to an index in [0, bound) using a mixed
// uint64 value and a power-of-two bound.
//
// This is the fast path for already-normalized index spaces. The function first
// applies MixUint64 and then uses a bit mask:
//
//	index = MixUint64(value) & (bound - 1)
//
// It avoids modulo division and uses mixed low bits rather than raw low bits of
// the input. That matters when caller-provided keys are sequential, aligned, or
// otherwise structured.
//
// The caller MUST pass a non-zero power-of-two bound. Invalid bounds indicate a
// bug in configuration normalization or shard-count preparation and cause a
// panic.
//
// Typical bufferpool use cases include:
//
//   - mapping an affinity key to a shard;
//   - mapping a request-derived key to a shard;
//   - deriving a fallback shard candidate when shard count is power-of-two.
func PowerOfTwoIndexUint64(value, bound uint64) uint64 {
	if bound == 0 {
		panic(errPowerOfTwoIndexUint64ZeroBound)
	}

	if !isPowerOfTwoUint64(bound) {
		panic(errPowerOfTwoIndexUint64InvalidBound)
	}

	return MixUint64(value) & (bound - 1)
}

// BoundedIndexUint64 maps value to an index in [0, bound) using a mixed uint64
// value.
//
// Unlike PowerOfTwoIndexUint64, bound does not have to be a power of two. The
// function uses the high half of a 64x64 multiplication:
//
//	index = high64(MixUint64(value) * bound)
//
// This multiply-high mapping avoids ordinary modulo division and gives good
// practical distribution for runtime index selection. It is appropriate for
// internal load-spreading decisions where deterministic, allocation-free,
// non-cryptographic distribution is needed.
//
// The caller MUST pass bound > 0. A zero bound means the caller attempted to
// select from an empty set, which is an internal programming error.
func BoundedIndexUint64(value, bound uint64) uint64 {
	if bound == 0 {
		panic(errBoundedIndexUint64ZeroBound)
	}

	high, _ := bits.Mul64(MixUint64(value), bound)
	return high
}

// isPowerOfTwoUint64 reports whether value is a non-zero power of two.
//
// randx keeps this helper local instead of importing internal/mathx so the
// low-level utility packages remain independent leaf packages. The
// implementation is the standard power-of-two bit test and should stay
// intentionally boring.
func isPowerOfTwoUint64(value uint64) bool {
	return value != 0 && value&(value-1) == 0
}
