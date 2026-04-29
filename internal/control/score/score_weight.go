package score

import "arcoris.dev/bufferpool/internal/control/numeric"

// usableScoreWeight normalizes optional score weights.
//
// Score weights are configuration inputs for control-plane heuristics. Invalid
// weights must not invert a score or propagate NaN into controller projections,
// so negative and non-finite values are treated as disabled weights.
func usableScoreWeight(weight float64) float64 {
	weight = numeric.FiniteOrZero(weight)
	if weight < 0 {
		return 0
	}
	return weight
}
