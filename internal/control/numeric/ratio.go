package numeric

import (
	"math"

	"arcoris.dev/bufferpool/internal/mathx"
)

// SaturatingAddUint64 returns left plus right, capped at math.MaxUint64.
//
// Control windows often add independent monotonic counters to form a
// denominator, for example hits+misses or successes+failures. A wrapped
// denominator would make a saturated workload look empty or tiny, so overflow is
// represented as the largest possible denominator instead of wrapping to zero.
// Exact byte accounting should still stay in integer domain-specific code; this
// helper is only for safe aggregate control arithmetic.
func SaturatingAddUint64(left, right uint64) uint64 {
	if left > math.MaxUint64-right {
		return math.MaxUint64
	}
	return left + right
}

// SaturatingSumUint64 returns the sum of values, capped at math.MaxUint64.
//
// Empty input returns zero. The helper applies the same control-plane overflow
// contract as SaturatingAddUint64 across more than two counters: exact sums are
// preserved when representable, and overflow saturates instead of wrapping a
// denominator or throughput source back toward zero.
func SaturatingSumUint64(values ...uint64) uint64 {
	var sum uint64
	for _, value := range values {
		sum = SaturatingAddUint64(sum, value)
		if sum == math.MaxUint64 {
			return math.MaxUint64
		}
	}
	return sum
}

// SafeRatio divides numerator by denominator and returns zero for denominator
// zero or non-finite results.
func SafeRatio(numerator, denominator uint64) float64 {
	return FiniteOrZero(mathx.RatioOrZero(numerator, denominator))
}

// SafeIntRatio divides numerator by denominator and returns zero for
// denominator zero or non-finite results.
func SafeIntRatio(numerator, denominator int64) float64 {
	return FiniteOrZero(mathx.RatioOrZero(numerator, denominator))
}

// SafeFloatRatio divides numerator by denominator and returns zero for
// denominator zero or non-finite inputs or results.
func SafeFloatRatio(numerator, denominator float64) float64 {
	if denominator == 0 || !IsFinite(numerator) || !IsFinite(denominator) {
		return 0
	}
	return FiniteOrZero(mathx.RatioOrZero(numerator, denominator))
}
