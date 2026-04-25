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

import (
	"testing"

	"arcoris.dev/bufferpool/internal/testutil"
)

// TestIsPowerOfTwo verifies the canonical power-of-two predicate.
//
// Power-of-two checks are used by bufferpool internals for values where bitwise
// layout matters: shard counts, segment slot counts, class sizes, and internal
// table capacities. The predicate must be strict: zero is not a power of two,
// and values with more than one bit set must be rejected.
func TestIsPowerOfTwo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// value is the candidate unsigned runtime value being checked.
		value uint64

		// want is true only when value is non-zero and has exactly one bit set.
		want bool
	}{
		{
			name:  "zero is not power of two",
			value: 0,
			want:  false,
		},
		{
			name:  "one is power of two",
			value: 1,
			want:  true,
		},
		{
			name:  "two is power of two",
			value: 2,
			want:  true,
		},
		{
			name:  "three is not power of two",
			value: 3,
			want:  false,
		},
		{
			name:  "four is power of two",
			value: 4,
			want:  true,
		},
		{
			name:  "seven is not power of two",
			value: 7,
			want:  false,
		},
		{
			name:  "eight is power of two",
			value: 8,
			want:  true,
		},
		{
			name:  "typical shard count is power of two",
			value: 32,
			want:  true,
		},
		{
			name:  "typical non-power shard count is rejected",
			value: 48,
			want:  false,
		},
		{
			name:  "typical segment slot count is power of two",
			value: 1024,
			want:  true,
		},
		{
			name:  "large power of two is accepted",
			value: uint64(1) << 62,
			want:  true,
		},
		{
			name:  "largest representable power of two is accepted",
			value: uint64(1) << 63,
			want:  true,
		},
		{
			name:  "max uint64 is not power of two",
			value: ^uint64(0),
			want:  false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := IsPowerOfTwo(tt.value)
			if got != tt.want {
				t.Fatalf("IsPowerOfTwo(%d) = %t, want %t", tt.value, got, tt.want)
			}
		})
	}
}

// TestNextPowerOfTwoReturnsInputWhenAlreadyPowerOfTwo verifies that
// NextPowerOfTwo is idempotent for valid power-of-two inputs.
//
// This matters for configuration normalization: a caller may pass an already
// normalized shard count, segment size, class size, or table capacity. The
// helper must not change such values.
func TestNextPowerOfTwoReturnsInputWhenAlreadyPowerOfTwo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// value is already a valid power of two.
		value uint64
	}{
		{
			name:  "one",
			value: 1,
		},
		{
			name:  "two",
			value: 2,
		},
		{
			name:  "four",
			value: 4,
		},
		{
			name:  "eight",
			value: 8,
		},
		{
			name:  "typical shard count",
			value: 32,
		},
		{
			name:  "typical segment slot count",
			value: 1024,
		},
		{
			name:  "large class size",
			value: 1 << 20,
		},
		{
			name:  "largest representable power of two",
			value: uint64(1) << 63,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := NextPowerOfTwo(tt.value)
			if got != tt.value {
				t.Fatalf("NextPowerOfTwo(%d) = %d, want %d", tt.value, got, tt.value)
			}
		})
	}
}

// TestNextPowerOfTwoRoundsUp verifies that non-power-of-two values are rounded
// up to the nearest representable power of two.
//
// This is the core behavior needed when bufferpool derives internal geometry
// from runtime or configuration hints, for example deriving a power-of-two shard
// count from GOMAXPROCS or normalizing a requested segment slot count.
func TestNextPowerOfTwoRoundsUp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// value is a positive non-power-of-two value.
		value uint64

		// want is the smallest power of two greater than value.
		want uint64
	}{
		{
			name:  "three rounds to four",
			value: 3,
			want:  4,
		},
		{
			name:  "five rounds to eight",
			value: 5,
			want:  8,
		},
		{
			name:  "six rounds to eight",
			value: 6,
			want:  8,
		},
		{
			name:  "seven rounds to eight",
			value: 7,
			want:  8,
		},
		{
			name:  "nine rounds to sixteen",
			value: 9,
			want:  16,
		},
		{
			name:  "gomaxprocs-like value rounds to next shard count",
			value: 24,
			want:  32,
		},
		{
			name:  "segment hint rounds to next slot count",
			value: 1000,
			want:  1024,
		},
		{
			name:  "class-like size rounds to next power of two",
			value: 3000,
			want:  4096,
		},
		{
			name:  "large value below two to the sixty two rounds up",
			value: (uint64(1) << 62) - 1,
			want:  uint64(1) << 62,
		},
		{
			name:  "large value below maximum representable power rounds up",
			value: (uint64(1) << 63) - 1,
			want:  uint64(1) << 63,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := NextPowerOfTwo(tt.value)
			if got != tt.want {
				t.Fatalf("NextPowerOfTwo(%d) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

// TestNextPowerOfTwoPanicsForZero verifies that zero is rejected.
//
// Returning 1 for zero would hide invalid runtime normalization. In bufferpool,
// zero shard count, zero segment size, or zero class size is not a harmless
// input; it usually indicates a broken caller, incomplete defaulting, or invalid
// configuration that should have been caught earlier.
func TestNextPowerOfTwoPanicsForZero(t *testing.T) {
	t.Parallel()

	testutil.MustPanic(t, func() {
		_ = NextPowerOfTwo(0)
	})
}

// TestNextPowerOfTwoPanicsForOverflow verifies that values requiring 2^64 after
// rounding are rejected instead of overflowing.
//
// uint64 can represent up to 2^64 - 1, but 2^64 itself is not representable.
// Therefore the largest valid power-of-two result is 2^63. Any input greater
// than 2^63 would need to round to 2^64 and must panic.
func TestNextPowerOfTwoPanicsForOverflow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// value is greater than the largest representable power-of-two result.
		value uint64
	}{
		{
			name:  "one above largest representable power of two",
			value: (uint64(1) << 63) + 1,
		},
		{
			name:  "max uint64",
			value: ^uint64(0),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testutil.MustPanic(t, func() {
				_ = NextPowerOfTwo(tt.value)
			})
		})
	}
}

// TestNextPowerOfTwoResultIsPowerOfTwo verifies the structural postcondition of
// NextPowerOfTwo over representative runtime values.
//
// The exact expected values are checked elsewhere. This test documents the
// broader invariant: every successful result must be a non-zero power of two and
// must be greater than or equal to the input.
func TestNextPowerOfTwoResultIsPowerOfTwo(t *testing.T) {
	t.Parallel()

	values := []uint64{
		1,
		2,
		3,
		4,
		5,
		7,
		8,
		15,
		16,
		24,
		31,
		32,
		63,
		64,
		1000,
		1023,
		1024,
		3000,
		4095,
		4096,
		(1 << 20) - 1,
		1 << 20,
		(uint64(1) << 62) - 1,
		uint64(1) << 62,
		(uint64(1) << 63) - 1,
		uint64(1) << 63,
	}

	for _, value := range values {
		value := value

		t.Run("representative value", func(t *testing.T) {
			t.Parallel()

			got := NextPowerOfTwo(value)

			if got < value {
				t.Fatalf("NextPowerOfTwo(%d) = %d, want result >= input", value, got)
			}

			if !IsPowerOfTwo(got) {
				t.Fatalf("NextPowerOfTwo(%d) = %d, want power of two", value, got)
			}
		})
	}
}
