package score

const (
	// DefaultWasteLowHitWeight emphasizes retained capacity that is not serving
	// reuse hits.
	DefaultWasteLowHitWeight = 0.35

	// DefaultWasteRetainedPressureWeight makes memory pressure a strong waste
	// signal because retained bytes have opportunity cost.
	DefaultWasteRetainedPressureWeight = 0.30

	// DefaultWasteLowActivityWeight marks quiet retained capacity as a shrink or
	// trim candidate without overreacting to one quiet window.
	DefaultWasteLowActivityWeight = 0.20

	// DefaultWasteDropWeight keeps drops visible while avoiding domination by
	// intentional pressure-policy drops.
	DefaultWasteDropWeight = 0.15
)

// WasteWeights configures waste score composition.
type WasteWeights struct {
	// LowHit weights ineffective retained reuse.
	LowHit float64

	// RetainedPressure weights retained memory opportunity cost.
	RetainedPressure float64

	// LowActivity weights cold workload behavior.
	LowActivity float64

	// Drop weights returned-buffer drop behavior.
	Drop float64
}

// DefaultWasteWeights returns conservative initial waste weights.
func DefaultWasteWeights() WasteWeights {
	return WasteWeights{
		LowHit:           DefaultWasteLowHitWeight,
		RetainedPressure: DefaultWasteRetainedPressureWeight,
		LowActivity:      DefaultWasteLowActivityWeight,
		Drop:             DefaultWasteDropWeight,
	}
}

// normalizeWasteWeights converts invalid weights to zero and applies the
// documented defaults when the caller leaves the entire config unset.
func normalizeWasteWeights(weights WasteWeights) WasteWeights {
	defaults := DefaultWasteWeights()
	if weights == (WasteWeights{}) {
		return defaults
	}
	weights.LowHit = usableScoreWeight(weights.LowHit)
	weights.RetainedPressure = usableScoreWeight(weights.RetainedPressure)
	weights.LowActivity = usableScoreWeight(weights.LowActivity)
	weights.Drop = usableScoreWeight(weights.Drop)
	return weights
}
