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
	return NewWeightedScore([]Component{
		NewComponent("low_hit", input.LowHitScore, 0.30),
		NewComponent("retained_pressure", input.RetainedPressure, 0.30),
		NewComponent("low_activity", input.LowActivityScore, 0.20),
		NewComponent("drop", input.DropScore, 0.20),
	})
}
