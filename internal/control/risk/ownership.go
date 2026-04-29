package risk

import "arcoris.dev/bufferpool/internal/control/numeric"

// OwnershipRisk returns a high-severity score for ownership violations or double releases.
func OwnershipRisk(ownershipViolationRatio, doubleReleaseRatio float64) float64 {
	return numeric.Clamp01(ownershipViolationRatio*0.70 + doubleReleaseRatio*0.30)
}
