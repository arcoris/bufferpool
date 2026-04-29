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
	base := NewWeightedScore([]Component{
		NewComponent("hit_ratio", input.HitRatio, 0.40),
		NewComponent("retain_ratio", input.RetainRatio, 0.20),
		NewComponent("allocation_avoidance", input.AllocationAvoidance, 0.20),
		NewComponent("activity", input.ActivityScore, 0.20),
	})
	base.Value = numeric.Clamp01(base.Value - numeric.Clamp01(input.DropPenalty)*0.30)
	return base
}
