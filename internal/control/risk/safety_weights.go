package risk

const (
	// DefaultRiskReturnFailureWeight keeps Pool handoff failures visible while
	// preventing expected shutdown handoff failures from dominating safety risk.
	DefaultRiskReturnFailureWeight = 0.25

	// DefaultRiskOwnershipWeight makes ownership safety the strongest risk
	// component because ownership violations can indicate caller misuse or
	// boundary corruption.
	DefaultRiskOwnershipWeight = 0.50

	// DefaultRiskMisuseWeight keeps invalid releases and double releases visible
	// as caller-misuse signals without outweighing strict ownership violations.
	DefaultRiskMisuseWeight = 0.25
)

// Weights configures top-level risk aggregation.
type Weights struct {
	// ReturnFailure weights Pool return handoff failures.
	ReturnFailure float64

	// Ownership weights strict ownership safety failures.
	Ownership float64

	// Misuse weights invalid release and double-release misuse signals.
	Misuse float64
}

// DefaultWeights returns conservative initial risk aggregation weights.
func DefaultWeights() Weights {
	return Weights{
		ReturnFailure: DefaultRiskReturnFailureWeight,
		Ownership:     DefaultRiskOwnershipWeight,
		Misuse:        DefaultRiskMisuseWeight,
	}
}

// normalizeWeights converts invalid top-level risk weights to zero and applies
// defaults only when the entire config is left unset.
func normalizeWeights(weights Weights) Weights {
	if weights == (Weights{}) {
		return DefaultWeights()
	}
	weights.ReturnFailure = usableRiskWeight(weights.ReturnFailure)
	weights.Ownership = usableRiskWeight(weights.Ownership)
	weights.Misuse = usableRiskWeight(weights.Misuse)
	return weights
}
