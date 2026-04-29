package numeric

import "arcoris.dev/bufferpool/internal/mathx"

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
