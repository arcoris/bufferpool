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
	"math"
	"testing"

	"arcoris.dev/bufferpool/internal/testutil"
)

// ratioTestBytes represents a domain-specific byte counter used only by ratio
// tests.
//
// The type verifies that Ratio helpers accept project-defined unsigned numeric
// types through the Number constraint. This matters because bufferpool runtime
// code should be able to keep byte-oriented domain types instead of converting
// everything to raw uint64 before calling math helpers.
type ratioTestBytes uint64

// ratioTestCount represents a domain-specific observation counter.
//
// Counters such as gets, hits, misses, drops, allocations, retained buffers, and
// trimmed buffers are naturally unsigned values and frequently participate in
// ratio calculations.
type ratioTestCount uint64

// ratioTestDelta represents a signed runtime delta.
//
// Signed ratio inputs are uncommon for unit ratios, but strict Ratio may be used
// for internal analytical values where signed deltas or correction factors are
// meaningful.
type ratioTestDelta int64

// ratioTestScore represents a domain-specific floating-point score.
//
// The type verifies that Ratio helpers accept project-defined float values
// while still applying NaN and infinity validation at the ratio-construction
// boundary.
type ratioTestScore float64

// TestRatio verifies strict ratio calculation for valid finite operands.
//
// Ratio is strict: denominator == 0 is not treated as an empty observation
// window. It is an internal invariant violation. This test covers ordinary
// signed, unsigned, and floating-point cases that should produce finite float64
// results.
func TestRatio(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// numerator is the value above the division bar.
		numerator float64

		// denominator is the value below the division bar and must be non-zero.
		denominator float64

		// want is the expected finite ratio result.
		want float64
	}{
		{
			name:        "positive integer-like values",
			numerator:   10,
			denominator: 20,
			want:        0.5,
		},
		{
			name:        "negative numerator is supported for strict ratios",
			numerator:   -6,
			denominator: 3,
			want:        -2,
		},
		{
			name:        "negative denominator is supported for strict ratios",
			numerator:   6,
			denominator: -3,
			want:        -2,
		},
		{
			name:        "fractional result",
			numerator:   3,
			denominator: 2,
			want:        1.5,
		},
		{
			name:        "zero numerator returns zero",
			numerator:   0,
			denominator: 128,
			want:        0,
		},
		{
			name:        "floating point operands",
			numerator:   0.25,
			denominator: 0.5,
			want:        0.5,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Ratio(tt.numerator, tt.denominator)
			if got != tt.want {
				t.Fatalf("Ratio(%f, %f) = %f, want %f",
					tt.numerator,
					tt.denominator,
					got,
					tt.want,
				)
			}
		})
	}
}

// TestRatioPreservesNamedNumericTypes verifies that Ratio accepts explicit
// domain-oriented numeric types.
//
// This protects the design choice behind Number constraints: bufferpool code can
// introduce meaningful domain types for bytes, counters, deltas, and scores
// without losing access to low-level math helpers.
func TestRatioPreservesNamedNumericTypes(t *testing.T) {
	t.Parallel()

	t.Run("byte domain type", func(t *testing.T) {
		t.Parallel()

		got := Ratio(ratioTestBytes(512), ratioTestBytes(1024))
		if got != 0.5 {
			t.Fatalf("Ratio returned %f, want %f", got, 0.5)
		}
	})

	t.Run("counter domain type", func(t *testing.T) {
		t.Parallel()

		got := Ratio(ratioTestCount(75), ratioTestCount(100))
		if got != 0.75 {
			t.Fatalf("Ratio returned %f, want %f", got, 0.75)
		}
	})

	t.Run("signed delta domain type", func(t *testing.T) {
		t.Parallel()

		got := Ratio(ratioTestDelta(-50), ratioTestDelta(100))
		if got != -0.5 {
			t.Fatalf("Ratio returned %f, want %f", got, -0.5)
		}
	})

	t.Run("score domain type", func(t *testing.T) {
		t.Parallel()

		got := Ratio(ratioTestScore(0.25), ratioTestScore(0.5))
		if got != 0.5 {
			t.Fatalf("Ratio returned %f, want %f", got, 0.5)
		}
	})
}

// TestRatioPanicsForZeroDenominator verifies strict denominator handling.
//
// Ratio is used when denominator == 0 represents broken internal math. Callers
// that intentionally model empty observation windows must use RatioOrZero or
// UnitRatioOrZero instead.
func TestRatioPanicsForZeroDenominator(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errRatioZeroDenominator, func() {
		_ = Ratio(10, 0)
	})
}

// TestRatioPanicsForNaNInputs verifies that NaN operands are rejected at the
// ratio-construction boundary.
//
// This is the correct layer for NaN validation because division-based values
// feed workload scoring, retention efficiency, pressure multipliers, and budget
// allocation. NaN must not be allowed to leak into those systems.
func TestRatioPanicsForNaNInputs(t *testing.T) {
	t.Parallel()

	t.Run("NaN numerator", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errRatioNaNNumerator, func() {
			_ = Ratio(math.NaN(), 1.0)
		})
	})

	t.Run("NaN denominator", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errRatioNaNDenominator, func() {
			_ = Ratio(1.0, math.NaN())
		})
	})
}

// TestRatioPanicsForInfiniteInputs verifies that infinite operands are rejected.
//
// Infinite values are not useful control-plane inputs. A budget share, pressure
// factor, score, or efficiency value must remain finite and explainable.
func TestRatioPanicsForInfiniteInputs(t *testing.T) {
	t.Parallel()

	t.Run("positive infinite numerator", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errRatioInfiniteNumerator, func() {
			_ = Ratio(math.Inf(1), 1.0)
		})
	})

	t.Run("negative infinite numerator", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errRatioInfiniteNumerator, func() {
			_ = Ratio(math.Inf(-1), 1.0)
		})
	})

	t.Run("positive infinite denominator", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errRatioInfiniteDenominator, func() {
			_ = Ratio(1.0, math.Inf(1))
		})
	})

	t.Run("negative infinite denominator", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errRatioInfiniteDenominator, func() {
			_ = Ratio(1.0, math.Inf(-1))
		})
	})
}

// TestRatioPanicsForInfiniteResult verifies that finite operands producing an
// infinite result are rejected.
//
// This can happen with extreme finite float64 values. The guard prevents an
// overflowed analytical value from becoming a budget factor or retention score.
func TestRatioPanicsForInfiniteResult(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errRatioInfiniteResult, func() {
		_ = Ratio(math.MaxFloat64, math.SmallestNonzeroFloat64)
	})
}

// TestRatioOrZero verifies ratio calculation for observation windows where a
// zero denominator means "no samples yet".
//
// RatioOrZero is intentionally different from Ratio. It is suitable for metrics
// such as hits/gets or drops/puts where an empty window should produce a neutral
// zero value instead of panicking.
func TestRatioOrZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// numerator is the observed numerator value.
		numerator uint64

		// denominator is the observed denominator value. Zero represents an empty
		// observation window for this helper.
		denominator uint64

		// want is the expected ratio or zero fallback.
		want float64
	}{
		{
			name:        "normal ratio",
			numerator:   25,
			denominator: 100,
			want:        0.25,
		},
		{
			name:        "zero denominator returns zero",
			numerator:   25,
			denominator: 0,
			want:        0,
		},
		{
			name:        "zero numerator and zero denominator returns zero",
			numerator:   0,
			denominator: 0,
			want:        0,
		},
		{
			name:        "zero numerator and non-zero denominator returns zero",
			numerator:   0,
			denominator: 100,
			want:        0,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := RatioOrZero(tt.numerator, tt.denominator)
			if got != tt.want {
				t.Fatalf("RatioOrZero(%d, %d) = %f, want %f",
					tt.numerator,
					tt.denominator,
					got,
					tt.want,
				)
			}
		})
	}
}

// TestRatioOrZeroStillValidatesWhenDividing verifies that RatioOrZero is lenient
// only for denominator == 0.
//
// When division is actually performed, RatioOrZero must preserve the same
// floating-point safety guarantees as Ratio.
func TestRatioOrZeroStillValidatesWhenDividing(t *testing.T) {
	t.Parallel()

	t.Run("NaN numerator with non-zero denominator panics", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errRatioNaNNumerator, func() {
			_ = RatioOrZero(math.NaN(), 1.0)
		})
	})

	t.Run("infinite denominator with non-zero denominator panics", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errRatioInfiniteDenominator, func() {
			_ = RatioOrZero(1.0, math.Inf(1))
		})
	})
}

// TestUnitRatio verifies valid unit ratios.
//
// UnitRatio is stricter than Clamp(Ratio(...), 0, 1). It verifies that the
// caller's counters or normalized values already satisfy the unit-ratio
// invariant before division.
func TestUnitRatio(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// numerator must be non-negative and less than or equal to denominator.
		numerator uint64

		// denominator must be greater than zero.
		denominator uint64

		// want is the exact expected unit-ratio result.
		want float64
	}{
		{
			name:        "zero numerator",
			numerator:   0,
			denominator: 100,
			want:        0,
		},
		{
			name:        "partial ratio",
			numerator:   25,
			denominator: 100,
			want:        0.25,
		},
		{
			name:        "half ratio",
			numerator:   50,
			denominator: 100,
			want:        0.5,
		},
		{
			name:        "full ratio",
			numerator:   100,
			denominator: 100,
			want:        1,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := UnitRatio(tt.numerator, tt.denominator)
			if got != tt.want {
				t.Fatalf("UnitRatio(%d, %d) = %f, want %f",
					tt.numerator,
					tt.denominator,
					got,
					tt.want,
				)
			}
		})
	}
}

// TestUnitRatioFloatInputs verifies unit-ratio behavior for floating-point
// values.
//
// Floating-point unit ratios are useful for already-normalized score factors,
// pressure factors, or efficiency values where the producer has supplied finite
// values satisfying 0 <= numerator <= denominator.
func TestUnitRatioFloatInputs(t *testing.T) {
	t.Parallel()

	got := UnitRatio(0.25, 0.5)
	if got != 0.5 {
		t.Fatalf("UnitRatio returned %f, want %f", got, 0.5)
	}
}

// TestUnitRatioPreservesLargeIntegerInvariant checks unit-ratio invariants
// before float64 conversion becomes relevant.
//
// Large uint64 values may lose precision when converted to float64. UnitRatio
// must validate numerator <= denominator on the original integer type before
// division, so the invariant is not weakened by floating-point rounding.
func TestUnitRatioPreservesLargeIntegerInvariant(t *testing.T) {
	t.Parallel()

	maxUint64 := ^uint64(0)

	got := UnitRatio(maxUint64-1, maxUint64)

	if math.IsNaN(got) {
		t.Fatal("UnitRatio returned NaN, want finite value")
	}

	if math.IsInf(got, 0) {
		t.Fatal("UnitRatio returned infinity, want finite value")
	}

	if got < 0 || got > 1 {
		t.Fatalf("UnitRatio returned %f, want value in [0, 1]", got)
	}
}

// TestUnitRatioPanicsForInvalidInvariants verifies that UnitRatio rejects broken
// unit-ratio inputs instead of clamping them.
//
// These panics catch invalid counter aggregation, impossible metric values, and
// broken normalization before those values can affect adaptive scoring or budget
// allocation.
func TestUnitRatioPanicsForInvalidInvariants(t *testing.T) {
	t.Parallel()

	t.Run("negative numerator", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errUnitRatioNegativeNumerator, func() {
			_ = UnitRatio(-1, 10)
		})
	})

	t.Run("zero denominator", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errUnitRatioNonPositiveDenominator, func() {
			_ = UnitRatio(0, 0)
		})
	})

	t.Run("negative denominator", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errUnitRatioNonPositiveDenominator, func() {
			_ = UnitRatio(0, -1)
		})
	})

	t.Run("numerator greater than denominator", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errUnitRatioNumeratorExceedsDenominator, func() {
			_ = UnitRatio(11, 10)
		})
	})
}

// TestUnitRatioPanicsForNaNInputs verifies that UnitRatio eventually delegates
// floating-point validity to Ratio.
//
// UnitRatio owns the unit-ratio ordering invariant. Ratio owns NaN/Inf
// validation. Together they ensure unit ratios are both semantically valid and
// finite.
func TestUnitRatioPanicsForNaNInputs(t *testing.T) {
	t.Parallel()

	t.Run("NaN numerator", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errRatioNaNNumerator, func() {
			_ = UnitRatio(math.NaN(), 1.0)
		})
	})

	t.Run("NaN denominator", func(t *testing.T) {
		t.Parallel()

		testutil.MustPanicWithMessage(t, errRatioNaNDenominator, func() {
			_ = UnitRatio(0.5, math.NaN())
		})
	})
}

// TestUnitRatioOrZero verifies the empty-window variant for unit ratios.
//
// UnitRatioOrZero is intended for sampled counters where denominator == 0 means
// no observations were collected during the window. In that case, only a zero
// numerator is valid.
func TestUnitRatioOrZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// numerator must be zero when denominator is zero.
		numerator uint64

		// denominator may be zero for an empty observation window.
		denominator uint64

		// want is the expected unit ratio or zero fallback.
		want float64
	}{
		{
			name:        "empty window returns zero",
			numerator:   0,
			denominator: 0,
			want:        0,
		},
		{
			name:        "normal partial unit ratio",
			numerator:   25,
			denominator: 100,
			want:        0.25,
		},
		{
			name:        "normal full unit ratio",
			numerator:   100,
			denominator: 100,
			want:        1,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := UnitRatioOrZero(tt.numerator, tt.denominator)
			if got != tt.want {
				t.Fatalf("UnitRatioOrZero(%d, %d) = %f, want %f",
					tt.numerator,
					tt.denominator,
					got,
					tt.want,
				)
			}
		})
	}
}

// TestUnitRatioOrZeroPanicsForNonZeroNumeratorInEmptyWindow verifies that broken
// counter aggregation is not silently erased.
//
// If denominator == 0 but numerator > 0, the caller has produced an impossible
// unit-ratio state. Returning 0 would hide a real accounting or sampling bug.
func TestUnitRatioOrZeroPanicsForNonZeroNumeratorInEmptyWindow(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errUnitRatioNumeratorExceedsDenominator, func() {
		_ = UnitRatioOrZero(1, 0)
	})
}
