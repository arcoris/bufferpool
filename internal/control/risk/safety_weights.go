package risk

const (
	// DefaultRiskReturnFailureWeight keeps Pool handoff failures visible while
	// preventing expected shutdown handoff failures from dominating safety risk.
	// It is weaker than ownership because return failures may be operational
	// pressure or close noise, encourages diagnostics for handoff reliability,
	// and avoids suppressing retention solely due to shutdown paths. This is a
	// tunable heuristic.
	DefaultRiskReturnFailureWeight = 0.25

	// DefaultRiskOwnershipWeight makes ownership safety the strongest risk
	// component because ownership violations can indicate caller misuse or
	// boundary corruption. It encourages safety-first recommendations and avoids
	// letting ordinary pressure or return-path failures hide ownership boundary
	// problems. This is a tunable heuristic, not a registry invariant.
	DefaultRiskOwnershipWeight = 0.50

	// DefaultRiskMisuseWeight keeps invalid releases and double releases visible
	// as caller-misuse signals without outweighing strict ownership violations.
	// It encourages API-misuse diagnostics while avoiding overreaction to noisy
	// invalid-release attempts. This is a tunable heuristic.
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
	return sanitizeWeights(weights)
}

// sanitizeWeights converts invalid top-level risk weights to zero without
// applying defaults. It supports domain adapters that distinguish "unset
// config" from "explicit zero weight" before constructing a Scorer.
func sanitizeWeights(weights Weights) Weights {
	weights.ReturnFailure = usableRiskWeight(weights.ReturnFailure)
	weights.Ownership = usableRiskWeight(weights.Ownership)
	weights.Misuse = usableRiskWeight(weights.Misuse)
	return weights
}
