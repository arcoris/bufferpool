package risk

const (
	// DefaultOwnershipViolationWeight treats strict ownership violations as the
	// dominant ownership safety signal because they cross the checked-out
	// ownership boundary. It encourages investigation before retention tuning
	// and avoids hiding registry-safety signals behind double-release noise. This
	// is a tunable heuristic, not an ownership-rule invariant.
	DefaultOwnershipViolationWeight = 0.75

	// DefaultOwnershipDoubleReleaseWeight treats double release as severe, but
	// lower than an ownership violation because it may also be counted as caller
	// misuse. It encourages visibility for repeated-release bugs while avoiding
	// double counting as the dominant ownership signal. This is a tunable
	// heuristic.
	DefaultOwnershipDoubleReleaseWeight = 0.25
)

// OwnershipWeights configures ownership safety composition.
type OwnershipWeights struct {
	// OwnershipViolation weights strict ownership violations.
	OwnershipViolation float64

	// DoubleRelease weights double-release attempts.
	DoubleRelease float64
}

// DefaultOwnershipWeights returns conservative ownership safety weights.
func DefaultOwnershipWeights() OwnershipWeights {
	return OwnershipWeights{
		OwnershipViolation: DefaultOwnershipViolationWeight,
		DoubleRelease:      DefaultOwnershipDoubleReleaseWeight,
	}
}

// normalizeOwnershipWeights converts invalid ownership weights to zero and
// applies defaults only when the entire config is left unset.
func normalizeOwnershipWeights(weights OwnershipWeights) OwnershipWeights {
	if weights == (OwnershipWeights{}) {
		return DefaultOwnershipWeights()
	}
	return sanitizeOwnershipWeights(weights)
}

// sanitizeOwnershipWeights converts invalid ownership weights to zero without
// applying defaults. It is used when a domain adapter has already decided that
// a config group is explicit.
func sanitizeOwnershipWeights(weights OwnershipWeights) OwnershipWeights {
	weights.OwnershipViolation = usableRiskWeight(weights.OwnershipViolation)
	weights.DoubleRelease = usableRiskWeight(weights.DoubleRelease)
	return weights
}
