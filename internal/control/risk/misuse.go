package risk

import "arcoris.dev/bufferpool/internal/control/numeric"

// MisuseRisk scores caller API misuse signals with default weights.
func MisuseRisk(invalidReleaseRatio, doubleReleaseRatio float64) float64 {
	return MisuseRiskWithWeights(invalidReleaseRatio, doubleReleaseRatio, DefaultMisuseWeights())
}

// MisuseRiskWithWeights scores caller API misuse signals with explicit weights.
//
// Invalid release is misuse only: it means the caller attempted a release that
// did not map to a valid lease. Double release is both misuse and an ownership
// boundary signal, so it is intentionally also counted by OwnershipRisk. The
// inputs are normalized ratios, weights are sanitized by WeightedAverage, and a
// zero usable weight sum returns zero.
func MisuseRiskWithWeights(invalidReleaseRatio, doubleReleaseRatio float64, weights MisuseWeights) float64 {
	values := [2]numeric.WeightedValue{
		{Value: invalidReleaseRatio, Weight: weights.InvalidRelease},
		{Value: doubleReleaseRatio, Weight: weights.DoubleRelease},
	}
	return numeric.WeightedAverage(values[:])
}
