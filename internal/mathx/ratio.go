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

import "math"

const (
	// errRatioZeroDenominator is used when a strict ratio is requested with a
	// zero denominator.
	//
	// Strict ratio calculations are used where denominator == 0 represents an
	// invalid internal state rather than an ordinary "no samples yet" condition.
	// Callers that intentionally treat zero observations as ratio 0 SHOULD use
	// RatioOrZero instead.
	errRatioZeroDenominator = "mathx.Ratio: denominator must not be zero"

	// errRatioNaNNumerator is used when the numerator is NaN.
	//
	// NaN is rejected at ratio construction time because ratios feed adaptive
	// control-plane calculations such as EWMA scores, efficiency factors,
	// pressure multipliers, and budget shares. Allowing NaN to propagate would
	// make later budget or retention decisions non-deterministic.
	errRatioNaNNumerator = "mathx.Ratio: numerator must not be NaN"

	// errRatioNaNDenominator is used when the denominator is NaN.
	//
	// A NaN denominator usually indicates broken score, decay, or normalization
	// math before the ratio calculation. This must be detected near the source
	// of the invalid floating-point value.
	errRatioNaNDenominator = "mathx.Ratio: denominator must not be NaN"

	// errRatioInfiniteNumerator is used when the numerator is +Inf or -Inf.
	//
	// Infinite ratio inputs are not meaningful for bufferpool runtime control
	// because budgets, scores, pressure factors, and retention targets must
	// remain finite and explainable.
	errRatioInfiniteNumerator = "mathx.Ratio: numerator must be finite"

	// errRatioInfiniteDenominator is used when the denominator is +Inf or -Inf.
	//
	// Infinite denominator values are rejected for the same reason as infinite
	// numerators: ratio-producing code must produce finite control-plane inputs.
	errRatioInfiniteDenominator = "mathx.Ratio: denominator must be finite"

	// errRatioNaNResult is used when division produces NaN.
	//
	// This should normally be unreachable after numerator and denominator have
	// been validated, but it is kept as an explicit guard so Ratio remains the
	// single place that guarantees finite ratio results.
	errRatioNaNResult = "mathx.Ratio: result must not be NaN"

	// errRatioInfiniteResult is used when division produces +Inf or -Inf.
	//
	// This protects adaptive calculations from silently accepting values that
	// cannot be converted into bounded runtime targets.
	errRatioInfiniteResult = "mathx.Ratio: result must be finite"

	// errUnitRatioNegativeNumerator is used when a unit-ratio numerator is
	// negative.
	//
	// Unit ratios model values such as hit ratio, miss ratio, waste ratio,
	// retention efficiency, and normalized score factors. Negative values are
	// invalid for those domains and must be rejected instead of clamped.
	errUnitRatioNegativeNumerator = "mathx.UnitRatio: numerator must be greater than or equal to zero"

	// errUnitRatioNonPositiveDenominator is used when a strict unit ratio is
	// requested with denominator <= 0.
	//
	// UnitRatio is strict. Use UnitRatioOrZero when denominator == 0 represents
	// an ordinary no-observation window.
	errUnitRatioNonPositiveDenominator = "mathx.UnitRatio: denominator must be greater than zero"

	// errUnitRatioNumeratorExceedsDenominator is used when numerator >
	// denominator for a strict unit ratio.
	//
	// UnitRatio represents values constrained to [0, 1]. If the numerator is
	// greater than the denominator, the caller has either passed the wrong
	// counters or violated the invariant of the measured quantity.
	errUnitRatioNumeratorExceedsDenominator = "mathx.UnitRatio: numerator must be less than or equal to denominator"
)

// Ratio returns numerator / denominator as a finite float64 value.
//
// Ratio is the strict ratio helper for bufferpool internals. It is intended for
// calculations where denominator == 0 is an invalid state and should fail fast.
//
// Typical bufferpool use cases include:
//
//   - growth ratio calculations where the origin class size is known to be
//     non-zero;
//   - proportional budget calculations after the caller has already handled the
//     zero-total-score fallback;
//   - internal score factors where a missing denominator indicates broken
//     upstream math rather than a valid empty observation window.
//
// The function accepts Number values so callers can use raw numeric types or
// explicit domain-oriented numeric types such as RetainedBytes, BufferCount,
// Score, or BudgetWeight.
//
// Integer and unsigned inputs are converted to float64 because ratios are
// analytical control-plane values, not exact memory-accounting values. Exact
// byte accounting MUST remain in integer types outside ratio calculations.
//
// Ratio rejects NaN and infinite floating-point inputs and also validates that
// the final result is finite. This makes ratio.go the source of floating-point
// safety for division-based control-plane calculations.
func Ratio[T Number](numerator, denominator T) float64 {
	if denominator == 0 {
		panic(errRatioZeroDenominator)
	}

	result := float64(numerator) / float64(denominator)
	return mustFiniteRatioResult(
		mustFiniteRatioOperand(float64(numerator), errRatioNaNNumerator, errRatioInfiniteNumerator),
		mustFiniteRatioOperand(float64(denominator), errRatioNaNDenominator, errRatioInfiniteDenominator),
		result,
	)
}

// RatioOrZero returns numerator / denominator as a finite float64 value, or 0
// when denominator is zero.
//
// RatioOrZero is intended for observation windows where "no denominator" means
// "no samples yet" rather than an internal error.
//
// Typical bufferpool use cases include:
//
//   - hit ratio when gets == 0;
//   - miss ratio when gets == 0;
//   - drop ratio when puts == 0;
//   - allocation ratio when requests == 0;
//   - efficiency metrics for an empty workload window.
//
// RatioOrZero still validates NaN and infinite inputs when a division is
// performed. If denominator is zero, numerator is not used for division and the
// result is defined as 0.
func RatioOrZero[T Number](numerator, denominator T) float64 {
	if denominator == 0 {
		return 0
	}

	return Ratio(numerator, denominator)
}

// UnitRatio returns numerator / denominator as a finite value constrained by
// invariant to the closed interval [0, 1].
//
// Unlike Clamp(Ratio(...), 0, 1), UnitRatio does not silently hide invalid
// inputs. It verifies the unit-ratio invariant before division:
//
//   - numerator >= 0;
//   - denominator > 0;
//   - numerator <= denominator.
//
// This is useful when the caller expects a mathematically valid unit ratio and
// wants broken counters or broken normalization to fail fast.
//
// Typical bufferpool use cases include:
//
//   - hit ratio when hits <= gets;
//   - miss ratio when misses <= gets;
//   - retained waste ratio when wasted bytes <= allocated class bytes;
//   - normalized retention efficiency;
//   - normalized coldness or activity factors after upstream validation.
//
// Use RatioOrZero or UnitRatioOrZero when denominator == 0 is a valid empty
// observation window.
func UnitRatio[T Number](numerator, denominator T) float64 {
	validateUnitRatioOperands(numerator, denominator)
	return Ratio(numerator, denominator)
}

// UnitRatioOrZero returns UnitRatio(numerator, denominator), or 0 when
// denominator is zero.
//
// This helper is intended for unit ratios derived from sampled counters where an
// empty observation window is valid and should produce a neutral zero ratio.
//
// The numerator MUST still be zero when denominator is zero. A positive
// numerator with a zero denominator violates the unit-ratio invariant and causes
// a panic. This catches broken counter aggregation instead of silently erasing it
// as ratio 0.
func UnitRatioOrZero[T Number](numerator, denominator T) float64 {
	if denominator == 0 {
		if numerator != 0 {
			panic(errUnitRatioNumeratorExceedsDenominator)
		}

		return 0
	}

	return UnitRatio(numerator, denominator)
}

// validateUnitRatioOperands verifies the integer/float ordering invariants for
// ratios that must represent a value in [0, 1].
//
// The function is generic so the invariant can be checked before converting
// operands to float64. This avoids precision surprises for large integer inputs
// where converting to float64 first could make two distinct integer values look
// equal.
func validateUnitRatioOperands[T Number](numerator, denominator T) {
	if numerator < 0 {
		panic(errUnitRatioNegativeNumerator)
	}

	if denominator <= 0 {
		panic(errUnitRatioNonPositiveDenominator)
	}

	if numerator > denominator {
		panic(errUnitRatioNumeratorExceedsDenominator)
	}
}

// mustFiniteRatioOperand validates a floating-point ratio operand.
//
// Integer and unsigned inputs become finite float64 values after conversion.
// Float inputs may carry NaN or infinity, so every operand is checked after
// conversion. The function returns value to keep the caller expression compact
// while still making validation explicit.
func mustFiniteRatioOperand(value float64, nanMessage, infiniteMessage string) float64 {
	if math.IsNaN(value) {
		panic(nanMessage)
	}

	if math.IsInf(value, 0) {
		panic(infiniteMessage)
	}

	return value
}

// mustFiniteRatioResult validates the final ratio result.
//
// The numerator and denominator arguments are intentionally accepted even though
// only result is checked. This keeps call sites explicit about the validation
// sequence and prevents future refactors from accidentally removing operand
// validation as "unused" logic.
func mustFiniteRatioResult(numerator, denominator, result float64) float64 {
	_ = numerator
	_ = denominator

	if math.IsNaN(result) {
		panic(errRatioNaNResult)
	}

	if math.IsInf(result, 0) {
		panic(errRatioInfiniteResult)
	}

	return result
}
