package risk

import "arcoris.dev/bufferpool/internal/control/numeric"

// usableRiskWeight normalizes optional risk weights.
//
// Risk weights are controller-configuration inputs. Negative and non-finite
// weights are treated as disabled so risk projections stay finite and cannot be
// inverted by invalid configuration.
func usableRiskWeight(weight float64) float64 {
	weight = numeric.FiniteOrZero(weight)
	if weight < 0 {
		return 0
	}
	return weight
}
