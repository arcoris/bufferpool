package risk

const (
	// DefaultOwnershipViolationWeight treats strict ownership violations as the
	// dominant ownership safety signal.
	DefaultOwnershipViolationWeight = 0.75

	// DefaultOwnershipDoubleReleaseWeight treats double release as severe, but
	// lower than an ownership violation because it may also be counted as caller
	// misuse.
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
	weights.OwnershipViolation = usableRiskWeight(weights.OwnershipViolation)
	weights.DoubleRelease = usableRiskWeight(weights.DoubleRelease)
	return weights
}
