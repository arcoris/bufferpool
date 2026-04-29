package score

import "arcoris.dev/bufferpool/internal/control/numeric"

// ConfidenceFromScore clamps a normalized score into a confidence value.
func ConfidenceFromScore(score float64) float64 {
	return numeric.Clamp01(score)
}

// ConfidenceFromGap returns confidence from the difference between candidates.
func ConfidenceFromGap(best, secondBest float64) float64 {
	return numeric.Clamp01(best - secondBest)
}
