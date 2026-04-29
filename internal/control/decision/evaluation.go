package decision

import "arcoris.dev/bufferpool/internal/control/numeric"

// Evaluation combines a generic score and recommendation.
type Evaluation struct {
	// Score is a normalized evaluation score.
	Score float64

	// Recommendation is the recommendation derived from Score by the caller.
	Recommendation Recommendation
}

// NewEvaluation returns an evaluation with a clamped score.
func NewEvaluation(score float64, recommendation Recommendation) Evaluation {
	return Evaluation{Score: numeric.Clamp01(score), Recommendation: recommendation}
}
