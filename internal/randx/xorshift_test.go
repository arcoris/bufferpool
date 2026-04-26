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

// TestNewXorShift64StarNormalizesZeroSeed verifies constructor behavior for
// zero seeds.
//
// Raw xorshift generators cannot use zero as an active state because zero is an
// absorbing state: shifts and XORs keep it zero forever. The constructor
// normalizes zero to a fixed non-zero default seed so callers can safely pass
// zero when they want deterministic default behavior.
func TestNewXorShift64StarNormalizesZeroSeed(t *testing.T) {
	t.Parallel()

	source := NewXorShift64Star(0)

	if got := source.State(); got != xorShift64StarDefaultSeed {
		t.Fatalf("NewXorShift64Star(0).State() = %#x, want %#x", got, xorShift64StarDefaultSeed)
	}
}

// TestNewXorShift64StarPreservesNonZeroSeed verifies constructor behavior for
// explicit non-zero seeds.
//
// Reproducible shard-selection tests and benchmarks depend on the source keeping
// non-zero seeds exactly as supplied.
func TestNewXorShift64StarPreservesNonZeroSeed(t *testing.T) {
	t.Parallel()

	const seed = uint64(0x123456789abcdef0)

	source := NewXorShift64Star(seed)

	if got := source.State(); got != seed {
		t.Fatalf("NewXorShift64Star(%#x).State() = %#x, want %#x", seed, got, seed)
	}
}

// TestXorShift64StarSeedNormalizesZero verifies zero normalization through the
// mutating Seed method.
//
// This protects the same invariant as the constructor: active generator state
// must not remain zero after explicit seeding.
func TestXorShift64StarSeedNormalizesZero(t *testing.T) {
	t.Parallel()

	var source XorShift64Star

	source.Seed(0)

	if got := source.State(); got != xorShift64StarDefaultSeed {
		t.Fatalf("Seed(0) state = %#x, want %#x", got, xorShift64StarDefaultSeed)
	}
}

// TestXorShift64StarSeedPreservesNonZero verifies that Seed keeps explicit
// non-zero state unchanged.
//
// This is important because using the same seed must reproduce the same future
// sequence exactly.
func TestXorShift64StarSeedPreservesNonZero(t *testing.T) {
	t.Parallel()

	const seed = uint64(0xfeedfacecafebeef)

	var source XorShift64Star
	source.Seed(seed)

	if got := source.State(); got != seed {
		t.Fatalf("Seed(%#x) state = %#x, want %#x", seed, got, seed)
	}
}

// TestXorShift64StarZeroValueIsUsable verifies Go zero-value usability.
//
// The raw state of a zero-value source is zero before first use. NextUint64 must
// repair that state to the deterministic default seed internally and then
// advance normally. This makes the type safe to embed in larger runtime structs
// without explicit construction.
func TestXorShift64StarZeroValueIsUsable(t *testing.T) {
	t.Parallel()

	var source XorShift64Star

	if got := source.State(); got != 0 {
		t.Fatalf("zero-value State() = %#x, want 0", got)
	}

	got := source.NextUint64()

	if got != 0x0d83b3e29a21487a {
		t.Fatalf("zero-value first NextUint64() = %#x, want %#x", got, uint64(0x0d83b3e29a21487a))
	}

	if gotState := source.State(); gotState != 0x03f721dffe39b342 {
		t.Fatalf("state after zero-value first NextUint64() = %#x, want %#x", gotState, uint64(0x03f721dffe39b342))
	}
}

// TestXorShift64StarNextUint64GoldenSequence verifies stable xorshift64* output
// for representative seeds.
//
// Golden tests are appropriate here because the source is deterministic and has
// no external state. If these values change, the generator algorithm, seed
// repair, output multiplier, or state transition changed.
func TestXorShift64StarNextUint64GoldenSequence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// seed initializes the source before the sequence is generated.
		seed uint64

		// want is the expected sequence of NextUint64 outputs.
		want []uint64
	}{
		{
			name: "default seed",
			seed: xorShift64StarDefaultSeed,
			want: []uint64{
				0x0d83b3e29a21487a,
				0x54c44c79f1fe9d67,
				0xa845f342007a0e78,
			},
		},
		{
			name: "seed one",
			seed: 1,
			want: []uint64{
				0x47e4ce4b896cdd1d,
				0xabcfa6a8e079651d,
				0xb9d10d8feb731f57,
			},
		},
		{
			name: "structured hexadecimal seed",
			seed: 0x123456789abcdef0,
			want: []uint64{
				0xb7fb0288c5ee4339,
				0x42fef730e71e2254,
				0x835d6ba41ba14966,
			},
		},
		{
			name: "max uint64 seed",
			seed: maxUint64,
			want: []uint64{
				0xf92cc9e5c6000000,
				0x8ff484d8fd1eaee3,
				0x346c95f3326fabc6,
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source := NewXorShift64Star(tt.seed)

			for index, want := range tt.want {
				got := source.NextUint64()
				if got != want {
					t.Fatalf("NextUint64 output %d for seed %#x = %#x, want %#x", index, tt.seed, got, want)
				}
			}
		})
	}
}

// TestXorShift64StarSeedRestartsSequence verifies deterministic reseeding.
//
// Re-seeding with a previously used seed should restart the same sequence. This
// matters for reproducible benchmarks and tests that intentionally reset local
// shard-selection sources.
func TestXorShift64StarSeedRestartsSequence(t *testing.T) {
	t.Parallel()

	const seed = uint64(0x123456789abcdef0)

	source := NewXorShift64Star(seed)
	first := source.NextUint64()

	source.NextUint64()
	source.NextUint64()
	source.Seed(seed)

	if got := source.NextUint64(); got != first {
		t.Fatalf("first output after reseed = %#x, want %#x", got, first)
	}
}

// TestXorShift64StarStateTracksRawTransition verifies that State returns the raw
// post-transition state, not the multiplied output.
//
// xorshift* generators usually multiply only the output. The stored state should
// remain the raw xorshift transition state so future steps follow the intended
// sequence.
func TestXorShift64StarStateTracksRawTransition(t *testing.T) {
	t.Parallel()

	source := NewXorShift64Star(1)

	output := source.NextUint64()
	if output != 0x47e4ce4b896cdd1d {
		t.Fatalf("first output = %#x, want %#x", output, uint64(0x47e4ce4b896cdd1d))
	}

	if got := source.State(); got != 0x0000000002000001 {
		t.Fatalf("state after first output = %#x, want %#x", got, uint64(0x0000000002000001))
	}
}

// TestXorShift64StarIsDeterministic verifies that identical seeds produce
// identical sequences.
//
// The source is used for deterministic internal distribution, reproducible
// benchmarks, and stable tests. It must not depend on global time, process state,
// or shared randomness.
func TestXorShift64StarIsDeterministic(t *testing.T) {
	t.Parallel()

	const seed = uint64(0x123456789abcdef0)
	const samples = 32

	first := NewXorShift64Star(seed)
	second := NewXorShift64Star(seed)

	for index := 0; index < samples; index++ {
		a := first.NextUint64()
		b := second.NextUint64()

		if a != b {
			t.Fatalf("sample %d differs for identical seeds: first=%#x second=%#x", index, a, b)
		}
	}
}

// TestXorShift64StarDifferentSeedsProduceDifferentInitialOutputs verifies a
// basic distribution sanity property.
//
// This is not a statistical proof. It protects against severe implementation
// errors such as ignoring the seed, always using the default seed, or returning a
// constant output.
func TestXorShift64StarDifferentSeedsProduceDifferentInitialOutputs(t *testing.T) {
	t.Parallel()

	first := NewXorShift64Star(1)
	second := NewXorShift64Star(2)

	firstValue := first.NextUint64()
	secondValue := second.NextUint64()

	if firstValue == secondValue {
		t.Fatalf("different seeds produced same first output: %#x", firstValue)
	}
}

// TestXorShift64StarNextUint32 verifies that NextUint32 returns the low 32 bits
// of the next uint64 output.
//
// Most bufferpool runtime code should use NextUint64, but this method is useful
// for callers that naturally need a uint32 value.
func TestXorShift64StarNextUint32(t *testing.T) {
	t.Parallel()

	source := NewXorShift64Star(1)

	got := source.NextUint32()
	want := uint32(0x896cdd1d)

	if got != want {
		t.Fatalf("NextUint32() = %#x, want %#x", got, want)
	}
}

// TestXorShift64StarNextUintDispatchesByPlatformWidth verifies platform-width
// behavior for NextUint.
//
// NextUint is intentionally architecture-width aware. Callers that need stable
// cross-architecture output must use NextUint64 explicitly.
func TestXorShift64StarNextUintDispatchesByPlatformWidth(t *testing.T) {
	t.Parallel()

	source := NewXorShift64Star(1)
	got := source.NextUint()

	if bits.UintSize == 64 {
		want := uint(0x47e4ce4b896cdd1d)
		if got != want {
			t.Fatalf("NextUint() = %#x, want uint64-width value %#x", got, want)
		}

		return
	}

	want := uint(0x896cdd1d)
	if got != want {
		t.Fatalf("NextUint() = %#x, want uint32-width value %#x", got, want)
	}
}

// TestXorShift64StarPowerOfTwoIndexUint64MatchesMaskedOutput verifies the
// power-of-two index path.
//
// The method should advance the source once and return:
//
//	index = NextUint64() & (bound - 1)
//
// This is the fast path for already-normalized power-of-two index spaces such as
// shard counts.
func TestXorShift64StarPowerOfTwoIndexUint64MatchesMaskedOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// seed initializes both the expected-output source and the tested source.
		seed uint64

		// bound is the power-of-two index-space size.
		bound uint64
	}{
		{
			name:  "single slot",
			seed:  1,
			bound: 1,
		},
		{
			name:  "two slots",
			seed:  1,
			bound: 2,
		},
		{
			name:  "typical shard count",
			seed:  1,
			bound: 16,
		},
		{
			name:  "larger shard count",
			seed:  0x123456789abcdef0,
			bound: 64,
		},
		{
			name:  "large power-of-two bound",
			seed:  maxUint64,
			bound: 1 << 20,
		},
		{
			name:  "largest power-of-two bound",
			seed:  0x123456789abcdef0,
			bound: uint64(1) << 63,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			expectedSource := NewXorShift64Star(tt.seed)
			testedSource := NewXorShift64Star(tt.seed)

			want := expectedSource.NextUint64() & (tt.bound - 1)
			got := testedSource.PowerOfTwoIndexUint64(tt.bound)

			if got != want {
				t.Fatalf("PowerOfTwoIndexUint64(%d) = %d, want %d", tt.bound, got, want)
			}

			if got >= tt.bound {
				t.Fatalf("PowerOfTwoIndexUint64(%d) = %d, want value in [0, %d)", tt.bound, got, tt.bound)
			}

			if testedSource.State() != expectedSource.State() {
				t.Fatalf("state after PowerOfTwoIndexUint64 = %#x, want %#x", testedSource.State(), expectedSource.State())
			}
		})
	}
}

// TestXorShift64StarPowerOfTwoIndexUint64PanicsForInvalidBounds verifies
// invariant guards for the fast masked index path.
//
// Masking with bound-1 is correct only for non-zero power-of-two bounds. Invalid
// bounds indicate broken caller-side normalization.
func TestXorShift64StarPowerOfTwoIndexUint64PanicsForInvalidBounds(t *testing.T) {
	t.Parallel()

	t.Run("zero bound", func(t *testing.T) {
		t.Parallel()

		source := NewXorShift64Star(1)
		before := source.State()

		testutil.MustPanicWithMessage(t, errXorShift64StarPowerOfTwoIndexZeroBound, func() {
			_ = source.PowerOfTwoIndexUint64(0)
		})

		if got := source.State(); got != before {
			t.Fatalf("state changed after zero-bound panic: got %#x, want %#x", got, before)
		}
	})

	t.Run("non power of two bound", func(t *testing.T) {
		t.Parallel()

		source := NewXorShift64Star(1)
		before := source.State()

		testutil.MustPanicWithMessage(t, errXorShift64StarPowerOfTwoIndexInvalidBound, func() {
			_ = source.PowerOfTwoIndexUint64(12)
		})

		if got := source.State(); got != before {
			t.Fatalf("state changed after invalid-bound panic: got %#x, want %#x", got, before)
		}
	})
}

// TestXorShift64StarBoundedIndexUint64MatchesMultiplyHighMapping verifies the
// arbitrary-bound index path.
//
// The method should advance the source once and map the generated value into the
// interval [0, bound) using the high half of a 64x64 multiplication.
func TestXorShift64StarBoundedIndexUint64MatchesMultiplyHighMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// seed initializes both the expected-output source and the tested source.
		seed uint64

		// bound is the arbitrary non-zero index-space size.
		bound uint64
	}{
		{
			name:  "single slot",
			seed:  1,
			bound: 1,
		},
		{
			name:  "two slots",
			seed:  1,
			bound: 2,
		},
		{
			name:  "non power of two small bound",
			seed:  1,
			bound: 10,
		},
		{
			name:  "non power of two large bound",
			seed:  0x123456789abcdef0,
			bound: 1000,
		},
		{
			name:  "large arbitrary bound",
			seed:  maxUint64,
			bound: 1<<32 + 15,
		},
		{
			name:  "max uint64 bound",
			seed:  0x123456789abcdef0,
			bound: maxUint64,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			expectedSource := NewXorShift64Star(tt.seed)
			testedSource := NewXorShift64Star(tt.seed)

			output := expectedSource.NextUint64()
			want, _ := bits.Mul64(output, tt.bound)

			got := testedSource.BoundedIndexUint64(tt.bound)

			if got != want {
				t.Fatalf("BoundedIndexUint64(%d) = %d, want %d", tt.bound, got, want)
			}

			if got >= tt.bound {
				t.Fatalf("BoundedIndexUint64(%d) = %d, want value in [0, %d)", tt.bound, got, tt.bound)
			}

			if testedSource.State() != expectedSource.State() {
				t.Fatalf("state after BoundedIndexUint64 = %#x, want %#x", testedSource.State(), expectedSource.State())
			}
		})
	}
}

// TestXorShift64StarBoundedIndexUint64PanicsForZeroBound verifies that selecting
// from an empty index space is rejected.
//
// There is no valid index in [0, 0), so zero bound is always a caller bug.
func TestXorShift64StarBoundedIndexUint64PanicsForZeroBound(t *testing.T) {
	t.Parallel()

	source := NewXorShift64Star(1)
	before := source.State()

	testutil.MustPanicWithMessage(t, errXorShift64StarBoundedIndexZeroBound, func() {
		_ = source.BoundedIndexUint64(0)
	})

	if got := source.State(); got != before {
		t.Fatalf("state changed after zero-bound panic: got %#x, want %#x", got, before)
	}
}

// TestXorShift64StarIndexHelpersCoverRepresentativeSequentialOutputs is a smoke
// test for index coverage across generated values.
//
// This is not a statistical randomness test. It catches severe implementation
// errors where index helpers collapse generated values into a small subset of
// available indexes.
func TestXorShift64StarIndexHelpersCoverRepresentativeSequentialOutputs(t *testing.T) {
	t.Parallel()

	const samples = 4096

	t.Run("power of two bound covers all slots", func(t *testing.T) {
		t.Parallel()

		const bound = 16

		source := NewXorShift64Star(1)
		seen := make([]bool, bound)

		for i := 0; i < samples; i++ {
			index := source.PowerOfTwoIndexUint64(bound)
			seen[index] = true
		}

		for index, ok := range seen {
			if !ok {
				t.Fatalf("PowerOfTwoIndexUint64 did not produce index %d for %d samples", index, samples)
			}
		}
	})

	t.Run("arbitrary bound covers all slots", func(t *testing.T) {
		t.Parallel()

		const bound = 10

		source := NewXorShift64Star(1)
		seen := make([]bool, bound)

		for i := 0; i < samples; i++ {
			index := source.BoundedIndexUint64(bound)
			seen[index] = true
		}

		for index, ok := range seen {
			if !ok {
				t.Fatalf("BoundedIndexUint64 did not produce index %d for %d samples", index, samples)
			}
		}
	})
}
