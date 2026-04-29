package numeric

// NormalizeToLimit returns current divided by limit, clamped to [0, 1].
// A zero limit returns zero.
func NormalizeToLimit(current, limit uint64) float64 {
	if limit == 0 {
		return 0
	}
	return Clamp01(SafeRatio(current, limit))
}

// NormalizeFloatToLimit returns current divided by limit, clamped to [0, 1].
// A non-positive or non-finite limit returns zero.
func NormalizeFloatToLimit(current, limit float64) float64 {
	if limit <= 0 || !IsFinite(limit) {
		return 0
	}
	return Clamp01(SafeFloatRatio(current, limit))
}

// NormalizeToRange maps value within [minValue, maxValue] to [0, 1].
// Invalid, inverted, or zero-width ranges return zero.
func NormalizeToRange(value, minValue, maxValue float64) float64 {
	value = FiniteOrZero(value)
	minValue = FiniteOrZero(minValue)
	maxValue = FiniteOrZero(maxValue)
	if maxValue <= minValue {
		return 0
	}
	return Clamp01((value - minValue) / (maxValue - minValue))
}
