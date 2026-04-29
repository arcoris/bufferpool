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

// Waste returns a score that is high for cold or ineffective retained storage.
func Waste(input WasteInput) WeightedScore {
	return WasteWithWeights(input, DefaultWasteWeights())
}

// WasteWithWeights returns a waste score with caller-provided weights.
//
// The score is a pure projection. A high value identifies inefficient retained
// capacity, but it is not a trim command and should be combined with pressure,
// hysteresis, cooldown, and domain policy before any future mutation.
func WasteWithWeights(input WasteInput, weights WasteWeights) WeightedScore {
	return NewWeightedScore([]Component{
		NewComponent(ComponentWasteLowHit, input.LowHitScore, weights.LowHit),
		NewComponent(ComponentWasteRetainedPressure, input.RetainedPressure, weights.RetainedPressure),
		NewComponent(ComponentWasteLowActivity, input.LowActivityScore, weights.LowActivity),
		NewComponent(ComponentWasteDrop, input.DropScore, weights.Drop),
	})
}
