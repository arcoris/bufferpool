package numeric

import "math"

// IsFinite reports whether value is neither NaN nor positive or negative infinity.
func IsFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

// FiniteOrZero returns zero for NaN or infinity.
func FiniteOrZero(value float64) float64 {
	if !IsFinite(value) {
		return 0
	}
	return value
}
