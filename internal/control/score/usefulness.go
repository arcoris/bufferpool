package score

import "arcoris.dev/bufferpool/internal/control/numeric"

// UsefulnessInput contains normalized signals for usefulness scoring.
type UsefulnessInput struct {
	// HitRatio is the window or smoothed reuse hit ratio.
	HitRatio float64

	// RetainRatio is the window or smoothed retained-return ratio.
	RetainRatio float64

	// AllocationAvoidance is high when allocations are avoided.
	AllocationAvoidance float64

	// ActivityScore is a normalized hotness or recent activity signal.
	ActivityScore float64

	// DropPenalty reduces usefulness when returned buffers are dropped.
	DropPenalty float64
}

// Usefulness returns an initial deterministic usefulness score.
//
// The formula is intentionally simple controller scaffolding, not final
// production tuning. DropPenalty reduces the final score after component
// averaging and the result is clamped to [0, 1].
func Usefulness(input UsefulnessInput) WeightedScore {
	return UsefulnessWithWeights(input, DefaultUsefulnessWeights())
}

// UsefulnessWithWeights returns a usefulness score with caller-provided weights.
//
// Weights are normalized through NewComponent and WeightedScoreValue, so
// negative or non-finite values become ineffective rather than propagating
// invalid scores. DropPenalty is subtractive because frequent drops mean
// retained storage is either under pressure or failing admission.
func UsefulnessWithWeights(input UsefulnessInput, weights UsefulnessWeights) WeightedScore {
	base := NewWeightedScore([]Component{
		NewComponent(ComponentUsefulnessHitRatio, input.HitRatio, weights.HitRatio),
		NewComponent(ComponentUsefulnessAllocationAvoidance, input.AllocationAvoidance, weights.AllocationAvoidance),
		NewComponent(ComponentUsefulnessRetainRatio, input.RetainRatio, weights.RetainRatio),
		NewComponent(ComponentUsefulnessActivity, input.ActivityScore, weights.Activity),
	})
	base.Value = numeric.Clamp01(base.Value - numeric.Clamp01(input.DropPenalty)*numeric.FiniteOrZero(weights.DropPenalty))
	return base
}
