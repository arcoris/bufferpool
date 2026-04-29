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

// UsefulnessScorer is a prepared usefulness evaluator with stable weights.
//
// Usefulness measures whether retained memory is helping avoid allocation. It
// is not a pressure score, not a safety score, and not a grow command. Root
// adapters should combine it with pressure, waste, risk, and stability guards
// before any future policy decision. The zero value is valid but disabled:
// Score returns a zero-value WeightedScore and ScoreValue returns zero until a
// scorer is constructed with NewUsefulnessScorer.
type UsefulnessScorer struct {
	weights UsefulnessWeights
}

// NewUsefulnessScorer returns a scorer with normalized stable weights.
func NewUsefulnessScorer(weights UsefulnessWeights) UsefulnessScorer {
	return UsefulnessScorer{weights: normalizeUsefulnessWeights(weights)}
}

// Score returns an explainable usefulness score with copied components.
func (s UsefulnessScorer) Score(input UsefulnessInput) WeightedScore {
	if s.weights == (UsefulnessWeights{}) {
		return WeightedScore{}
	}
	return usefulnessWithNormalizedWeights(input, s.weights)
}

// ScoreValue returns only the scalar usefulness score without component allocation.
func (s UsefulnessScorer) ScoreValue(input UsefulnessInput) float64 {
	if s.weights == (UsefulnessWeights{}) {
		return 0
	}
	components := usefulnessComponents(input, s.weights)
	base := WeightedScoreValue(components[:])
	return numeric.Clamp01(base - numeric.Clamp01(input.DropPenalty)*numeric.FiniteOrZero(s.weights.DropPenalty))
}

// Usefulness is a one-off convenience wrapper over default usefulness weights.
//
// Repeated controller loops should construct UsefulnessScorer once and call its
// Score or ScoreValue methods so weights are normalized once.
//
// The formula is intentionally simple controller scaffolding, not final
// production tuning. DropPenalty reduces the final score after component
// averaging and the result is clamped to [0, 1].
func Usefulness(input UsefulnessInput) WeightedScore {
	return NewUsefulnessScorer(DefaultUsefulnessWeights()).Score(input)
}

// UsefulnessWithWeights is a one-off convenience wrapper with caller-provided weights.
//
// Repeated controller loops should construct UsefulnessScorer once with the
// stable weights and reuse it across windows.
//
// Weights are normalized through NewComponent and WeightedScoreValue, so
// negative or non-finite values become ineffective rather than propagating
// invalid scores. DropPenalty is subtractive because frequent drops mean
// retained storage is either under pressure or failing admission.
func UsefulnessWithWeights(input UsefulnessInput, weights UsefulnessWeights) WeightedScore {
	return NewUsefulnessScorer(weights).Score(input)
}

// usefulnessWithNormalizedWeights builds the diagnostic score once weights have
// already been sanitized by UsefulnessScorer.
func usefulnessWithNormalizedWeights(input UsefulnessInput, weights UsefulnessWeights) WeightedScore {
	components := usefulnessComponents(input, weights)
	base := NewWeightedScore(components[:])
	base.Value = numeric.Clamp01(base.Value - numeric.Clamp01(input.DropPenalty)*numeric.FiniteOrZero(weights.DropPenalty))
	return base
}

// usefulnessComponents returns the fixed component set used by both diagnostic
// and allocation-conscious usefulness evaluation.
func usefulnessComponents(input UsefulnessInput, weights UsefulnessWeights) [4]Component {
	return [4]Component{
		NewComponent(ComponentUsefulnessHitRatio, input.HitRatio, weights.HitRatio),
		NewComponent(ComponentUsefulnessAllocationAvoidance, input.AllocationAvoidance, weights.AllocationAvoidance),
		NewComponent(ComponentUsefulnessRetainRatio, input.RetainRatio, weights.RetainRatio),
		NewComponent(ComponentUsefulnessActivity, input.ActivityScore, weights.Activity),
	}
}
