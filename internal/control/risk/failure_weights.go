package risk

const (
	// DefaultReturnFailureAggregateWeight covers general handoff reliability. It
	// is lower than admission-specific failure weight so generic failures stay
	// visible without hiding the more actionable cause. This encourages
	// reliability diagnostics and avoids treating all close/admission failures as
	// equal. This is a tunable heuristic.
	DefaultReturnFailureAggregateWeight = 0.25

	// DefaultReturnFailureAdmissionWeight emphasizes admission/runtime failures
	// because they are more relevant to adaptive policy than close-time failures.
	// It encourages attention to pressure or policy admission problems and
	// avoids shutdown noise dominating return-path risk. This is a tunable
	// heuristic.
	DefaultReturnFailureAdmissionWeight = 0.55

	// DefaultReturnFailureClosedWeight keeps closed-Pool handoff failures lower
	// severity because hard or graceful shutdown can legitimately close Pools.
	// It preserves diagnostic visibility while avoiding poisoning normal scoring
	// during lifecycle transitions. This is a tunable heuristic.
	DefaultReturnFailureClosedWeight = 0.20
)

// ReturnFailureWeights configures Pool handoff failure composition.
type ReturnFailureWeights struct {
	// Aggregate weights all Pool return failures.
	Aggregate float64

	// Admission weights non-closed admission/runtime handoff failures.
	Admission float64

	// Closed weights close-related handoff failures.
	Closed float64
}

// DefaultReturnFailureWeights returns conservative return-failure weights.
func DefaultReturnFailureWeights() ReturnFailureWeights {
	return ReturnFailureWeights{
		Aggregate: DefaultReturnFailureAggregateWeight,
		Admission: DefaultReturnFailureAdmissionWeight,
		Closed:    DefaultReturnFailureClosedWeight,
	}
}

// normalizeReturnFailureWeights converts invalid Pool handoff weights to zero
// and applies defaults only when the entire config is left unset.
func normalizeReturnFailureWeights(weights ReturnFailureWeights) ReturnFailureWeights {
	if weights == (ReturnFailureWeights{}) {
		return DefaultReturnFailureWeights()
	}
	weights.Aggregate = usableRiskWeight(weights.Aggregate)
	weights.Admission = usableRiskWeight(weights.Admission)
	weights.Closed = usableRiskWeight(weights.Closed)
	return weights
}
