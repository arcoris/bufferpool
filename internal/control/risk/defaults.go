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

	// DefaultReturnFailureAggregateWeight covers general handoff reliability.
	DefaultReturnFailureAggregateWeight = 0.25

	// DefaultReturnFailureAdmissionWeight emphasizes admission/runtime failures
	// because they are more relevant to adaptive policy than close-time failures.
	DefaultReturnFailureAdmissionWeight = 0.55

	// DefaultReturnFailureClosedWeight keeps closed-Pool handoff failures lower
	// severity because hard or graceful shutdown can legitimately close Pools.
	DefaultReturnFailureClosedWeight = 0.20

	// DefaultOwnershipViolationWeight treats strict ownership violations as the
	// dominant ownership safety signal.
	DefaultOwnershipViolationWeight = 0.75

	// DefaultOwnershipDoubleReleaseWeight treats double release as severe, but
	// lower than an ownership violation because it may also be counted as caller
	// misuse.
	DefaultOwnershipDoubleReleaseWeight = 0.25
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
