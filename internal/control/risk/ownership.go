package risk

import "arcoris.dev/bufferpool/internal/control/numeric"

// OwnershipRisk returns a high-severity score for ownership violations or double releases.
func OwnershipRisk(ownershipViolationRatio, doubleReleaseRatio float64) float64 {
	return OwnershipRiskWithWeights(ownershipViolationRatio, doubleReleaseRatio, DefaultOwnershipWeights())
}

// OwnershipRiskWithWeights scores ownership safety signals with weights.
//
// Ownership violations are weighted higher by default because they indicate a
// stricter safety-boundary failure than double release. Double release still
// contributes because it often points to caller lifecycle misuse.
func OwnershipRiskWithWeights(ownershipViolationRatio, doubleReleaseRatio float64, weights OwnershipWeights) float64 {
	return numeric.WeightedAverage([]numeric.WeightedValue{
		{Value: ownershipViolationRatio, Weight: weights.OwnershipViolation},
		{Value: doubleReleaseRatio, Weight: weights.DoubleRelease},
	})
}
