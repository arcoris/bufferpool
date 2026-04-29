package budget

import "arcoris.dev/bufferpool/internal/control/numeric"

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
