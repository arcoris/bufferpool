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
	"time"

	"arcoris.dev/bufferpool/internal/testutil"
)

// testRetainedBytes represents a domain-specific byte counter.
//
// The test type verifies that Clamp accepts project-defined numeric types whose
// underlying type is uint64. This matters for bufferpool internals because byte
// accounting should eventually be expressed through explicit domain-oriented
// types instead of raw primitive values everywhere.
type testRetainedBytes uint64

// testShardCount represents a domain-specific shard count.
//
// The test type verifies that Clamp works with small unsigned integer domain
// types. Shard counts are discrete runtime values and must not be routed through
// floating-point arithmetic.
type testShardCount uint16

// testBudgetDelta represents a signed budget adjustment.
//
// The test type verifies that Clamp supports signed domain values. Signed values
// are useful for deltas, corrections, and budget movement calculations where a
// value may be temporarily negative before it is bounded.
type testBudgetDelta int64

// testScore represents a floating-point adaptive score.
//
// The test type verifies that Clamp accepts project-defined float types while
// still preserving the architectural rule that floating-point validity is owned
// by score, ratio, and budget-producing code.
type testScore float64

// TestClampSignedIntegers verifies Clamp behavior for signed integer values.
//
// Signed integers are used for internal deltas, indexes, correction values, and
// other calculations where the value may legitimately fall below zero before it
// is bounded into an allowed runtime range.
func TestClampSignedIntegers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// value is the candidate runtime value that must be constrained.
		value int

		// minValue is the inclusive lower bound.
		minValue int

		// maxValue is the inclusive upper bound.
		maxValue int

		// want is the expected bounded result.
		want int
	}{
		{
			name:     "below range returns minimum",
			value:    -10,
			minValue: 0,
			maxValue: 32,
			want:     0,
		},
		{
			name:     "inside range returns value",
			value:    16,
			minValue: 0,
			maxValue: 32,
			want:     16,
		},
		{
			name:     "above range returns maximum",
			value:    64,
			minValue: 0,
			maxValue: 32,
			want:     32,
		},
		{
			name:     "minimum boundary is inclusive",
			value:    0,
			minValue: 0,
			maxValue: 32,
			want:     0,
		},
		{
			name:     "maximum boundary is inclusive",
			value:    32,
			minValue: 0,
			maxValue: 32,
			want:     32,
		},
		{
			name:     "single-value range returns the only allowed value",
			value:    10,
			minValue: 8,
			maxValue: 8,
			want:     8,
		},
		{
			name:     "negative range clamps to negative minimum",
			value:    -50,
			minValue: -32,
			maxValue: -1,
			want:     -32,
		},
		{
			name:     "negative value inside negative range returns value",
			value:    -16,
			minValue: -32,
			maxValue: -1,
			want:     -16,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Clamp(tt.value, tt.minValue, tt.maxValue)
			if got != tt.want {
				t.Fatalf("Clamp(%d, %d, %d) = %d, want %d",
					tt.value,
					tt.minValue,
					tt.maxValue,
					got,
					tt.want,
				)
			}
		})
	}
}

// TestClampUnsignedIntegers verifies Clamp behavior for unsigned integer values.
//
// Unsigned integers are the most important numeric category for bufferpool's
// physical runtime model: byte capacities, retained bytes, buffer counts, class
// sizes, shard indexes, segment slot counts, and trim limits are all naturally
// non-negative.
func TestClampUnsignedIntegers(t *testing.T) {
	t.Parallel()

	maxUint64 := ^uint64(0)

	tests := []struct {
		name string

		// value is the candidate non-negative runtime value that must be bounded.
		value uint64

		// minValue is the inclusive lower bound.
		minValue uint64

		// maxValue is the inclusive upper bound.
		maxValue uint64

		// want is the expected bounded result.
		want uint64
	}{
		{
			name:     "below range returns minimum",
			value:    4,
			minValue: 8,
			maxValue: 64,
			want:     8,
		},
		{
			name:     "inside range returns value",
			value:    32,
			minValue: 8,
			maxValue: 64,
			want:     32,
		},
		{
			name:     "above range returns maximum",
			value:    128,
			minValue: 8,
			maxValue: 64,
			want:     64,
		},
		{
			name:     "zero minimum is supported",
			value:    0,
			minValue: 0,
			maxValue: 64,
			want:     0,
		},
		{
			name:     "single-value range returns the only allowed value",
			value:    16,
			minValue: 32,
			maxValue: 32,
			want:     32,
		},
		{
			name:     "large uint64 value remains exact",
			value:    maxUint64,
			minValue: 0,
			maxValue: maxUint64 - 1,
			want:     maxUint64 - 1,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Clamp(tt.value, tt.minValue, tt.maxValue)
			if got != tt.want {
				t.Fatalf("Clamp(%d, %d, %d) = %d, want %d",
					tt.value,
					tt.minValue,
					tt.maxValue,
					got,
					tt.want,
				)
			}
		})
	}
}

// TestClampFloats verifies Clamp behavior for floating-point values.
//
// Floating-point values belong to the analytical control plane: ratios, EWMA
// factors, scores, pressure multipliers, and efficiency values. Clamp may bound
// those values after the producer has already validated that they are meaningful
// finite numbers.
func TestClampFloats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// value is the candidate floating-point value that must be bounded.
		value float64

		// minValue is the inclusive lower bound.
		minValue float64

		// maxValue is the inclusive upper bound.
		maxValue float64

		// want is the expected bounded result.
		want float64
	}{
		{
			name:     "below range returns minimum",
			value:    -0.25,
			minValue: 0.0,
			maxValue: 1.0,
			want:     0.0,
		},
		{
			name:     "inside range returns value",
			value:    0.75,
			minValue: 0.0,
			maxValue: 1.0,
			want:     0.75,
		},
		{
			name:     "above range returns maximum",
			value:    1.25,
			minValue: 0.0,
			maxValue: 1.0,
			want:     1.0,
		},
		{
			name:     "minimum boundary is inclusive",
			value:    0.0,
			minValue: 0.0,
			maxValue: 1.0,
			want:     0.0,
		},
		{
			name:     "maximum boundary is inclusive",
			value:    1.0,
			minValue: 0.0,
			maxValue: 1.0,
			want:     1.0,
		},
		{
			name:     "single-value range returns the only allowed value",
			value:    0.5,
			minValue: 1.0,
			maxValue: 1.0,
			want:     1.0,
		},
		{
			name:     "negative float range clamps to negative minimum",
			value:    -2.5,
			minValue: -2.0,
			maxValue: -1.0,
			want:     -2.0,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Clamp(tt.value, tt.minValue, tt.maxValue)
			if got != tt.want {
				t.Fatalf("Clamp(%f, %f, %f) = %f, want %f",
					tt.value,
					tt.minValue,
					tt.maxValue,
					got,
					tt.want,
				)
			}
		})
	}
}

// TestClampPreservesNamedNumericTypes verifies that Clamp supports explicit
// domain types with numeric underlying types.
//
// This test is important because mathx.Number uses tilde constraints. Without
// tilde constraints, Clamp would accept raw uint64/int64/float64 values but
// would reject domain-oriented types such as RetainedBytes, ShardCount, or
// Score. That would force callers to strip domain types before using low-level
// math helpers, weakening type clarity across the runtime.
func TestClampPreservesNamedNumericTypes(t *testing.T) {
	t.Parallel()

	t.Run("retained bytes use uint64 underlying type", func(t *testing.T) {
		t.Parallel()

		got := Clamp(
			testRetainedBytes(2048),
			testRetainedBytes(0),
			testRetainedBytes(1024),
		)

		if got != testRetainedBytes(1024) {
			t.Fatalf("Clamp returned %d, want %d", got, testRetainedBytes(1024))
		}
	})

	t.Run("shard count uses uint16 underlying type", func(t *testing.T) {
		t.Parallel()

		got := Clamp(
			testShardCount(0),
			testShardCount(1),
			testShardCount(32),
		)

		if got != testShardCount(1) {
			t.Fatalf("Clamp returned %d, want %d", got, testShardCount(1))
		}
	})

	t.Run("budget delta uses int64 underlying type", func(t *testing.T) {
		t.Parallel()

		got := Clamp(
			testBudgetDelta(-128),
			testBudgetDelta(-64),
			testBudgetDelta(64),
		)

		if got != testBudgetDelta(-64) {
			t.Fatalf("Clamp returned %d, want %d", got, testBudgetDelta(-64))
		}
	})

	t.Run("score uses float64 underlying type", func(t *testing.T) {
		t.Parallel()

		got := Clamp(
			testScore(1.25),
			testScore(0.0),
			testScore(1.0),
		)

		if got != testScore(1.0) {
			t.Fatalf("Clamp returned %f, want %f", got, testScore(1.0))
		}
	})
}

// TestClampSupportsDuration verifies that Clamp supports time.Duration.
//
// time.Duration has int64 as its underlying type. Supporting it allows runtime
// code to bound controller intervals, idle timeouts, trim cadence values, and
// other duration-based settings without converting them to raw integers.
func TestClampSupportsDuration(t *testing.T) {
	t.Parallel()

	got := Clamp(
		150*time.Millisecond,
		10*time.Millisecond,
		100*time.Millisecond,
	)

	if got != 100*time.Millisecond {
		t.Fatalf("Clamp returned %s, want %s", got, 100*time.Millisecond)
	}
}

// TestClampPanicsForInvalidSignedRange verifies that invalid signed ranges are
// treated as internal invariant violations.
//
// Clamp is not expected to recover from minValue > maxValue. Such a range means
// a caller produced an impossible bound pair, usually due to broken validation,
// broken configuration normalization, or incorrect budget math.
func TestClampPanicsForInvalidSignedRange(t *testing.T) {
	t.Parallel()

	testutil.MustPanic(t, func() {
		_ = Clamp(10, 32, 1)
	})
}

// TestClampPanicsForInvalidUnsignedRange verifies that invalid unsigned ranges
// panic as well.
//
// This protects byte-count and capacity-bound calculations from silently
// accepting impossible ranges.
func TestClampPanicsForInvalidUnsignedRange(t *testing.T) {
	t.Parallel()

	testutil.MustPanic(t, func() {
		_ = Clamp(uint64(10), uint64(32), uint64(1))
	})
}

// TestClampPanicsForInvalidFloatRange verifies that invalid floating-point
// ranges panic.
//
// This test only covers invalid range ordering. It intentionally does not turn
// Clamp into a floating-point validator.
func TestClampPanicsForInvalidFloatRange(t *testing.T) {
	t.Parallel()

	testutil.MustPanic(t, func() {
		_ = Clamp(0.5, 1.0, 0.0)
	})
}

// TestClampDoesNotValidateNaN documents the intended responsibility boundary.
//
// Clamp performs range bounding using ordinary Go comparisons. It does not own
// floating-point validation. In Go, comparisons with NaN are false, so a NaN
// value passes through unchanged.
//
// This is intentional for the current design: ratio, score, decay, pressure,
// and budget-producing code must validate NaN/Inf at the source where the value
// is created. Clamp should not hide that responsibility behind a generic bound
// helper.
func TestClampDoesNotValidateNaN(t *testing.T) {
	t.Parallel()

	got := Clamp(math.NaN(), 0.0, 1.0)
	if !math.IsNaN(got) {
		t.Fatalf("Clamp returned %f, want NaN", got)
	}
}
