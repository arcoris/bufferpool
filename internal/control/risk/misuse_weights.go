package risk

const (
	// DefaultMisuseInvalidReleaseWeight gives malformed release attempts equal
	// default influence inside the caller-misuse component. Invalid releases
	// indicate API misuse, but they may be less severe than strict ownership
	// violations, so they are scoped to misuse rather than the ownership
	// component. This is a tunable conservative default, not a hard invariant.
	DefaultMisuseInvalidReleaseWeight = 0.50

	// DefaultMisuseDoubleReleaseWeight gives double-release attempts equal
	// default influence inside the caller-misuse component. Double release also
	// contributes to ownership risk because it crosses an ownership boundary,
	// but it remains visible here as direct caller API misuse. This is a
	// tunable conservative default, not a hard invariant.
	DefaultMisuseDoubleReleaseWeight = 0.50
)

// MisuseWeights configures caller-misuse risk composition.
type MisuseWeights struct {
	// InvalidRelease weights malformed release attempts that never identify a
	// valid checked-out lease.
	InvalidRelease float64

	// DoubleRelease weights attempts to release a lease more than once.
	DoubleRelease float64
}

// DefaultMisuseWeights returns conservative caller-misuse weights.
func DefaultMisuseWeights() MisuseWeights {
	return MisuseWeights{
		InvalidRelease: DefaultMisuseInvalidReleaseWeight,
		DoubleRelease:  DefaultMisuseDoubleReleaseWeight,
	}
}

// normalizeMisuseWeights converts invalid caller-misuse weights to zero and
// applies defaults only when the entire config is left unset.
func normalizeMisuseWeights(weights MisuseWeights) MisuseWeights {
	if weights == (MisuseWeights{}) {
		return DefaultMisuseWeights()
	}
	return sanitizeMisuseWeights(weights)
}

// sanitizeMisuseWeights converts invalid caller-misuse weights to zero without
// applying defaults. It is used when a domain adapter has already decided that
// a config group is explicit.
func sanitizeMisuseWeights(weights MisuseWeights) MisuseWeights {
	weights.InvalidRelease = usableRiskWeight(weights.InvalidRelease)
	weights.DoubleRelease = usableRiskWeight(weights.DoubleRelease)
	return weights
}
