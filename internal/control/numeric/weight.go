package numeric

// WeightedValue is one normalized value and its non-negative contribution weight.
type WeightedValue struct {
	// Value is clamped to [0, 1] when averaging.
	Value float64

	// Weight contributes to the total weight. Negative or non-finite weights
	// are treated as zero.
	Weight float64
}

// WeightedAverage returns a [0, 1] weighted average.
//
// Component values are clamped to [0, 1]. Negative and non-finite weights are
// ignored. If the total usable weight is zero, WeightedAverage returns zero.
func WeightedAverage(values []WeightedValue) float64 {
	var weightedSum float64
	var weightSum float64
	for _, value := range values {
		weight := FiniteOrZero(value.Weight)
		if weight <= 0 {
			continue
		}
		weightedSum += Clamp01(value.Value) * weight
		weightSum += weight
	}
	if weightSum == 0 {
		return 0
	}
	return Clamp01(weightedSum / weightSum)
}

// NormalizeWeights returns a new slice whose non-negative finite weights sum to one.
//
// Negative and non-finite input weights are treated as zero. If the usable total
// is zero, the returned slice contains zeros and has the same length as weights.
func NormalizeWeights(weights []float64) []float64 {
	return NormalizeWeightsInto(nil, weights)
}

// NormalizeWeightsInto writes normalized weights into dst and returns the
// resulting slice.
//
// The returned slice has len(weights). Existing dst capacity is reused when it
// is large enough, and weights is never mutated. Negative and non-finite input
// weights become zero. If the usable total is zero, the returned slice is
// zero-filled so callers can reuse scratch storage without retaining stale
// normalized values from an earlier window.
func NormalizeWeightsInto(dst, weights []float64) []float64 {
	normalized := dst
	if cap(normalized) < len(weights) {
		normalized = make([]float64, len(weights))
	} else {
		normalized = normalized[:len(weights)]
		clear(normalized)
	}

	var total float64
	for index, weight := range weights {
		weight = FiniteOrZero(weight)
		if weight <= 0 {
			continue
		}
		normalized[index] = weight
		total += weight
	}
	if total == 0 {
		return normalized
	}
	for index := range normalized {
		normalized[index] /= total
	}
	return normalized
}
