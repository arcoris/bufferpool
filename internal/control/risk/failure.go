package risk

import "arcoris.dev/bufferpool/internal/control/numeric"

// ReturnFailureRisk scores Pool handoff failure ratios.
func ReturnFailureRisk(failureRatio, admissionRatio, closedRatio float64) float64 {
	return numeric.Clamp01(failureRatio*0.30 + admissionRatio*0.50 + closedRatio*0.20)
}
