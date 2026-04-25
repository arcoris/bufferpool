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

package mathx

import "math/bits"

const (
	// errNextPowerOfTwoZero is used when NextPowerOfTwo receives zero.
	//
	// Zero is rejected deliberately. Although some low-level implementations
	// define nextPowerOfTwo(0) as 1, doing so in bufferpool internals would hide
	// invalid configuration or broken runtime normalization. Callers that want a
	// minimum of 1 MUST apply that policy explicitly before calling this helper
	// or through a higher-level configuration/defaulting path.
	errNextPowerOfTwoZero = "mathx.NextPowerOfTwo: value must be greater than zero"

	// errNextPowerOfTwoOverflow is used when the next power of two cannot be
	// represented as uint64.
	//
	// The largest power of two representable by uint64 is 1 << 63. Values above
	// that boundary would require 1 << 64 after rounding, which overflows uint64.
	errNextPowerOfTwoOverflow = "mathx.NextPowerOfTwo: next power of two overflows uint64"
)

const (
	// maxPowerOfTwoUint64 is the largest power of two representable by uint64.
	//
	// uint64 can represent values up to 2^64 - 1, but 2^64 itself is not
	// representable. Therefore, the largest valid power-of-two result is 2^63.
	maxPowerOfTwoUint64 = uint64(1) << 63
)

// IsPowerOfTwo reports whether value is a non-zero power of two.
//
// A value is a power of two when exactly one bit is set. Zero is not considered
// a power of two because it does not represent a valid positive size, count, or
// capacity in bufferpool runtime math.
//
// Typical bufferpool use cases include:
//
//   - validating shard counts;
//   - validating segment slot counts;
//   - validating class sizes when a power-of-two class table is required;
//   - checking already-normalized capacities or limits before applying them to
//     runtime state.
//
// The function is allocation-free and has constant time complexity.
func IsPowerOfTwo(value uint64) bool {
	return value != 0 && value&(value-1) == 0
}

// NextPowerOfTwo returns value rounded up to the nearest power of two.
//
// If value is already a power of two, it is returned unchanged.
//
// Examples:
//
//   - 1 returns 1;
//   - 2 returns 2;
//   - 3 returns 4;
//   - 7 returns 8;
//   - 8 returns 8.
//
// This helper is intended for low-level runtime normalization where power-of-two
// sizing improves indexing, masking, sharding, or bounded storage behavior.
//
// Typical bufferpool use cases include:
//
//   - deriving shard counts from runtime concurrency hints;
//   - normalizing segment slot counts;
//   - normalizing internal table capacities;
//   - preparing values that will later be clamped to configured min/max bounds.
//
// The caller MUST pass value > 0. Zero is treated as an internal programming
// error and causes a panic.
//
// The caller MUST also ensure that the rounded value can fit into uint64. If the
// next power of two would exceed 1 << 63, the function panics instead of
// wrapping or returning a misleading value.
//
// The function is allocation-free and has constant time complexity.
func NextPowerOfTwo(value uint64) uint64 {
	if value == 0 {
		panic(errNextPowerOfTwoZero)
	}

	if value > maxPowerOfTwoUint64 {
		panic(errNextPowerOfTwoOverflow)
	}

	// If value is already a power of two, value-1 has all lower bits set and
	// bits.Len64(value-1) produces the exponent of value. If value is not a power
	// of two, bits.Len64(value-1) produces the exponent of the next higher power
	// of two.
	//
	// Examples:
	//
	//	value = 1: value-1 = 0, Len64(0) = 0, 1 << 0 = 1
	//	value = 2: value-1 = 1, Len64(1) = 1, 1 << 1 = 2
	//	value = 3: value-1 = 2, Len64(2) = 2, 1 << 2 = 4
	//	value = 8: value-1 = 7, Len64(7) = 3, 1 << 3 = 8
	return uint64(1) << bits.Len64(value-1)
}
