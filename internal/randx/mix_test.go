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

import (
	"math/bits"
	"testing"

	"arcoris.dev/bufferpool/internal/testutil"
)

const maxUint64 = ^uint64(0)

// TestMixUint64GoldenValues verifies stable SplitMix64 finalizer outputs.
//
// These values make the mixer implementation difficult to change accidentally.
// The function is deterministic and has no runtime state, so golden tests are
// appropriate here.
//
// The zero input maps to zero for this finalizer. That is acceptable because the
// mixer is not a random number generator. Callers that require non-zero state
// must enforce that invariant before calling MixUint64.
func TestMixUint64GoldenValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// value is the raw structured input being mixed.
		value uint64

		// want is the expected SplitMix64-finalizer output.
		want uint64
	}{
		{
			name:  "zero remains zero",
			value: 0,
			want:  0x0000000000000000,
		},
		{
			name:  "one",
			value: 1,
			want:  0x5692161d100b05e5,
		},
		{
			name:  "two",
			value: 2,
			want:  0xdbd238973a2b148a,
		},
		{
			name:  "three",
			value: 3,
			want:  0x1e535eede31428f0,
		},
		{
			name:  "structured hexadecimal value",
			value: 0x123456789abcdef0,
			want:  0x9629f58e8ec5b906,
		},
		{
			name:  "max uint64",
			value: maxUint64,
			want:  0xb4d055fcf2cbbd7b,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := MixUint64(tt.value)
			if got != tt.want {
				t.Fatalf("MixUint64(%#x) = %#x, want %#x", tt.value, got, tt.want)
			}
		})
	}
}

// TestMixUint32GoldenValues verifies stable MurmurHash3 fmix32 outputs.
//
// MixUint32 is a compact 32-bit avalanche finalizer. It is useful for callers
// that naturally own uint32 values, but most bufferpool runtime code should
// prefer MixUint64 for shard-selection keys and derived runtime values.
func TestMixUint32GoldenValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// value is the raw structured input being mixed.
		value uint32

		// want is the expected MurmurHash3 fmix32 output.
		want uint32
	}{
		{
			name:  "zero remains zero",
			value: 0,
			want:  0x00000000,
		},
		{
			name:  "one",
			value: 1,
			want:  0x514e28b7,
		},
		{
			name:  "two",
			value: 2,
			want:  0x30f4c306,
		},
		{
			name:  "three",
			value: 3,
			want:  0x85f0b427,
		},
		{
			name:  "structured hexadecimal value",
			value: 0x12345678,
			want:  0xe37cd1bc,
		},
		{
			name:  "max uint32",
			value: ^uint32(0),
			want:  0x81f16f39,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := MixUint32(tt.value)
			if got != tt.want {
				t.Fatalf("MixUint32(%#x) = %#x, want %#x", tt.value, got, tt.want)
			}
		})
	}
}

// TestMixUint64IsDeterministic verifies that MixUint64 has no hidden state.
//
// randx mixers are deterministic bit mixers, not pseudo-random generators. The
// same input must always produce the same output so shard-selection behavior is
// reproducible in tests and benchmarks.
func TestMixUint64IsDeterministic(t *testing.T) {
	t.Parallel()

	values := []uint64{
		0,
		1,
		2,
		3,
		17,
		1024,
		0x123456789abcdef0,
		^uint64(0),
	}

	for _, value := range values {
		value := value

		t.Run("representative value", func(t *testing.T) {
			t.Parallel()

			first := MixUint64(value)
			second := MixUint64(value)

			if first != second {
				t.Fatalf("MixUint64(%#x) is not deterministic: first=%#x second=%#x", value, first, second)
			}
		})
	}
}

// TestMixUint32IsDeterministic verifies that MixUint32 has no hidden state.
//
// This mirrors the uint64 deterministic behavior for callers that use the 32-bit
// finalizer explicitly.
func TestMixUint32IsDeterministic(t *testing.T) {
	t.Parallel()

	values := []uint32{
		0,
		1,
		2,
		3,
		17,
		1024,
		0x12345678,
		^uint32(0),
	}

	for _, value := range values {
		value := value

		t.Run("representative value", func(t *testing.T) {
			t.Parallel()

			first := MixUint32(value)
			second := MixUint32(value)

			if first != second {
				t.Fatalf("MixUint32(%#x) is not deterministic: first=%#x second=%#x", value, first, second)
			}
		})
	}
}

// TestMixUint64NearbyInputsDiffer documents the avalanche-oriented behavior
// expected from the 64-bit mixer.
//
// This is a smoke test, not a statistical proof. It protects against accidental
// identity implementations, missing multipliers, or broken shift sequences.
func TestMixUint64NearbyInputsDiffer(t *testing.T) {
	t.Parallel()

	previous := MixUint64(1)

	for value := uint64(2); value <= 64; value++ {
		current := MixUint64(value)
		if current == previous {
			t.Fatalf("MixUint64(%d) = MixUint64(%d) = %#x", value-1, value, current)
		}

		previous = current
	}
}

// TestMixUint32NearbyInputsDiffer documents the avalanche-oriented behavior
// expected from the 32-bit mixer.
//
// The test intentionally checks only a representative sequential range. Broad
// distribution quality belongs to benchmarks or statistical tests.
func TestMixUint32NearbyInputsDiffer(t *testing.T) {
	t.Parallel()

	previous := MixUint32(1)

	for value := uint32(2); value <= 64; value++ {
		current := MixUint32(value)
		if current == previous {
			t.Fatalf("MixUint32(%d) = MixUint32(%d) = %#x", value-1, value, current)
		}

		previous = current
	}
}

// TestMixUintDispatchesByPlatformWidth verifies that MixUint follows Go's uint
// width for the current target architecture.
//
// MixUint is intentionally architecture-width aware. Callers that require stable
// cross-architecture behavior should use MixUint64 explicitly.
func TestMixUintDispatchesByPlatformWidth(t *testing.T) {
	t.Parallel()

	value := uint(0x12345678)
	got := MixUint(value)

	if bits.UintSize == 64 {
		want := uint(MixUint64(uint64(value)))
		if got != want {
			t.Fatalf("MixUint(%#x) = %#x, want MixUint64-dispatched value %#x", value, got, want)
		}

		return
	}

	want := uint(MixUint32(uint32(value)))
	if got != want {
		t.Fatalf("MixUint(%#x) = %#x, want MixUint32-dispatched value %#x", value, got, want)
	}
}

// TestPowerOfTwoIndexUint64MatchesMaskedMixedValue verifies the fast
// power-of-two index path.
//
// For power-of-two bounds, mask-based indexing is valid and fast:
//
//	index = MixUint64(value) & (bound - 1)
//
// The test fixes that relationship explicitly so future refactors cannot change
// the index semantics accidentally.
func TestPowerOfTwoIndexUint64MatchesMaskedMixedValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// value is the raw key before mixing.
		value uint64

		// bound is the size of the index space and must be a power of two.
		bound uint64
	}{
		{
			name:  "single slot",
			value: 123,
			bound: 1,
		},
		{
			name:  "two slots",
			value: 123,
			bound: 2,
		},
		{
			name:  "typical small shard count",
			value: 123,
			bound: 8,
		},
		{
			name:  "typical large shard count",
			value: 0x123456789abcdef0,
			bound: 64,
		},
		{
			name:  "large power-of-two bound",
			value: maxUint64,
			bound: 1 << 20,
		},
		{
			name:  "largest power-of-two bound",
			value: 0x123456789abcdef0,
			bound: uint64(1) << 63,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := PowerOfTwoIndexUint64(tt.value, tt.bound)
			want := MixUint64(tt.value) & (tt.bound - 1)

			if got != want {
				t.Fatalf("PowerOfTwoIndexUint64(%#x, %d) = %d, want %d", tt.value, tt.bound, got, want)
			}

			if got >= tt.bound {
				t.Fatalf("PowerOfTwoIndexUint64(%#x, %d) = %d, want value in [0, %d)", tt.value, tt.bound, got, tt.bound)
			}
		})
	}
}

// TestPowerOfTwoIndexUint64SingleSlotAlwaysReturnsZero verifies the degenerate
// non-empty power-of-two index space.
//
// A bound of one is valid and should always produce index zero. This case is
// useful for runtime configurations that temporarily collapse a partition or
// shard group to a single local target.
func TestPowerOfTwoIndexUint64SingleSlotAlwaysReturnsZero(t *testing.T) {
	t.Parallel()

	values := []uint64{
		0,
		1,
		2,
		17,
		0x123456789abcdef0,
		maxUint64,
	}

	for _, value := range values {
		value := value

		t.Run("representative value", func(t *testing.T) {
			t.Parallel()

			if got := PowerOfTwoIndexUint64(value, 1); got != 0 {
				t.Fatalf("PowerOfTwoIndexUint64(%#x, 1) = %d, want 0", value, got)
			}
		})
	}
}

// TestPowerOfTwoIndexUint64PanicsForInvalidBounds verifies invariant guards for
// the fast masked path.
//
// The mask path is correct only for non-zero power-of-two bounds. A bad bound
// means shard-count or table-size normalization failed before the selection path
// was reached.
func TestPowerOfTwoIndexUint64PanicsForInvalidBounds(t *testing.T) {
	t.Parallel()

	t.Run("zero bound", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errPowerOfTwoIndexUint64ZeroBound, func() {
			_ = PowerOfTwoIndexUint64(123, 0)
		})
	})

	t.Run("non power of two bound", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errPowerOfTwoIndexUint64InvalidBound, func() {
			_ = PowerOfTwoIndexUint64(123, 12)
		})
	})
}

// TestBoundedIndexUint64MatchesMultiplyHighMapping verifies the arbitrary-bound
// index path.
//
// BoundedIndexUint64 maps a mixed uint64 value to [0, bound) using the high half
// of a 64x64 multiplication. This avoids ordinary modulo division and is useful
// for deterministic runtime load-spreading decisions.
func TestBoundedIndexUint64MatchesMultiplyHighMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// value is the raw key before mixing.
		value uint64

		// bound is the arbitrary non-zero index-space size.
		bound uint64
	}{
		{
			name:  "single slot",
			value: 123,
			bound: 1,
		},
		{
			name:  "two slots",
			value: 123,
			bound: 2,
		},
		{
			name:  "non power of two small bound",
			value: 123,
			bound: 10,
		},
		{
			name:  "non power of two large bound",
			value: 0x123456789abcdef0,
			bound: 1000,
		},
		{
			name:  "large bound",
			value: maxUint64,
			bound: 1<<32 + 15,
		},
		{
			name:  "max uint64 bound",
			value: 0x123456789abcdef0,
			bound: maxUint64,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := BoundedIndexUint64(tt.value, tt.bound)

			want, _ := bits.Mul64(MixUint64(tt.value), tt.bound)
			if got != want {
				t.Fatalf("BoundedIndexUint64(%#x, %d) = %d, want %d", tt.value, tt.bound, got, want)
			}

			if got >= tt.bound {
				t.Fatalf("BoundedIndexUint64(%#x, %d) = %d, want value in [0, %d)", tt.value, tt.bound, got, tt.bound)
			}
		})
	}
}

// TestBoundedIndexUint64SingleSlotAlwaysReturnsZero verifies the degenerate
// non-empty arbitrary-bound index space.
//
// The multiply-high mapping should behave sensibly for bound == 1: every input
// maps to the only valid index.
func TestBoundedIndexUint64SingleSlotAlwaysReturnsZero(t *testing.T) {
	t.Parallel()

	values := []uint64{
		0,
		1,
		2,
		17,
		0x123456789abcdef0,
		maxUint64,
	}

	for _, value := range values {
		value := value

		t.Run("representative value", func(t *testing.T) {
			t.Parallel()

			if got := BoundedIndexUint64(value, 1); got != 0 {
				t.Fatalf("BoundedIndexUint64(%#x, 1) = %d, want 0", value, got)
			}
		})
	}
}

// TestBoundedIndexUint64PanicsForZeroBound verifies that selecting from an empty
// index space is rejected.
//
// A zero bound is always a caller bug. There is no valid index in [0, 0).
func TestBoundedIndexUint64PanicsForZeroBound(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errBoundedIndexUint64ZeroBound, func() {
		_ = BoundedIndexUint64(123, 0)
	})
}

// TestIndexHelpersCoverRepresentativeSequentialInputs is a distribution smoke
// test for structured sequential values.
//
// This is not a statistical proof and must not be treated as a randomness test.
// Its purpose is narrower: catch severe implementation errors where all or most
// sequential values collapse into a small subset of available indexes.
func TestIndexHelpersCoverRepresentativeSequentialInputs(t *testing.T) {
	t.Parallel()

	const samples = 4096

	t.Run("power of two bound covers all slots", func(t *testing.T) {
		t.Parallel()

		const bound = 16

		seen := make([]bool, bound)

		for value := uint64(0); value < samples; value++ {
			index := PowerOfTwoIndexUint64(value, bound)
			seen[index] = true
		}

		for index, ok := range seen {
			if !ok {
				t.Fatalf("PowerOfTwoIndexUint64 did not produce index %d for %d sequential samples", index, samples)
			}
		}
	})

	t.Run("arbitrary bound covers all slots", func(t *testing.T) {
		t.Parallel()

		const bound = 10

		seen := make([]bool, bound)

		for value := uint64(0); value < samples; value++ {
			index := BoundedIndexUint64(value, bound)
			seen[index] = true
		}

		for index, ok := range seen {
			if !ok {
				t.Fatalf("BoundedIndexUint64 did not produce index %d for %d sequential samples", index, samples)
			}
		}
	})
}

// TestIsPowerOfTwoUint64 verifies randx's local power-of-two predicate.
//
// randx keeps this helper local to avoid creating dependencies between low-level
// leaf utility packages. The test protects the local implementation from
// accidental drift.
func TestIsPowerOfTwoUint64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// value is the candidate bound.
		value uint64

		// want is true only for non-zero powers of two.
		want bool
	}{
		{
			name:  "zero",
			value: 0,
			want:  false,
		},
		{
			name:  "one",
			value: 1,
			want:  true,
		},
		{
			name:  "two",
			value: 2,
			want:  true,
		},
		{
			name:  "three",
			value: 3,
			want:  false,
		},
		{
			name:  "eight",
			value: 8,
			want:  true,
		},
		{
			name:  "twelve",
			value: 12,
			want:  false,
		},
		{
			name:  "largest power of two",
			value: uint64(1) << 63,
			want:  true,
		},
		{
			name:  "max uint64",
			value: maxUint64,
			want:  false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isPowerOfTwoUint64(tt.value)
			if got != tt.want {
				t.Fatalf("isPowerOfTwoUint64(%d) = %t, want %t", tt.value, got, tt.want)
			}
		})
	}
}
