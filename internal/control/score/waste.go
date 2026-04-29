package score

// WasteInput contains normalized signals for waste scoring.
type WasteInput struct {
	// LowHitScore is high when reuse hit ratio is low.
	LowHitScore float64

	// RetainedPressure is high when retained memory is under pressure.
	RetainedPressure float64

	// LowActivityScore is high when recent activity is low.
	LowActivityScore float64

	// DropScore is high when returned buffers are frequently dropped.
	DropScore float64
}

// WasteScorer is a prepared waste evaluator with stable weights.
//
// Waste identifies cold or inefficient retained capacity. It is not a trim
// command and does not know memory budgets or ownership risk by itself. The
// zero value is valid but disabled: Score returns a zero-value WeightedScore
// and ScoreValue returns zero until a scorer is constructed with NewWasteScorer.
type WasteScorer struct {
	weights WasteWeights
}

// NewWasteScorer returns a scorer with normalized stable weights.
func NewWasteScorer(weights WasteWeights) WasteScorer {
	return WasteScorer{weights: normalizeWasteWeights(weights)}
}

// Score returns an explainable waste score with copied components.
func (s WasteScorer) Score(input WasteInput) WeightedScore {
	if s.weights == (WasteWeights{}) {
		return WeightedScore{}
	}
	components := wasteComponents(input, s.weights)
	return NewWeightedScore(components[:])
}

// ScoreValue returns only the scalar waste score without component allocation.
func (s WasteScorer) ScoreValue(input WasteInput) float64 {
	if s.weights == (WasteWeights{}) {
		return 0
	}
	components := wasteComponents(input, s.weights)
	return WeightedScoreValue(components[:])
}

// Waste is a one-off convenience wrapper over default waste weights.
//
// Repeated controller loops should construct WasteScorer once and call its Score
// or ScoreValue methods so weights are normalized once.
func Waste(input WasteInput) WeightedScore {
	return NewWasteScorer(DefaultWasteWeights()).Score(input)
}

// WasteWithWeights is a one-off convenience wrapper with caller-provided weights.
//
// Repeated controller loops should construct WasteScorer once with the stable
// weights and reuse it across windows.
//
// The score is a pure projection. A high value identifies inefficient retained
// capacity, but it is not a trim command and should be combined with pressure,
// hysteresis, cooldown, and domain policy before any future mutation.
func WasteWithWeights(input WasteInput, weights WasteWeights) WeightedScore {
	return NewWasteScorer(weights).Score(input)
}

// wasteComponents returns the fixed component set used by both diagnostic and
// allocation-conscious waste evaluation.
func wasteComponents(input WasteInput, weights WasteWeights) [4]Component {
	return [4]Component{
		NewComponent(ComponentWasteLowHit, input.LowHitScore, weights.LowHit),
		NewComponent(ComponentWasteRetainedPressure, input.RetainedPressure, weights.RetainedPressure),
		NewComponent(ComponentWasteLowActivity, input.LowActivityScore, weights.LowActivity),
		NewComponent(ComponentWasteDrop, input.DropScore, weights.Drop),
	}
}
