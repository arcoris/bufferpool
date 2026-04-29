package budget

import (
	"math/bits"

	"arcoris.dev/bufferpool/internal/control/numeric"
)

// AllocationInput is a generic budget target input.
type AllocationInput struct {
	// Score is the normalized demand or usefulness score.
	Score float64

	// Base is caller-defined fixed allocation.
	Base uint64

	// Min is the lower bound when non-zero.
	Min uint64

	// Max is the upper bound when non-zero.
	Max uint64
}

// AllocationResult is a generic budget allocation output.
type AllocationResult struct {
	// Target is the clamped target budget.
	Target uint64
}

// ProportionalShare returns totalAdaptive multiplied by score/scoreSum.
//
// This helper accepts floating-point scores because score composition is
// analytical and normalized. Very large totals can lose low-bit precision during
// float64 conversion; exact byte-budget redistribution should prefer
// ProportionalShareByWeight with integer weights.
func ProportionalShare(totalAdaptive uint64, score, scoreSum float64) uint64 {
	if score <= 0 || scoreSum <= 0 {
		return 0
	}
	ratio := numeric.SafeFloatRatio(score, scoreSum)
	if ratio <= 0 {
		return 0
	}
	return uint64(float64(totalAdaptive) * ratio)
}

// ProportionalShareByWeight returns floor(total*weight/weightSum).
//
// The calculation uses 128-bit intermediate arithmetic through math/bits, so
// total*weight cannot overflow uint64. If weight is greater than or equal to
// weightSum, the share is capped at total; callers should normally pass
// weightSum as the sum of all candidate weights. A zero weight or weightSum
// returns zero.
func ProportionalShareByWeight(total, weight, weightSum uint64) uint64 {
	if total == 0 || weight == 0 || weightSum == 0 {
		return 0
	}
	if weight >= weightSum {
		return total
	}
	hi, lo := bits.Mul64(total, weight)
	share, _ := bits.Div64(hi, lo, weightSum)
	return share
}

// ClampTarget clamps value to non-zero min and max bounds.
func ClampTarget(value, minValue, maxValue uint64) uint64 {
	if minValue > maxValue && maxValue != 0 {
		minValue, maxValue = maxValue, minValue
	}
	if minValue != 0 && value < minValue {
		return minValue
	}
	if maxValue != 0 && value > maxValue {
		return maxValue
	}
	return value
}
